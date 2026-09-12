package markdown

import (
	"bytes"
	"strings"
	"testing"

	"github.com/a-h/templ"

	"github.com/CoreyCole/vamos/server/layouts/workbench"
)

func TestThreadWorkbenchPageHidesAppHeaderBelowMd(t *testing.T) {
	t.Parallel()

	state, err := workbench.BuildWorkbenchV2State(workbench.WorkbenchV2Args{
		ViewportClass: workbench.ViewportMobile,
		ThreadsOpen:   true,
		ChatOpen:      true,
		ArtifactOpen:  true,
		Threads:       templ.NopComponent,
		Chat:          templ.NopComponent,
		Artifact:      templ.NopComponent,
		Comments:      templ.NopComponent,
	})
	if err != nil {
		t.Fatalf("BuildWorkbenchV2State: %v", err)
	}
	var body bytes.Buffer
	if err := ThreadWorkbenchPage("t@example.com", state).Render(t.Context(), &body); err != nil {
		t.Fatalf("ThreadWorkbenchPage.Render: %v", err)
	}
	html := body.String()
	if !strings.Contains(html, `id="app-header"`) {
		t.Fatalf("missing app-header: %s", html)
	}
	if !strings.Contains(html, `hidden md:block`) {
		t.Fatalf("app-header missing hidden md:block for mobile single chrome: %s", html)
	}
	if !strings.Contains(html, `id="workbench-mobile-chat-comments"`) {
		t.Fatalf("missing shared mobile icon header: %s", html)
	}
	if strings.Contains(html, `id="workbench-mobile-tabs"`) || strings.Contains(html, `role="tablist"`) {
		t.Fatalf("unexpected mobile tablist: %s", html)
	}
}
