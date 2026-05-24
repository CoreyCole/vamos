package generate

import (
	"bytes"
	"os"
	"path/filepath"
)

func Write(result Result) error {
	for _, file := range result.Files {
		if err := os.MkdirAll(filepath.Dir(file.Path), 0o755); err != nil {
			return err
		}
		if existing, err := os.ReadFile(
			file.Path,
		); err == nil &&
			bytes.Equal(existing, file.Content) {
			continue
		}
		if err := os.WriteFile(file.Path, file.Content, 0o644); err != nil {
			return err
		}
	}
	return nil
}
