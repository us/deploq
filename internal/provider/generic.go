package provider

import (
	"crypto/subtle"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
)

// Generic implements the Provider interface for generic/CI webhooks.
// Uses X-Deploq-Token header for authentication.
type Generic struct{}

func (g *Generic) Name() string { return "generic" }

// Verify checks the X-Deploq-Token header using constant-time comparison.
func (g *Generic) Verify(r *http.Request, body []byte, secret string) error {
	if secret == "" {
		return fmt.Errorf("webhook secret is not configured")
	}

	token := r.Header.Get("X-Deploq-Token")
	if token == "" {
		return fmt.Errorf("missing X-Deploq-Token header")
	}

	if subtle.ConstantTimeCompare([]byte(token), []byte(secret)) != 1 {
		return fmt.Errorf("token verification failed")
	}

	return nil
}

// ParseEvent extracts ref and SHA from a generic webhook payload.
func (g *Generic) ParseEvent(body []byte) (Event, error) {
	var payload struct {
		Ref string `json:"ref"`
		SHA string `json:"sha"`
	}
	if err := json.Unmarshal(body, &payload); err != nil {
		return Event{}, fmt.Errorf("parsing generic payload: %w", err)
	}

	if payload.Ref == "" {
		return Event{}, fmt.Errorf("missing ref in payload")
	}
	if payload.SHA == "" {
		return Event{}, fmt.Errorf("missing sha in payload")
	}
	if err := ValidateSHA(payload.SHA); err != nil {
		return Event{}, fmt.Errorf("generic payload: %w", err)
	}

	branch := strings.TrimPrefix(payload.Ref, "refs/heads/")

	return Event{
		Ref:    payload.Ref,
		SHA:    payload.SHA,
		Branch: branch,
	}, nil
}
