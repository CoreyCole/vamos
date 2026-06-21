package qrspicmd

import (
	"embed"
	"os"
	"path/filepath"
)

//go:embed assets/q_manager_child_extension.js
var qManagerAssets embed.FS

func ResolveChildExtensionPath(runRoot string) (string, error) {
	data, err := qManagerAssets.ReadFile("assets/q_manager_child_extension.js")
	if err != nil {
		return "", err
	}
	path := filepath.Join(runRoot, "assets", "q_manager_child_extension.js")
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return "", err
	}
	if err := os.WriteFile(path, data, 0o644); err != nil {
		return "", err
	}
	return path, nil
}
