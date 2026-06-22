package pickleball

import "context"

// AppletEditor applies a plain-language prompt to a hidden applet iteration.
// Implementations may launch a real Agent Chat/Pi run or a test-only fixture,
// but normal UI must only expose friendly summaries from AppletEditResult.
type AppletEditor interface {
	ApplyPrompt(ctx context.Context, input AppletEditInput) (AppletEditResult, error)
}
