package deploy

import (
	"context"
	"fmt"
)

// ComposeBuild runs docker compose build in the given directory.
func ComposeBuild(ctx context.Context, dir, composeFile string) (string, error) {
	_, stderr, err := runCommand(ctx, dir, "docker", "compose", "-f", composeFile, "build")
	if err != nil {
		return stderr, fmt.Errorf("docker compose build: %w", err)
	}
	return "", nil
}

// ComposeUp runs docker compose up -d in the given directory.
func ComposeUp(ctx context.Context, dir, composeFile string) (string, error) {
	_, stderr, err := runCommand(ctx, dir, "docker", "compose", "-f", composeFile, "up", "-d")
	if err != nil {
		return stderr, fmt.Errorf("docker compose up: %w", err)
	}
	return "", nil
}
