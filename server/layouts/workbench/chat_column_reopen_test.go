package workbench

import (
	"context"
	"strings"
	"testing"

	"github.com/a-h/templ"
)

func TestChatColumnWithReopen_ClosedUsesDataShowNotHiddenClass(t *testing.T) {
	var b strings.Builder
	if err := ChatColumnWithReopen(false, "Bot", templ.Raw("<p>chat</p>")).Render(context.Background(), &b); err != nil {
		t.Fatal(err)
	}
	out := b.String()
	if !strings.Contains(out, `id="workbench-v2-threads-reopen"`) {
		t.Fatalf("missing reopen: %s", out)
	}
	if !strings.Contains(out, `data-show="!$workbench.regions.workbenchV2Threads.visible"`) &&
		!strings.Contains(out, "data-show=\"!$workbench.regions.workbenchV2Threads.visible\"") {
		// templ may escape differently
		if !strings.Contains(out, "workbenchV2Threads.visible") || !strings.Contains(out, "data-show") {
			t.Fatalf("missing data-show binding: %s", out)
		}
	}
	idx := strings.Index(out, `id="workbench-v2-threads-reopen"`)
	snippet := out[idx:min(idx+400, len(out))]
	if strings.Contains(snippet, "max-md:hidden") {
		t.Fatalf("max-md:hidden must not be on reopen: %s", snippet)
	}
	// closed SSR: no display:none inline
	if strings.Contains(snippet, "display: none") || strings.Contains(snippet, "display:none") {
		t.Fatalf("closed SSR should not inline-hide reopen: %s", snippet)
	}
}

func TestChatColumnWithReopen_OpenSSRHidesInline(t *testing.T) {
	var b strings.Builder
	if err := ChatColumnWithReopen(true, "Bot", templ.Raw("<p>chat</p>")).Render(context.Background(), &b); err != nil {
		t.Fatal(err)
	}
	out := b.String()
	idx := strings.Index(out, `id="workbench-v2-threads-reopen"`)
	snippet := out[idx:min(idx+400, len(out))]
	if !strings.Contains(snippet, "display: none") && !strings.Contains(snippet, "display:none") {
		t.Fatalf("open SSR should inline-hide reopen: %s", snippet)
	}
}
