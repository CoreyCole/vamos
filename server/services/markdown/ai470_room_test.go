package markdown

import (
	"context"
	"net/http"
	"net/http/httptest"
	"net/url"
	"path/filepath"
	"strings"
	"testing"

	"github.com/labstack/echo/v4"

	"github.com/CoreyCole/vamos/server/services/agenthome"
	servicedb "github.com/CoreyCole/vamos/server/services/db"
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

func TestServeAI470Room404sOutOfV1Kinds(t *testing.T) {
	t.Parallel()
	svc, err := NewService(t.TempDir(), nil, nil)
	if err != nil {
		t.Fatal(err)
	}
	svc.WithWorkbenchThreadRenderer(&threadWorkbenchTestRenderer{findID: "other"})
	for _, tc := range []struct {
		kind, id string
	}{
		{"group", "vamos-dev"},
		{"agent_dm", "pair"},
	} {
		rec := httptest.NewRecorder()
		c := echo.New().NewContext(
			httptest.NewRequest(
				http.MethodGet,
				"/rooms/"+tc.kind+"/"+tc.id,
				http.NoBody,
			),
			rec,
		)
		c.SetParamNames("kind", "id")
		c.SetParamValues(tc.kind, tc.id)
		err := svc.ServeAI470Room(c)
		httpErr, ok := err.(*echo.HTTPError)
		if !ok || httpErr.Code != http.StatusNotFound {
			t.Fatalf("%s/%s err = %v", tc.kind, tc.id, err)
		}
	}
}

func TestServeAI470RoomMissingBotSlug404s(t *testing.T) {
	t.Parallel()
	dbSvc, err := servicedb.NewService(filepath.Join(t.TempDir(), "agents.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = dbSvc.Close() })
	svc, err := NewService(t.TempDir(), nil, nil)
	if err != nil {
		t.Fatal(err)
	}
	svc.WithQueries(dbSvc.Queries)
	svc.WithWorkbenchThreadRenderer(&threadWorkbenchTestRenderer{})
	rec := httptest.NewRecorder()
	c := echo.New().NewContext(
		httptest.NewRequest(http.MethodGet, "/rooms/dm/missing", http.NoBody),
		rec,
	)
	c.SetParamNames("kind", "id")
	c.SetParamValues("dm", "missing")
	err = svc.ServeAI470Room(c)
	httpErr, ok := err.(*echo.HTTPError)
	if !ok || httpErr.Code != http.StatusNotFound {
		t.Fatalf("err = %v", err)
	}
}

func TestServeAI470RoomKnownBotHydratesHomeThread(t *testing.T) {
	t.Parallel()
	root := t.TempDir()
	mustMkdirAll(t, filepath.Join(root, "owner", "plans", "alpha"))
	dbSvc, err := servicedb.NewService(filepath.Join(t.TempDir(), "agents.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = dbSvc.Close() })
	svc, err := NewService(root, nil, nil)
	if err != nil {
		t.Fatal(err)
	}
	svc.WithQueries(dbSvc.Queries)
	nova, err := svc.createAgent(context.Background(), createAgentInput{
		Slug: "nova", Name: "Nova", UserEmail: "t@example.com",
	})
	if err != nil {
		t.Fatal(err)
	}
	threadID, err := svc.ensureBotHomeThread(context.Background(), nova, "t@example.com")
	if err != nil {
		t.Fatal(err)
	}
	renderer := &threadWorkbenchTestRenderer{}
	svc.WithWorkbenchThreadRenderer(renderer)
	rec := httptest.NewRecorder()
	c := echo.New().NewContext(
		httptest.NewRequest(http.MethodGet, "/rooms/dm/nova", http.NoBody),
		rec,
	)
	c.SetParamNames("kind", "id")
	c.SetParamValues("dm", "nova")
	c.Set("user_email", "t@example.com")
	if err := svc.ServeAI470Room(c); err != nil {
		t.Fatal(err)
	}
	if renderer.chatThreadID != threadID {
		t.Fatalf("chat thread = %q want %q", renderer.chatThreadID, threadID)
	}
	body := rec.Body.String()
	for _, bad := range []string{
		"No shared thread mapped for this room yet.",
		"Select a thread to open chat.",
		"Message Bot",
		"Message Vamos dev",
	} {
		if strings.Contains(body, bad) {
			t.Fatalf("body contains %q", bad)
		}
	}
	for _, want := range []string{`id="thread-chat"`, "Nova"} {
		if !strings.Contains(body, want) {
			t.Fatalf("missing %q: %s", want, body)
		}
	}
}

func TestAI470RoomTitleUsesLiveAgentName(t *testing.T) {
	t.Parallel()
	dbSvc, err := servicedb.NewService(filepath.Join(t.TempDir(), "agents.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = dbSvc.Close() })
	svc, err := NewService(t.TempDir(), nil, nil)
	if err != nil {
		t.Fatal(err)
	}
	svc.WithQueries(dbSvc.Queries)
	if _, err := svc.createAgent(context.Background(), createAgentInput{
		Slug: "nova", Name: "Nova", UserEmail: "t@example.com",
	}); err != nil {
		t.Fatal(err)
	}
	got := svc.ai470RoomTitle(context.Background(), agenthome.KindDM, "nova")
	if got != "Nova" {
		t.Fatalf("title = %q", got)
	}
	if got := svc.ai470RoomTitle(
		context.Background(),
		agenthome.KindPlan,
		"alpha",
	); got != "alpha" {
		t.Fatalf("plan title fixture leftover = %q", got)
	}
}
