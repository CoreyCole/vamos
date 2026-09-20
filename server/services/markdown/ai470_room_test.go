package markdown

import (
	"bytes"
	"context"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/labstack/echo/v4"

	"github.com/CoreyCole/vamos/pkg/db"
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

func TestServeAI470RoomOpensPlanDesignWithoutArtifactQuery(t *testing.T) {
	t.Parallel()

	root := t.TempDir()
	plan := filepath.Join(root, "owner-a", "plans", "plan-one")
	mustMkdirAll(t, plan)
	heading := "Unique live plan design heading 3-2"
	mustWriteFile(t, filepath.Join(plan, "design.md"), []byte("# "+heading+"\n"))

	dbSvc, err := servicedb.NewService(
		filepath.Join(t.TempDir(), "plan-room.db"),
		filepath.Join(t.TempDir(), "agents.yml"),
	)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = dbSvc.Close() })
	if _, err := dbSvc.Queries.UpsertDiscoveredPlanWorkspace(
		context.Background(),
		db.UpsertDiscoveredPlanWorkspaceParams{
			PlanDirRel:     "owner-a/plans/plan-one",
			ProjectID:      "vamos",
			PlanDir:        "thoughts/owner-a/plans/plan-one",
			Label:          "Plan One",
			QrspiLifecycle: "design",
		},
	); err != nil {
		t.Fatal(err)
	}

	svc, err := NewService(root, nil, nil)
	if err != nil {
		t.Fatal(err)
	}
	svc.WithQueries(dbSvc.Queries)
	svc.WithWorkbenchThreadRenderer(&threadWorkbenchTestRenderer{})

	rec := httptest.NewRecorder()
	c := echo.New().NewContext(
		httptest.NewRequest(http.MethodGet, "/rooms/plan/plan-one", http.NoBody),
		rec,
	)
	c.SetParamNames("kind", "id")
	c.SetParamValues("plan", "plan-one")
	c.Set("user_email", "t@example.com")
	if err := svc.ServeAI470Room(c); err != nil {
		t.Fatal(err)
	}
	body := rec.Body.String()
	if !strings.Contains(body, heading) {
		t.Fatalf("missing live design heading %q: %s", heading, body)
	}
	if strings.Contains(body, "Select a thread to view an artifact.") {
		t.Fatalf("blank artifact pane: %s", body)
	}
	for _, want := range []string{
		`id="workbench-mobile-chat-comments"`,
		`data-testid="mobile-toggle-threads"`,
	} {
		if !strings.Contains(body, want) {
			t.Fatalf("plan artifact missing mobile chrome %q: %s", want, body)
		}
	}
	for _, refuse := range []string{
		`id="workbench-mobile-tabs"`,
		`aria-label="Workbench regions"`,
		`role="tablist"`,
	} {
		if strings.Contains(body, refuse) {
			t.Fatalf("plan artifact extra mobile chrome %q: %s", refuse, body)
		}
	}
	if !strings.Contains(body, `data-testid="view-chat"`) {
		t.Fatalf("missing artifact-header chat toggle: %s", body)
	}
}

