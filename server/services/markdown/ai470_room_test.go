package markdown

import (
	"net/http"
	"net/http/httptest"
	"net/url"
	"path/filepath"
	"strings"
	"testing"

	"github.com/labstack/echo/v4"

	"github.com/CoreyCole/vamos/server/services/agenthome"
)

func TestServeAI470RoomUsesArtifactPathForPlanChat(t *testing.T) {
	t.Parallel()

	root := t.TempDir()
	plan := filepath.Join(root, "creative-mode-agent", "plans", "real-plan")
	mustMkdirAll(t, plan)
	mustWriteFile(t, filepath.Join(plan, "design.md"), []byte("# Real plan\n"))
	svc, err := NewService(root, nil, nil)
	if err != nil {
		t.Fatal(err)
	}
	renderer := &threadWorkbenchTestRenderer{
		findID:        "thread-real",
		threadPlanDir: "thoughts/creative-mode-agent/plans/real-plan",
	}
	svc.WithWorkbenchThreadRenderer(renderer)

	artifact := "thoughts/creative-mode-agent/plans/real-plan/design.md"
	rec := httptest.NewRecorder()
	c := echo.New().NewContext(
		httptest.NewRequest(
			http.MethodGet,
			"/rooms/plan/real-plan?artifact="+url.QueryEscape(artifact),
			http.NoBody,
		),
		rec,
	)
	c.SetParamNames("kind", "id")
	c.SetParamValues("plan", "real-plan")
	c.Set("user_email", "t@example.com")
	if err := svc.ServeAI470Room(c); err != nil {
		t.Fatal(err)
	}
	if renderer.lastFindDoc != artifact {
		t.Fatalf("FindSharedThreadForDoc doc = %q", renderer.lastFindDoc)
	}
	if renderer.chatThreadID != "thread-real" {
		t.Fatalf("chat thread = %q", renderer.chatThreadID)
	}
	body := rec.Body.String()
	for _, want := range []string{
		`id="thread-chat"`,
		"Real plan",
	} {
		if !strings.Contains(body, want) {
			t.Fatalf("missing %q: %s", want, body)
		}
	}
	if strings.Contains(body, "Select a thread to view an artifact.") {
		t.Fatalf("blank artifact pane: %s", body)
	}
}

func TestServeAI470RoomEnsuresPlanThreadWhenMissing(t *testing.T) {
	t.Parallel()

	root := t.TempDir()
	plan := filepath.Join(root, "creative-mode-agent", "plans", "real-plan")
	mustMkdirAll(t, plan)
	mustWriteFile(t, filepath.Join(plan, "design.md"), []byte("# Real plan\n"))
	svc, err := NewService(root, nil, nil)
	if err != nil {
		t.Fatal(err)
	}
	renderer := &threadWorkbenchTestRenderer{
		ensureID:      "thread-new",
		threadPlanDir: "thoughts/creative-mode-agent/plans/real-plan",
	}
	svc.WithWorkbenchThreadRenderer(renderer)

	artifact := "thoughts/creative-mode-agent/plans/real-plan/design.md"
	rec := httptest.NewRecorder()
	c := echo.New().NewContext(
		httptest.NewRequest(
			http.MethodGet,
			"/rooms/plan/real-plan?artifact="+url.QueryEscape(artifact),
			http.NoBody,
		),
		rec,
	)
	c.SetParamNames("kind", "id")
	c.SetParamValues("plan", "real-plan")
	c.Set("user_email", "t@example.com")
	if err := svc.ServeAI470Room(c); err != nil {
		t.Fatal(err)
	}
	if renderer.lastEnsureDoc != artifact {
		t.Fatalf("EnsureSharedThreadForDoc doc = %q", renderer.lastEnsureDoc)
	}
	if renderer.chatThreadID != "thread-new" {
		t.Fatalf("chat thread = %q", renderer.chatThreadID)
	}
	body := rec.Body.String()
	if !strings.Contains(body, `id="thread-chat"`) {
		t.Fatalf("missing plan chat: %s", body)
	}
	if strings.Contains(body, "Select a thread to view an artifact.") {
		t.Fatalf("blank artifact pane: %s", body)
	}
}

func TestAI470RoomComposerDisabledDoesNotTreatKindAgentDMAsPairwise(t *testing.T) {
	t.Parallel()
	if AI470RoomComposerDisabled(agenthome.KindAgentDM, "pair") {
		t.Fatal("KindAgentDM must not disable composer as pairwise")
	}
	for _, kind := range []agenthome.RoomKind{
		agenthome.KindDM, agenthome.KindGroup, agenthome.KindPlan,
	} {
		if AI470RoomComposerDisabled(kind, "bot") {
			t.Fatalf("kind %s unexpectedly disables composer", kind)
		}
	}
	if !AI470PairwiseComposerDisabled() {
		t.Fatal("pairwise rooms must disable composer")
	}
	if !AI470RoomComposerDisabled(agenthome.KindA2A, "nova/research") {
		t.Fatal("KindA2A must disable composer")
	}
}
