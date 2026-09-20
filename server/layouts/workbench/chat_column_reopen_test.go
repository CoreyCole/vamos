package workbench

import (
	"bytes"
	"context"
	"strings"
	"testing"

	"github.com/a-h/templ"
)

func TestChatColumnWithReopen_ClosedShowsVisibleSlot(t *testing.T) {
	var b strings.Builder
	if err := ChatColumnWithReopen(
		false,
		true,
		"Bot",
		templ.Raw("<p>chat</p>"),
		nil,
	).Render(context.Background(), &b); err != nil {
		t.Fatal(err)
	}
	out := b.String()
	if !strings.Contains(out, `id="workbench-v2-threads-reopen"`) {
		t.Fatalf("missing reopen: %s", out)
	}
	if !strings.Contains(out, `id="workbench-v2-chat-header"`) ||
		!strings.Contains(out, "h-10") ||
		!strings.Contains(out, "hidden") ||
		!strings.Contains(out, "md:flex") {
		t.Fatalf("chat header missing fixed h-10 desktop-only band: %s", out)
	}
	if !strings.Contains(out, ">B</span>") ||
		!strings.Contains(out, `data-testid="workbench-overflow-actions"`) {
		t.Fatalf("chat header missing avatar or share: %s", out)
	}
	if !strings.Contains(out, "Roster sidebar (Ctrl+B)") {
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

func TestChatColumnWithReopen_OpenKeepsHamburgerPressed(t *testing.T) {
	var b strings.Builder
	if err := ChatColumnWithReopen(
		true,
		true,
		"Bot",
		templ.Raw("<p>chat</p>"),
		nil,
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
	if strings.Contains(classVal, "hidden") {
		t.Fatalf("open threads must keep hamburger visible: %s", classVal)
	}
	if !strings.Contains(out, `aria-pressed="true"`) {
		t.Fatalf("open roster hamburger must be pressed: %s", out)
	}
}

func TestChatColumnWithPlanReopenUsesSwatchNotGlyph(t *testing.T) {
	var buf bytes.Buffer
	if err := ChatColumnWithPlanReopen(
		true,
		true,
		"2-alpha",
		templ.Raw("<div id=\"chat-body\"></div>"),
		nil,
	).Render(context.Background(), &buf); err != nil {
		t.Fatal(err)
	}
	out := buf.String()
	if !strings.Contains(out, `data-chat-plan-swatch`) {
		t.Fatalf("missing plan swatch: %s", out)
	}
	if strings.Contains(out, ">2</span>") {
		t.Fatal("plan header must not show digit glyph")
	}
}

func TestChatColumnArtifactReopenWhenClosed(t *testing.T) {
	var buf bytes.Buffer
	if err := ChatColumnWithReopen(
		true,
		false,
		"Bot",
		templ.Raw("<p>chat</p>"),
		nil,
	).Render(context.Background(), &buf); err != nil {
		t.Fatal(err)
	}
	out := buf.String()
	if !strings.Contains(out, `data-testid="artifact-open-details"`) {
		t.Fatalf("missing Open details: %s", out)
	}
	if !strings.Contains(out, `id="workbench-v2-artifact-reopen"`) {
		t.Fatalf("missing artifact reopen slot: %s", out)
	}
	if !strings.Contains(out, `class="sr-only"`) ||
		!strings.Contains(out, "Open details") {
		t.Fatalf("want icon-only Open details with sr-only: %s", out)
	}
	if strings.Contains(out, `>Open details</span>`) &&
		!strings.Contains(out, `sr-only">Open details`) {
		t.Fatalf("Open details must not be visible label: %s", out)
	}
}

func TestChatColumnShareOverflowMenu(t *testing.T) {
	var buf bytes.Buffer
	if err := ChatColumnWithReopen(
		true,
		true,
		"Bot",
		templ.Raw("<p>chat</p>"),
		nil,
	).Render(context.Background(), &buf); err != nil {
		t.Fatal(err)
	}
	out := buf.String()
	if !strings.Contains(out, `data-testid="workbench-overflow-actions"`) {
		t.Fatalf("missing overflow menu: %s", out)
	}
	if !strings.Contains(out, "Share artifact") || !strings.Contains(out, "Share chat") {
		t.Fatalf("missing Share menu items: %s", out)
	}
	if strings.Contains(out, `d="M4 12v7a1 1 0 001 1h14`) {
		t.Fatalf("standalone share icon still present: %s", out)
	}
}

func TestChatColumnUnifiedOverflowFoldsArtifactActions(t *testing.T) {
	var buf bytes.Buffer
	overflow := OverflowActions(OverflowActionsArgs{
		Label: "Share",
		Groups: []OverflowActionGroup{{
			Actions: []OverflowAction{
				{Label: "Share artifact", Kind: OverflowActionButton, ClientAction: "1"},
				{Label: "Share chat", Kind: OverflowActionButton, ClientAction: "1"},
				{
					Label:        "Copy document contents",
					Kind:         OverflowActionButton,
					ClientAction: "1",
				},
				{Label: "Comment", Kind: OverflowActionButton, ClientAction: "1"},
			},
		}},
	})
	if err := ChatColumnWithReopen(
		true,
		true,
		"Bot",
		templ.Raw("<p>chat</p>"),
		overflow,
	).Render(context.Background(), &buf); err != nil {
		t.Fatal(err)
	}
	out := buf.String()
	if n := strings.Count(out, `data-testid="workbench-overflow-actions"`); n != 1 {
		t.Fatalf("want exactly one overflow menu, got %d: %s", n, out)
	}
	for _, want := range []string{"Share artifact", "Share chat", "Copy document contents", "Comment"} {
		if !strings.Contains(out, want) {
			t.Fatalf("missing %q in unified overflow: %s", want, out)
		}
	}
	if strings.Contains(out, ">Share</p>") || strings.Contains(out, ">Artifact</p>") {
		t.Fatalf("flat menu must not paint section headers: %s", out)
	}
	if strings.Contains(out, "Copy path") {
		t.Fatalf("Copy path must be folded into Share artifact: %s", out)
	}
	if strings.Contains(out, "Chat about this plan") {
		t.Fatalf("plan-room chat link must not appear in this fixture: %s", out)
	}
	if strings.Contains(out, `d="M4 12v7a1 1 0 001 1h14`) {
		t.Fatalf("standalone share icon still present: %s", out)
	}
}

func TestChatColumnPlanSlugTitleAndDatetime(t *testing.T) {
	var buf bytes.Buffer
	title := "2026-09-08_10-10-54_agent-memory-observable-context"
	if err := ChatColumnWithPlanReopen(
		true,
		true,
		title,
		templ.Raw("<div></div>"),
		nil,
	).Render(context.Background(), &buf); err != nil {
		t.Fatal(err)
	}
	out := buf.String()
	if !strings.Contains(out, "Agent Memory Observable Context") {
		t.Fatalf("missing human title: %s", out)
	}
	if !strings.Contains(out, "Sep 8, 2026 · 10:10") {
		t.Fatalf("missing datetime: %s", out)
	}
	if strings.Contains(out, title) {
		t.Fatalf("raw plan id must not remain in header: %s", out)
	}
	if n := strings.Count(out, `data-testid="workbench-overflow-actions"`); n != 1 {
		t.Fatalf("HARD LOCK A: want one header kebab, got %d", n)
	}
}
