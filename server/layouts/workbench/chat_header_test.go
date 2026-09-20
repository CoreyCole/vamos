package workbench

import "testing"

func TestParseChatHeaderTitlePlanSlug(t *testing.T) {
	got := ParseChatHeaderTitle("2026-09-08_10-10-54_agent-memory-observable-context")
	if got.Display != "Agent Memory Observable Context" {
		t.Fatalf("Display = %q, want humanized slug", got.Display)
	}
	if got.Datetime != "Sep 8, 2026 · 10:10" {
		t.Fatalf("Datetime = %q, want Sep 8, 2026 · 10:10", got.Datetime)
	}
}

func TestParseChatHeaderTitleUnderscoreSlug(t *testing.T) {
	got := ParseChatHeaderTitle("2026-01-02_03-04-05_foo_bar-baz")
	if got.Display != "Foo Bar Baz" {
		t.Fatalf("Display = %q", got.Display)
	}
	if got.Datetime != "Jan 2, 2026 · 03:04" {
		t.Fatalf("Datetime = %q", got.Datetime)
	}
}

func TestParseChatHeaderTitleBotNamePassthrough(t *testing.T) {
	got := ParseChatHeaderTitle("Vamos Lead Engineer")
	if got.Display != "Vamos Lead Engineer" {
		t.Fatalf("Display = %q", got.Display)
	}
	if got.Datetime != "" {
		t.Fatalf("Datetime = %q, want empty for non-slug", got.Datetime)
	}
}

func TestParseChatHeaderTitleEmpty(t *testing.T) {
	got := ParseChatHeaderTitle("  ")
	if got.Display != "" || got.Datetime != "" {
		t.Fatalf("got %+v", got)
	}
}

func TestParseChatHeaderTitleInvalidDate(t *testing.T) {
	got := ParseChatHeaderTitle("2026-13-40_99-99-99_still-a-slug")
	if got.Display != "2026-13-40_99-99-99_still-a-slug" || got.Datetime != "" {
		t.Fatalf("invalid date must pass through: %+v", got)
	}
}

func TestParseChatHeaderTitleDocsRoomID(t *testing.T) {
	got := ParseChatHeaderTitle("docs--vamos")
	if got.Display != "Docs / Vamos" || got.Datetime != "" {
		t.Fatalf("docs room: %+v", got)
	}
	nested := ParseChatHeaderTitle("docs--vamos--shots")
	if nested.Display != "Docs / Vamos / Shots" {
		t.Fatalf("nested docs room: %+v", nested)
	}
}

func TestHumanizePlanRoomID(t *testing.T) {
	if got := HumanizePlanRoomID("docs--vamos"); got != "Docs / Vamos" {
		t.Fatalf("got %q", got)
	}
	slug := "2026-09-08_10-10-54_agent-memory-observable-context"
	if got := HumanizePlanRoomID(slug); got != slug {
		t.Fatalf("timestamp slug must stay raw for ParseChatHeaderTitle: %q", got)
	}
}
