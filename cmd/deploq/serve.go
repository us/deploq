package main

import (
	"flag"
	"fmt"
	"log/slog"
	"os"

	"github.com/us/deploq/internal/config"
	"github.com/us/deploq/internal/deploy"
	"github.com/us/deploq/internal/server"
)

func runServe() error {
	fs := flag.NewFlagSet("serve", flag.ExitOnError)
	configPath := fs.String("config", "deploq.yaml", "path to config file")
	logFormat := fs.String("log-format", "text", "log format: text or json")
	fs.Parse(os.Args[2:])

	setupLogger(*logFormat)

	cfg, err := config.Load(*configPath)
	if err != nil {
		return fmt.Errorf("failed to load config: %w", err)
	}

	if err := cfg.Validate(); err != nil {
		return fmt.Errorf("config validation failed: %w", err)
	}

	deployer := deploy.New(cfg)
	srv := server.New(cfg, deployer)

	slog.Info("starting deploq server", "listen", cfg.Listen, "projects", len(cfg.Projects))
	return srv.ListenAndServe()
}

func setupLogger(format string) {
	var handler slog.Handler
	opts := &slog.HandlerOptions{Level: slog.LevelInfo}
	if format == "json" {
		handler = slog.NewJSONHandler(os.Stdout, opts)
	} else {
		handler = slog.NewTextHandler(os.Stdout, opts)
	}
	slog.SetDefault(slog.New(handler))
}
