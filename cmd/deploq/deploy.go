package main

import (
	"context"
	"flag"
	"fmt"
	"os"

	"github.com/us/deploq/internal/config"
	"github.com/us/deploq/internal/deploy"
)

func runDeploy() error {
	fs := flag.NewFlagSet("deploy", flag.ExitOnError)
	configPath := fs.String("config", "deploq.yaml", "path to config file")
	logFormat := fs.String("log-format", "text", "log format: text or json")
	fs.Parse(os.Args[2:])

	setupLogger(*logFormat)

	args := fs.Args()
	if len(args) < 1 {
		return fmt.Errorf("usage: deploq deploy <project>")
	}
	projectName := args[0]

	cfg, err := config.Load(*configPath)
	if err != nil {
		return fmt.Errorf("failed to load config: %w", err)
	}

	if err := cfg.Validate(); err != nil {
		return fmt.Errorf("config validation failed: %w", err)
	}

	project, ok := cfg.Projects[projectName]
	if !ok {
		return fmt.Errorf("project %q not found in config", projectName)
	}

	deployer := deploy.New(cfg)
	result := deployer.DeploySync(context.Background(), projectName, project)
	if result.Err != nil {
		return fmt.Errorf("deploy failed at step %q: %w", result.Step, result.Err)
	}

	fmt.Printf("deploy successful: %s → %s\n", projectName, result.SHA)
	return nil
}