func TestServeAI470RoomFallsBackToAgentsMd(t *testing.T) {
	t.Parallel()

	root := t.TempDir()
	plan := filepath.Join(root, "owner-a", "plans", "agents-only")
	mustMkdirAll(t, plan)
	heading := "Unique live plan agents heading"
	mustWriteFile(t, filepath.Join(plan, "AGENTS.md"), []byte("# "+heading+"\n"))

	dbSvc, err := servicedb.NewService(
		filepath.Join(t.TempDir(), "agents-plan.db"),
		filepath.Join(t.TempDir(), "agents.yml"),
	)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = dbSvc.Close() })
	if _, err := dbSvc.Queries.UpsertDiscoveredPlanWorkspace(
		context.Background(),
		db.UpsertDiscoveredPlanWorkspaceParams{
			PlanDirRel:     "owner-a/plans/agents-only",
			ProjectID:      "vamos",
			PlanDir:        "thoughts/owner-a/plans/agents-only",
			Label:          "Agents Only",
			QrspiLifecycle: "design",
		},
	); err != nil {
		t.Fatal(err)
	}

	svc, err := NewService(root, nil, nil)
	if err != nil {
		t.Fatal(err)
	}
	svc.WithQueries(dbSvc.Queries)
	svc.WithWorkbenchThreadRenderer(&threadWorkbenchTestRenderer{})

	rec := httptest.NewRecorder()
	c := echo.New().NewContext(
		httptest.NewRequest(http.MethodGet, "/rooms/plan/agents-only", http.NoBody),
		rec,
	)
	c.SetParamNames("kind", "id")
	c.SetParamValues("plan", "agents-only")
	c.Set("user_email", "t@example.com")
	if err := svc.ServeAI470Room(c); err != nil {
		t.Fatal(err)
	}
	body := rec.Body.String()
	if !strings.Contains(body, heading) {
		t.Fatalf("missing AGENTS.md heading %q: %s", heading, body)
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
	dbSvc, err := servicedb.NewService(
		filepath.Join(t.TempDir(), "agents.db"),
		filepath.Join(t.TempDir(), "agents.yml"),
	)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = dbSvc.Close() })
	svc, err := NewService(t.TempDir(), nil, nil)
	if err != nil {
		t.Fatal(err)
	}
	svc.WithQueries(dbSvc.Queries)
	withTestRoster(t, svc)
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
	dbSvc, err := servicedb.NewService(
		filepath.Join(t.TempDir(), "agents.db"),
		filepath.Join(t.TempDir(), "agents.yml"),
	)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = dbSvc.Close() })
	svc, err := NewService(root, nil, nil)
	if err != nil {
		t.Fatal(err)
	}
	svc.WithQueries(dbSvc.Queries)
	withTestRoster(t, svc)
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
		"workbench-v2-artifact-list",
		"$artPreview",
		"reply-draft.md",
		"onboarding-short.md",
		"AgentFixtureMessage",
		`id="workbench-mobile-tabs"`,
		`aria-label="Workbench regions"`,
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
	dbSvc, err := servicedb.NewService(
		filepath.Join(t.TempDir(), "agents.db"),
		filepath.Join(t.TempDir(), "agents.yml"),
	)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = dbSvc.Close() })
	svc, err := NewService(t.TempDir(), nil, nil)
	if err != nil {
		t.Fatal(err)
	}
	svc.WithQueries(dbSvc.Queries)
	withTestRoster(t, svc)
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
	if got := svc.ai470RoomTitle(
		context.Background(),
		agenthome.KindPlan,
		"docs--vamos",
	); got != "Docs / Vamos" {
		t.Fatalf("docs room title = %q", got)
	}
	if got := svc.ai470RoomTitle(
		context.Background(),
		agenthome.KindPlan,
		"docs--vamos--shots",
	); got != "Docs / Vamos / Shots" {
		t.Fatalf("nested docs room title = %q", got)
	}
	slug := "2026-09-08_10-10-54_agent-memory-observable-context"
	if got := svc.ai470RoomTitle(
		context.Background(),
		agenthome.KindPlan,
		slug,
	); got != slug {
		t.Fatalf("timestamp plan id should stay raw for header parse = %q", got)
	}
}

func TestServeAI470RoomForcesArtifactOpenOnPlanViewChat(t *testing.T) {
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
	req := httptest.NewRequest(
		http.MethodGet,
		"/rooms/plan/real-plan?artifact="+url.QueryEscape(artifact),
		http.NoBody,
	)
	req.Header.Set("X-Vamos-Viewport-Class", "desktop-full")
	req.AddCookie(&http.Cookie{Name: "wb2_artifact_open", Value: "0"})
	rec := httptest.NewRecorder()
	c := echo.New().NewContext(req, rec)
	c.SetParamNames("kind", "id")
	c.SetParamValues("plan", "real-plan")
	c.Set("user_email", "t@example.com")
	if err := svc.ServeAI470Room(c); err != nil {
		t.Fatal(err)
	}
	body := rec.Body.String()
	needle := `&#34;workbenchV2Artifact&#34;:{&#34;ratio&#34;`
	idx := strings.Index(body, needle)
	if idx < 0 {
		idx = strings.Index(body, `"workbenchV2Artifact":{"ratio"`)
	}
	if idx < 0 {
		t.Fatalf("missing artifact region signal: %s", body)
	}
	end := idx + 80
	if end > len(body) {
		end = len(body)
	}
	window := body[idx:end]
	if !strings.Contains(window, `visible&#34;:true`) &&
		!strings.Contains(window, `"visible":true`) {
		t.Fatalf("plan View Chat must keep artifact open despite cookie=0: %s", window)
	}
	if !strings.Contains(body, `id="thread-chat"`) {
		t.Fatalf("missing chat: %s", body)
	}
	found := false
	for _, ck := range rec.Result().Cookies() {
		if ck.Name == "wb2_artifact_open" && ck.Value == "1" {
			found = true
			break
		}
	}
	if !found {
		t.Fatalf(
			"expected Set-Cookie wb2_artifact_open=1, got %#v",
			rec.Result().Cookies(),
		)
	}
}

