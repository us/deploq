package main

import (
	"flag"
	"fmt"
	"os"

	"github.com/uscompany/deploq/internal/config"
)

func runValidate() error {
	fs := flag.NewFlagSet("validate", flag.ExitOnError)
	configPath := fs.String("config", "deploq.yaml", "path to config file")
	fs.Parse(os.Args[2:])

	cfg, err := config.Load(*configPath)
	if err != nil {
		return fmt.Errorf("failed to load config: %w", err)
	}

	if err := cfg.Validate(); err != nil {
		return fmt.Errorf("validation failed: %w", err)
	}

	fmt.Printf("config OK: %d project(s) configured\n", len(cfg.Projects))
	for name, p := range cfg.Projects {
		fmt.Printf("  %s: path=%s branch=%s compose=%s\n", name, p.Path, p.Branch, p.ComposeFile)
	}
	return nil
}
