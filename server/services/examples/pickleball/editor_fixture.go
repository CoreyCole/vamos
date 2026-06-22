package pickleball

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"
)

// FixtureEditor is the deterministic applet editor used by tests and explicit
// local fixture mode. It is intentionally not the product prompt-to-code path.
type FixtureEditor struct {
	Enabled bool
}

func (e FixtureEditor) ApplyPrompt(ctx context.Context, input AppletEditInput) (AppletEditResult, error) {
	if !e.Enabled {
		return AppletEditResult{FailureUserMessage: unchangedFailureMessage()}, fmt.Errorf("fixture applet editor is disabled")
	}
	if err := ctx.Err(); err != nil {
		return AppletEditResult{}, err
	}
	mainPath := filepath.Join(input.IterationDir, "main.go")
	data, err := os.ReadFile(mainPath)
	if err != nil {
		return AppletEditResult{}, fmt.Errorf("read applet source: %w", err)
	}
	source := string(data)
	marker := "// Last friendly prompt: " + promptSummary(input.Prompt) + "\n"
	if strings.Contains(source, "// Last friendly prompt:") {
		re := regexp.MustCompile(`(?m)^// Last friendly prompt:.*\n`)
		source = re.ReplaceAllString(source, marker)
	} else {
		source = marker + source
	}
	if err := os.WriteFile(mainPath, []byte(source), 0o644); err != nil {
		return AppletEditResult{}, fmt.Errorf("write applet source: %w", err)
	}
	return AppletEditResult{
		ChangedFiles:       []string{"main.go"},
		UserSummary:        "Done — I updated the app and files.",
		FailureUserMessage: unchangedFailureMessage(),
	}, nil
}

func unchangedFailureMessage() string {
	return "I couldn't make that change safely. Your app is unchanged."
}