func TestLiveRosterListsPlanDirsFromIndex(t *testing.T) {
	t.Parallel()
	root := t.TempDir()
	planA := filepath.Join(root, "owner-a", "plans", "plan-one")
	planB := filepath.Join(root, "owner-b", "plans", "plan-two")
	mustMkdirAll(t, planA)
	mustMkdirAll(t, planB)
	mustWriteFile(t, filepath.Join(planA, "design.md"), []byte("# One\n"))
	mustWriteFile(t, filepath.Join(planB, "design.md"), []byte("# Two\n"))

	dbSvc, err := servicedb.NewService(
		filepath.Join(t.TempDir(), "plans.db"),
		filepath.Join(t.TempDir(), "agents.yml"),
	)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = dbSvc.Close() })
	ctx := context.Background()
	stampA := time.Date(2026, 9, 11, 13, 2, 0, 0, time.Local)
	stampB := time.Date(2026, 9, 10, 16, 5, 0, 0, time.Local)
	if _, err := dbSvc.Queries.UpsertDiscoveredPlanWorkspace(
		ctx,
		db.UpsertDiscoveredPlanWorkspaceParams{
			PlanDirRel:        "owner-a/plans/plan-one",
			ProjectID:         "vamos",
			PlanDir:           "thoughts/owner-a/plans/plan-one",
			Label:             "Plan One",
			ArtifactUpdatedAt: stampA,
			QrspiLifecycle:    "implement",
		},
	); err != nil {
		t.Fatal(err)
	}
	if _, err := dbSvc.Queries.UpsertDiscoveredPlanWorkspace(
		ctx,
		db.UpsertDiscoveredPlanWorkspaceParams{
			PlanDirRel:        "owner-b/plans/plan-two",
			ProjectID:         "vamos",
			PlanDir:           "thoughts/owner-b/plans/plan-two",
			Label:             "Plan Two",
			ArtifactUpdatedAt: stampB,
			QrspiLifecycle:    "design",
		},
	); err != nil {
		t.Fatal(err)
	}

	svc, err := NewService(root, nil, nil)
	if err != nil {
		t.Fatal(err)
	}
	svc.WithQueries(dbSvc.Queries)

	view := svc.liveRoster(ctx, agenthome.RosterSelection{})
	var buf bytes.Buffer
	if err := agenthome.RosterRail(view).Render(ctx, &buf); err != nil {
		t.Fatal(err)
	}
	html := buf.String()
	for _, want := range []string{
		"Plan One",
		"Plan Two",
		"design.md",
		"/rooms/plan/plan-one",
		"/rooms/plan/plan-two",
		"artifact=",
	} {
		if !strings.Contains(html, want) {
			t.Fatalf("missing %q in %s", want, html)
		}
	}
	if !strings.Contains(
		html,
		url.QueryEscape("thoughts/owner-a/plans/plan-one/design.md"),
	) &&
		!strings.Contains(html, "thoughts/owner-a/plans/plan-one/design.md") {
		t.Fatalf("missing plan-one design.md artifact query: %s", html)
	}
	if !strings.Contains(
		html,
		url.QueryEscape("thoughts/owner-b/plans/plan-two/design.md"),
	) &&
		!strings.Contains(html, "thoughts/owner-b/plans/plan-two/design.md") {
		t.Fatalf("missing plan-two design.md artifact query: %s", html)
	}
	if strings.Contains(html, `href="/rooms/plan/alpha"`) ||
		strings.Contains(html, ">Alpha<") {
		t.Fatal("Alpha fixture must not appear")
	}
	if !strings.Contains(html, rosterPlanTime(stampA)) {
		t.Fatalf("plan roster must show timestamp %q", rosterPlanTime(stampA))
	}
	if !strings.Contains(html, `data-roster-plan-swatch`) {
		t.Fatal("plan roster must render color swatch")
	}
}

