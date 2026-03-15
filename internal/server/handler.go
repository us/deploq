package server

import (
	"encoding/json"
	"errors"
	"io"
	"log/slog"
	"net/http"

	"github.com/us/deploq/internal/config"
	"github.com/us/deploq/internal/provider"
)

const maxBodySize = 1 << 20 // 1 MB

func (s *Server) handleWebhook(w http.ResponseWriter, r *http.Request) {
	projectName := r.PathValue("project")

	// Validate project name (prevent path traversal)
	if !config.ValidProjectName.MatchString(projectName) {
		slog.Warn("invalid project name", "project", projectName)
		http.Error(w, "invalid project name", http.StatusBadRequest)
		return
	}

	// Look up project config
	project, ok := s.cfg.Projects[projectName]
	if !ok {
		slog.Warn("unknown project", "project", projectName)
		http.Error(w, "project not found", http.StatusNotFound)
		return
	}

	// Read body with size limit
	r.Body = http.MaxBytesReader(w, r.Body, maxBodySize)
	body, err := io.ReadAll(r.Body)
	if err != nil {
		slog.Warn("failed to read request body", "error", err)
		var maxBytesErr *http.MaxBytesError
		if errors.As(err, &maxBytesErr) {
			http.Error(w, "request body too large", http.StatusRequestEntityTooLarge)
		} else {
			http.Error(w, "failed to read request body", http.StatusBadRequest)
		}
		return
	}

	// Detect provider
	prov, err := provider.Detect(r)
	if err != nil {
		slog.Warn("unknown webhook provider", "error", err)
		http.Error(w, "unrecognized webhook source", http.StatusBadRequest)
		return
	}
	slog.Info("webhook received", "project", projectName, "provider", prov.Name())

	// Verify signature/token
	if err := prov.Verify(r, body, project.Secret); err != nil {
		slog.Warn("webhook verification failed",
			"project", projectName,
			"provider", prov.Name(),
			"error", err,
		)
		http.Error(w, "unauthorized", http.StatusUnauthorized)
		return
	}

	// Parse event
	event, err := prov.ParseEvent(body)
	if err != nil {
		slog.Warn("failed to parse webhook event",
			"project", projectName,
			"provider", prov.Name(),
			"error", err,
		)
		http.Error(w, "invalid payload", http.StatusBadRequest)
		return
	}

	// Branch filter
	if event.Branch != project.Branch {
		slog.Info("skipping non-matching branch",
			"project", projectName,
			"got", event.Branch,
			"want", project.Branch,
		)
		respondJSON(w, http.StatusOK, map[string]string{
			"status": "skipped",
			"reason": "branch mismatch",
		})
		return
	}

	// Start deploy (async)
	isDuplicate, isLocked := s.deployer.Deploy(projectName, project, event.SHA)

	if isDuplicate {
		respondJSON(w, http.StatusOK, map[string]string{
			"status": "skipped",
			"reason": "duplicate sha",
		})
		return
	}

	if isLocked {
		respondJSON(w, http.StatusConflict, map[string]string{
			"status": "rejected",
			"reason": "deploy already in progress",
		})
		return
	}

	respondJSON(w, http.StatusAccepted, map[string]string{
		"status": "accepted",
		"sha":    event.SHA,
	})
}

func (s *Server) handleHealth(w http.ResponseWriter, r *http.Request) {
	respondJSON(w, http.StatusOK, map[string]string{
		"status": "ok",
	})
}

func respondJSON(w http.ResponseWriter, status int, data any) {
	buf, err := json.Marshal(data)
	if err != nil {
		slog.Error("failed to marshal json response", "error", err)
		http.Error(w, "internal server error", http.StatusInternalServerError)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	w.Write(buf)
}
