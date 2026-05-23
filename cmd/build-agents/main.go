package main

import (
	"context"
	"fmt"
	"os"

	"github.com/CoreyCole/vamos/cmd/build-agents/internal/build"
)

func main() {
	repoRoot, err := os.Getwd()
	if err != nil {
		fmt.Fprintf(os.Stderr, "build-agents: get cwd: %v\n", err)
		os.Exit(1)
	}

	cmd := build.NewRootCommand(build.Options{
		RepoRoot:   repoRoot,
		StateDir:   ".build-agents",
		BinaryName: envDefault("BINARY_NAME", "agents-server"),
		Stdout:     os.Stdout,
		Stderr:     os.Stderr,
	})
	if err := cmd.ExecuteContext(context.Background()); err != nil {
		fmt.Fprintf(os.Stderr, "build-agents: %v\n", err)
		os.Exit(1)
	}
}

func envDefault(name, fallback string) string {
	if value := os.Getenv(name); value != "" {
		return value
	}
	return fallback
}
