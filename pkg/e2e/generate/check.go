package generate

import (
	"bytes"
	"fmt"
	"os"

	"github.com/CoreyCole/vamos/pkg/e2e/story"
)

func CheckFresh(features []story.Feature, opts Options) error {
	result, err := Generate(features, opts)
	if err != nil {
		return err
	}
	for _, file := range result.Files {
		existing, err := os.ReadFile(file.Path)
		if err != nil {
			return fmt.Errorf(
				"generated file %s missing or unreadable: %w; run vamos e2e generate",
				file.Path,
				err,
			)
		}
		if !bytes.Equal(existing, file.Content) {
			return fmt.Errorf(
				"generated file %s is stale; run vamos e2e generate",
				file.Path,
			)
		}
	}
	return nil
}
