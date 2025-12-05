package gqlgen

import (
	"bytes"
	"context"
	"fmt"
	"os/exec"
	"strings"
)

// Run executes the gqlgen binary against the provided config file.
func Run(ctx context.Context, configPath string) error {
	if _, err := exec.LookPath("gqlgen"); err != nil {
		return fmt.Errorf("gqlgen binary not found in PATH: %w", err)
	}

	cmd := exec.CommandContext(ctx, "gqlgen", "--config", configPath)
	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr

	if err := cmd.Run(); err != nil {
		output := strings.TrimSpace(stdout.String() + "\n" + stderr.String())
		if output != "" {
			return fmt.Errorf("run gqlgen: %w: %s", err, output)
		}
		return fmt.Errorf("run gqlgen: %w", err)
	}
	return nil
}