func TestLiveRosterPlansEmptyIndexHasNoAlpha(t *testing.T) {
	t.Parallel()
	svc, err := NewService(t.TempDir(), nil, nil)
	if err != nil {
		t.Fatal(err)
	}
	dbSvc, err := servicedb.NewService(
		filepath.Join(t.TempDir(), "empty.db"),
		filepath.Join(t.TempDir(), "agents.yml"),
	)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = dbSvc.Close() })
	svc.WithQueries(dbSvc.Queries)
	view := svc.liveRoster(context.Background(), agenthome.RosterSelection{})
	if len(view.Plans) != 0 {
		t.Fatalf("empty index plans = %#v", view.Plans)
	}
	var buf bytes.Buffer
	if err := agenthome.RosterRail(view).Render(context.Background(), &buf); err != nil {
		t.Fatal(err)
	}
	html := buf.String()
	if strings.Contains(html, "Alpha") || strings.Contains(html, "/rooms/plan/alpha") {
		t.Fatalf("empty roster leaked Alpha: %s", html)
	}
}

func TestLiveRosterBotPreviewFromJSONL(t *testing.T) {
	t.Parallel()
	root := t.TempDir()
	dbSvc, err := servicedb.NewService(
		filepath.Join(t.TempDir(), "agents.db"),
		filepath.Join(t.TempDir(), "agents.yml"),
	)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = dbSvc.Close() })
	svc, err := NewService(root, nil, nil)
	if err != nil {
		t.Fatal(err)
	}
	svc.WithQueries(dbSvc.Queries)
	withTestRoster(t, svc)
	ctx := context.Background()
	if _, err := svc.createAgent(ctx, createAgentInput{
		Slug: "nova", Name: "Nova", UserEmail: "t@example.com",
	}); err != nil {
		t.Fatal(err)
	}
	if _, err := svc.createAgent(ctx, createAgentInput{
		Slug: "other", Name: "Other", UserEmail: "t@example.com",
	}); err != nil {
		t.Fatal(err)
	}
	sentence := "UNIQUE_PREVIEW_SENTENCE_willow_bamboo_4421"
	jsonl := filepath.Join(root, "agents", "nova", "sessions", "current.jsonl")
	body := `{"type":"message","id":"u1","message":{"role":"user","content":"` +
		sentence + `"}}` + "\n"
	if err := os.WriteFile(jsonl, []byte(body), 0o644); err != nil {
		t.Fatal(err)
	}
	view := svc.liveRoster(ctx, agenthome.RosterSelection{})
	var buf bytes.Buffer
	if err := agenthome.RosterRail(view).Render(ctx, &buf); err != nil {
		t.Fatal(err)
	}
	html := buf.String()
	if !strings.Contains(html, sentence) {
		t.Fatalf("roster missing preview sentence: %s", html)
	}
	n := strings.Count(html, sentence)
	if n != 1 {
		t.Fatalf("preview count = %d want 1", n)
	}
	novaIdx := strings.Index(html, `id="roster-row-dm-nova"`)
	otherIdx := strings.Index(html, `id="roster-row-dm-other"`)
	sentIdx := strings.Index(html, sentence)
	if novaIdx < 0 || otherIdx < 0 || sentIdx < 0 {
		t.Fatal("missing row ids")
	}
	if sentIdx < novaIdx || (otherIdx > novaIdx && sentIdx > otherIdx) {
		t.Fatal("preview must sit in nova row only")
	}
	if strings.Contains(html, "Ready when you are.") {
		t.Fatal("fixture subtitle leaked")
	}
}

func TestLiveRosterDocsPrefersIndexHTML(t *testing.T) {
	t.Parallel()
	root := t.TempDir()
	mustMkdirAll(t, filepath.Join(root, "docs", "vamos"))
	mustMkdirAll(t, filepath.Join(root, "docs", "chestnut"))
	mustWriteFile(
		t,
		filepath.Join(root, "docs", "vamos", "index.html"),
		[]byte("<html></html>"),
	)
	svc, err := NewService(root, nil, nil)
	if err != nil {
		t.Fatal(err)
	}
	docs := svc.liveRosterDocs()
	byID := map[string]agenthome.RosterDocRow{}
	for _, row := range docs {
		byID[row.ID] = row
	}
	if byID["vamos"].Href != "/thoughts/docs/vamos/index.html" {
		t.Fatalf("vamos href = %q", byID["vamos"].Href)
	}
	if byID["chestnut"].Href != "/thoughts/docs/chestnut/" {
		t.Fatalf("chestnut href = %q", byID["chestnut"].Href)
	}
}
