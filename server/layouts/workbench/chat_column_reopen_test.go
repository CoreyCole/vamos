package workbench

import (
	"context"
	"strings"
	"testing"

	"github.com/a-h/templ"
)

func TestChatColumnWithReopen_ClosedShowsVisibleSlot(t *testing.T) {
	var b strings.Builder
	if err := ChatColumnWithReopen(
		false,
		"Bot",
		templ.Raw("<p>chat</p>"),
	).Render(context.Background(), &b); err != nil {
		t.Fatal(err)
	}
	out := b.String()
	if !strings.Contains(out, `id="workbench-v2-threads-reopen"`) {
		t.Fatalf("missing reopen: %s", out)
	}
	if !strings.Contains(out, `id="workbench-v2-chat-header"`) ||
		!strings.Contains(out, "h-10") {
		t.Fatalf("chat header missing fixed h-10: %s", out)
	}
	if !strings.Contains(out, ">B</span>") ||
		!strings.Contains(out, `aria-label="Share"`) {
		t.Fatalf("chat header missing avatar or share: %s", out)
	}
	if !strings.Contains(out, "Show roster sidebar (Ctrl+B)") {
		t.Fatalf("reopen tooltip must show Ctrl+B: %s", out)
	}
	if strings.Contains(out, "/new") {
		t.Fatalf("chat header must not offer /new: %s", out)
	}
	idx := strings.Index(out, `id="workbench-v2-threads-reopen"`)
	end := strings.Index(out[idx:], ">")
	openTag := out[idx : idx+end]
	if strings.Contains(openTag, "max-md:hidden") {
		t.Fatalf("max-md:hidden must not be on reopen: %s", openTag)
	}
	classIdx := strings.Index(openTag, `class="`)
	if classIdx < 0 {
		t.Fatalf("no class on reopen: %s", openTag)
	}
	classEnd := strings.Index(openTag[classIdx+7:], `"`)
	classVal := openTag[classIdx+7 : classIdx+7+classEnd]
	if strings.Contains(classVal, "invisible") || strings.Contains(classVal, "hidden") {
		t.Fatalf("closed SSR hamburger should take space: %s", classVal)
	}
}

func TestChatColumnWithReopen_OpenHidesHamburgerSlot(t *testing.T) {
	var b strings.Builder
	if err := ChatColumnWithReopen(
		true,
		"Bot",
		templ.Raw("<p>chat</p>"),
	).Render(context.Background(), &b); err != nil {
		t.Fatal(err)
	}
	out := b.String()
	idx := strings.Index(out, `id="workbench-v2-threads-reopen"`)
	end := strings.Index(out[idx:], ">")
	openTag := out[idx : idx+end]
	classIdx := strings.Index(openTag, `class="`)
	classEnd := strings.Index(openTag[classIdx+7:], `"`)
	classVal := openTag[classIdx+7 : classIdx+7+classEnd]
	if !strings.Contains(classVal, "hidden") {
		t.Fatalf(
			"open threads should hide hamburger so title is left-aligned: %s",
			classVal,
		)
	}
	if !strings.Contains(out, "visible !== false") {
		t.Fatalf("reopen data-class must not flash open before signals hydrate: %s", out)
	}
	if strings.Contains(classVal, "invisible") {
		t.Fatalf("hidden hamburger must not keep layout space: %s", classVal)
	}
}
