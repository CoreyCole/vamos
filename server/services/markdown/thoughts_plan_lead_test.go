package markdown

import (
	"bytes"
	"context"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"testing"

	"github.com/labstack/echo/v4"

	"github.com/CoreyCole/vamos/server/layouts/workbench"
)

type planLeadThreadRenderer struct {
	threadWorkbenchTestRenderer
}

func (r *planLeadThreadRenderer) FindSharedThreadForDoc(
	_ context.Context,
	docPath string,
) (string, error) {
	if strings.Contains(docPath, "plans/alpha") {
		return "plan-alpha-thread", nil
	}
	return "", nil
}

func TestThoughtsPlanDocKeepsPlanLeadChat(t *testing.T) {
	root := t.TempDir()
	mustMkdirAll(t, filepath.Join(root, "owner", "plans", "alpha"))
	mustWriteFile(
		t,
		filepath.Join(root, "owner", "plans", "alpha", "design.md"),
		[]byte("# Design\n\nBody.\n"),
	)
	svc, err := NewService(root, nil, nil)
	if err != nil {
		t.Fatal(err)
	}
	renderer := &planLeadThreadRenderer{}
	svc.WithWorkbenchThreadRenderer(renderer)

	e := echo.New()
	req := httptest.NewRequest(
		http.MethodGet,
		"/thoughts/owner/plans/alpha/design.md",
		nil,
	)
	c := e.NewContext(req, httptest.NewRecorder())
	page := &PageArgs{
		FilePath:  "owner/plans/alpha/design.md",
		UserEmail: "t@example.com",
		ViewerArgs: ViewerArgs{
			RawMarkdown: "# Design\n",
		},
	}
	state, err := svc.buildThoughtsV2WorkbenchState(c, page)
	if err != nil {
		t.Fatal(err)
	}
	var body bytes.Buffer
	if err := workbench.Workbench(state).Render(t.Context(), &body); err != nil {
		t.Fatal(err)
	}
	html := body.String()
	for _, want := range []string{
		`id="workbench-v2-chat"`,
		`id="thread-chat"`,
		`<span>chat about this plan</span>`,
		`href="/rooms/plan/alpha?artifact=thoughts%2Fowner%2Fplans%2Falpha%2Fdesign.md"`,
		`id="workbench-v2-roster"`,
		`href="/rooms/plan/alpha"`,
	} {
		if !strings.Contains(html, want) {
			t.Fatalf("missing %q in:\n%s", want, html)
		}
	}
	if renderer.chatThreadID != "plan-alpha-thread" {
		t.Fatalf("chat thread = %q", renderer.chatThreadID)
	}
	overflowStart := strings.Index(html, `data-testid="workbench-overflow-actions"`)
	if overflowStart < 0 {
		t.Fatal("missing overflow actions")
	}
	overflow := html[overflowStart:]
	if end := strings.Index(overflow, `id="thread-artifact-browser"`); end > 0 {
		overflow = overflow[:end]
	}
	if strings.Contains(overflow, "<span>Thoughts</span>") {
		t.Fatalf("Thoughts still in 3-dot on thoughts:\n%s", overflow)
	}
}
