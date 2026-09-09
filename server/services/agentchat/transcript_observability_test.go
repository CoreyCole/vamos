package agentchat

import (
	"strings"
	"testing"

	"github.com/CoreyCole/vamos/pkg/agents/conversation"
	"github.com/CoreyCole/vamos/server/testhelpers"
)

func TestDecodePersistedTranscriptItems_HandoffIsDetailCut(t *testing.T) {
	t.Parallel()

	s := &Service{}
	items, err := s.decodePersistedTranscriptItems(
		`{"type":"handoff","id":"cut-1","timestamp":"2026-09-09_16-04-47","handoffPath":"thoughts/agents/bot/sessions/handoffs/2026-09-09_16-04-47.md","historyPath":"thoughts/agents/bot/sessions/history/2026-09-09_16-04-47.jsonl"}`,
		nil,
	)
	if err != nil {
		t.Fatalf("decodePersistedTranscriptItems() error = %v", err)
	}
	if len(items) != 1 {
		t.Fatalf("len(items) = %d, want 1", len(items))
	}
	got := items[0]
	if got.Variant != "detail" {
		t.Fatalf("Variant = %q, want detail", got.Variant)
	}
	if got.Title != "handoff" {
		t.Fatalf("Title = %q, want handoff", got.Title)
	}
	wantHref := "thoughts/agents/bot/sessions/handoffs/2026-09-09_16-04-47.md"
	if got.HeaderHref != wantHref {
		t.Fatalf("HeaderHref = %q, want %q", got.HeaderHref, wantHref)
	}
	wantHist := "thoughts/agents/bot/sessions/history/2026-09-09_16-04-47.jsonl"
	if got.SecondaryHref != wantHist {
		t.Fatalf("SecondaryHref = %q, want %q", got.SecondaryHref, wantHist)
	}

	html := testhelpers.RenderToDocument(t, TranscriptDetailCard(got)).Doc
	out, err := html.Html()
	if err != nil {
		t.Fatalf("Html() error = %v", err)
	}
	if !strings.Contains(out, wantHref) {
		t.Fatalf("rendered handoff missing href %q; html = %s", wantHref, out)
	}
}

func TestDecodePersistedTranscriptItems_CompactionIsCutCard(t *testing.T) {
	t.Parallel()

	s := &Service{}
	items, err := s.decodePersistedTranscriptItems(
		`{"type":"compaction","id":"cut-2","timestamp":"2026-09-09_16-04-47"}`,
		nil,
	)
	if err != nil {
		t.Fatalf("decodePersistedTranscriptItems() error = %v", err)
	}
	if len(items) != 1 {
		t.Fatalf("len(items) = %d, want 1", len(items))
	}
	if items[0].Variant != "detail" || items[0].Title != "compaction" {
		t.Fatalf("got %+v, want detail compaction card", items[0])
	}
}

func TestDecodePersistedTranscriptItems_UserEmailAttribution(t *testing.T) {
	t.Parallel()

	s := &Service{}
	items, err := s.decodePersistedTranscriptItems(
		`{"type":"message","id":"u1","message":{"role":"user","content":"hello","userEmail":"ada@example.com"}}`,
		nil,
	)
	if err != nil {
		t.Fatalf("decodePersistedTranscriptItems() error = %v", err)
	}
	if len(items) != 1 {
		t.Fatalf("len(items) = %d, want 1", len(items))
	}
	if items[0].Role != "user" {
		t.Fatalf("Role = %q, want user", items[0].Role)
	}
	if items[0].AuthorName != "ada@example.com" {
		t.Fatalf("AuthorName = %q, want ada@example.com", items[0].AuthorName)
	}

	html := testhelpers.RenderToDocument(
		t,
		ChatMessage(chatMessageArgsFromTranscript(items[0])),
	).Doc
	out, err := html.Html()
	if err != nil {
		t.Fatalf("Html() error = %v", err)
	}
	if !strings.Contains(out, "ada@example.com") {
		t.Fatalf("user bubble missing email; html = %s", out)
	}
	if !strings.Contains(out, `data-role="user"`) {
		t.Fatalf("user bubble lost user role; html = %s", out)
	}
}

func TestDecodePersistedTranscriptItems_InboundA2AUsesSenderSlug(t *testing.T) {
	t.Parallel()

	s := &Service{}
	items, err := s.decodePersistedTranscriptItems(
		`{"type":"message","id":"a1","fromAgentSlug":"infra-bot","message":{"role":"user","content":"ping from bot"}}`,
		nil,
	)
	if err != nil {
		t.Fatalf("decodePersistedTranscriptItems() error = %v", err)
	}
	if len(items) != 1 {
		t.Fatalf("len(items) = %d, want 1", len(items))
	}
	got := items[0]
	if got.Role == "user" {
		t.Fatalf("inbound A2A Role = user, want agent chrome")
	}
	if got.AuthorName != "infra-bot" {
		t.Fatalf("AuthorName = %q, want infra-bot", got.AuthorName)
	}

	html := testhelpers.RenderToDocument(
		t,
		ChatMessage(chatMessageArgsFromTranscript(got)),
	).Doc
	out, err := html.Html()
	if err != nil {
		t.Fatalf("Html() error = %v", err)
	}
	if !strings.Contains(out, "infra-bot") {
		t.Fatalf("inbound A2A missing sender slug; html = %s", out)
	}
	if strings.Contains(out, `data-role="user"`) {
		t.Fatalf("inbound A2A looks like a human user bubble; html = %s", out)
	}
}

func TestDecodePersistedTranscriptItems_MemoryWriteIsDetail(t *testing.T) {
	t.Parallel()

	s := &Service{}
	items, err := s.decodePersistedTranscriptItems(
		`{"type":"memory_write","id":"m1"}`,
		nil,
	)
	if err != nil {
		t.Fatalf("decodePersistedTranscriptItems() error = %v", err)
	}
	if len(items) != 1 || items[0].Variant != "detail" ||
		items[0].Title != "memory_write" {
		t.Fatalf("got %+v, want memory_write detail card", items)
	}
}

func TestTranscriptArtifactHrefRejectsAbsolute(t *testing.T) {
	t.Parallel()
	if got := transcriptArtifactHref("/etc/passwd"); got != "" {
		t.Fatalf("absolute path leaked: %q", got)
	}
	if got := transcriptArtifactHref("https://example.invalid/x"); got != "" {
		t.Fatalf("url leaked: %q", got)
	}
}

func TestLiveCutTranscriptItems_Handoff(t *testing.T) {
	t.Parallel()

	s := &Service{}
	items := s.liveCutTranscriptItems("live-cut", "live-cut", conversation.LiveTurnItem{
		Kind: conversation.LiveTurnHandoff,
		MessageJSON: []byte(
			`{"timestamp":"2026-09-09_16-04-47","handoffPath":"thoughts/a2a/a__b/sessions/handoffs/2026-09-09_16-04-47.md"}`,
		),
	})
	if len(items) != 1 || items[0].Variant != "detail" || items[0].Title != "handoff" {
		t.Fatalf("got %+v", items)
	}
	if !strings.HasPrefix(items[0].HeaderHref, "thoughts/") {
		t.Fatalf("HeaderHref = %q, want thoughts-relative", items[0].HeaderHref)
	}
}
