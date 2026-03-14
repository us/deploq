package main

import (
	"fmt"
	"os"
	"path/filepath"
)

func runInit() error {
	const filename = "pushup.yaml"

	if _, err := os.Stat(filename); err == nil {
		return fmt.Errorf("%s already exists — refusing to overwrite", filename)
	}

	// Detect existing compose files
	composeFile := "docker-compose.yml"
	for _, candidate := range []string{"docker-compose.yml", "docker-compose.yaml", "compose.yml", "compose.yaml"} {
		if _, err := os.Stat(candidate); err == nil {
			composeFile = candidate
			break
		}
	}

	cwd, err := os.Getwd()
	if err != nil {
		return fmt.Errorf("failed to get working directory: %w", err)
	}
	projectName := filepath.Base(cwd)

	content := fmt.Sprintf(`# pushup configuration
# Documentation: https://github.com/uscompany/pushup

listen: ":9090"

projects:
  %s:
    path: "%s"
    branch: main
    secret: "${PUSHUP_SECRET_%s}"
    compose_file: "%s"
    # deploy_timeout: 15m  # optional, default: 15m
`, projectName, cwd, toEnvName(projectName), composeFile)

	if err := os.WriteFile(filename, []byte(content), 0644); err != nil {
		return fmt.Errorf("failed to write %s: %w", filename, err)
	}

	fmt.Printf("created %s\n", filename)
	fmt.Println("edit the file and set the required environment variables before running 'pushup serve'")
	return nil
}

func toEnvName(s string) string {
	result := make([]byte, 0, len(s))
	for _, c := range []byte(s) {
		switch {
		case c >= 'a' && c <= 'z':
			result = append(result, c-32) // uppercase
		case c >= 'A' && c <= 'Z', c >= '0' && c <= '9':
			result = append(result, c)
		default:
			result = append(result, '_')
		}
	}
	return string(result)
}
