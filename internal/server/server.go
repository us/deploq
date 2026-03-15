package server

import (
	"context"
	"log/slog"
	"net/http"
	"os/signal"
	"syscall"
	"time"

	"github.com/us/deploq/internal/config"
	"github.com/us/deploq/internal/deploy"
)

// Server is the deploq HTTP server.
type Server struct {
	cfg        *config.Config
	deployer   *deploy.Deployer
	httpServer *http.Server
}

// New creates a new Server.
func New(cfg *config.Config, deployer *deploy.Deployer) *Server {
	s := &Server{
		cfg:      cfg,
		deployer: deployer,
	}

	mux := http.NewServeMux()
	mux.HandleFunc("POST /webhook/{project}", s.handleWebhook)
	mux.HandleFunc("GET /health", s.handleHealth)
	mux.HandleFunc("GET /status/{project}", s.handleStatus)

	s.httpServer = &http.Server{
		Addr:              cfg.Listen,
		Handler:           mux,
		ReadTimeout:       10 * time.Second,
		WriteTimeout:      15 * time.Second,
		IdleTimeout:       60 * time.Second,
		ReadHeaderTimeout: 5 * time.Second,
	}

	return s
}

// ListenAndServe starts the server with graceful shutdown support.
func (s *Server) ListenAndServe() error {
	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()

	errCh := make(chan error, 1)
	go func() {
		errCh <- s.httpServer.ListenAndServe()
	}()

	select {
	case err := <-errCh:
		if err != http.ErrServerClosed {
			return err
		}
	case <-ctx.Done():
		slog.Info("shutting down server...")

		shutdownCtx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
		defer cancel()

		if err := s.httpServer.Shutdown(shutdownCtx); err != nil {
			slog.Error("server shutdown error", "error", err)
		}

		slog.Info("waiting for active deploys to complete...")
		deployCtx, deployCancel := context.WithTimeout(context.Background(), 5*time.Minute)
		defer deployCancel()
		if err := s.deployer.Wait(deployCtx); err != nil {
			slog.Warn("not all deploys completed during shutdown", "error", err)
		}
		slog.Info("shutdown complete")
	}

	return nil
}
