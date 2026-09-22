package markdown

import (
	"bytes"
	"context"
	"database/sql"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/labstack/echo/v4"

	"github.com/CoreyCole/vamos/pkg/agents/roster"
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
	mustWriteFile(t, filepath.Join(plan, "AGENTS.md"), []byte("# Agents\n"))
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
	if renderer.lastEnsureDoc != "" {
		t.Fatalf("GET must not ensure, doc = %q", renderer.lastEnsureDoc)
	}
	if renderer.chatThreadID != "" {
		t.Fatalf("GET must not auto click-in, got %q", renderer.chatThreadID)
	}
	body := rec.Body.String()
	for _, want := range []string{
		`id="agent-chat-composer"`,
		"Real plan",
		"design.md",
		"AGENTS.md",
		`name="attached_paths[]"`,
		`/rooms/plan/real-plan?artifact=thoughts%2Fcreative-mode-agent%2Fplans%2Freal-plan%2FAGENTS.md`,
	} {
		if !strings.Contains(body, want) {
			t.Fatalf("missing %q: %s", want, body)
		}
	}
	if strings.Contains(body, `/threads?artifact=`) {
		t.Fatalf("Files href bounced to pick-scope /threads: %s", body)
	}
	if strings.Contains(body, `id="thread-chat"`) {
		t.Fatalf("must not render SharedThreadChat: %s", body)
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
	if renderer.lastEnsureDoc != "" {
		t.Fatalf(
			"GET must not EnsureSharedThreadForDoc, doc = %q",
			renderer.lastEnsureDoc,
		)
	}
	if renderer.chatThreadID != "" {
		t.Fatalf("GET must not auto click-in, got %q", renderer.chatThreadID)
	}
	body := rec.Body.String()
	if !strings.Contains(body, `id="agent-chat-composer"`) {
		t.Fatalf("missing N=0 composer: %s", body)
	}
	if strings.Contains(body, `id="thread-chat"`) {
		t.Fatalf("must not render SharedThreadChat: %s", body)
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

func TestServeAI470RoomKnownBotListsHomeThread(t *testing.T) {
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
	writeBotThreadUserMessage(t, dbSvc.Queries, root, "nova", threadID)
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
	if renderer.chatThreadID != "" {
		t.Fatalf(
			"GET must not auto click-in SharedThreadChat, got %q",
			renderer.chatThreadID,
		)
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
		`id="thread-chat"`,
		"/thoughts/chat/freeform/send",
		`id="workbench-mobile-tabs"`,
		`aria-label="Workbench regions"`,
	} {
		if strings.Contains(body, bad) {
			t.Fatalf("body contains %q", bad)
		}
	}
	for _, want := range []string{
		`id="scoped-thread-list"`,
		`href="/threads/` + threadID + `"`,
		"Nova",
		`id="roster-row-dm-nova"`,
	} {
		if !strings.Contains(body, want) {
			t.Fatalf("missing %q: %s", want, body)
		}
	}
}

func TestServeAI470RoomEmptyHomeNotListRow(t *testing.T) {
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
	svc.WithWorkbenchThreadRenderer(&threadWorkbenchTestRenderer{})
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
	body := rec.Body.String()
	if strings.Contains(body, `id="scoped-thread-list"`) {
		t.Fatal("empty home must not list")
	}
	if strings.Contains(body, `/threads/`+threadID) {
		t.Fatal("empty home listed as row")
	}
	if !strings.Contains(body, `id="agent-chat-composer"`) {
		t.Fatalf("want N=0 composer: %s", body)
	}
}

func TestServeAI470RoomBotZeroThreadsComposerDoesNotInsert(t *testing.T) {
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
	if _, err := svc.roster.Create(roster.Bot{Slug: "nova", Name: "Nova"}); err != nil {
		t.Fatal(err)
	}
	svc.WithWorkbenchThreadRenderer(&threadWorkbenchTestRenderer{})
	before, err := dbSvc.Queries.ListAgentThreadsByAgentSlug(
		context.Background(),
		sql.NullString{String: "nova", Valid: true},
	)
	if err != nil {
		t.Fatal(err)
	}
	if len(before) != 0 {
		t.Fatalf("precondition threads = %d", len(before))
	}
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
	after, err := dbSvc.Queries.ListAgentThreadsByAgentSlug(
		context.Background(),
		sql.NullString{String: "nova", Valid: true},
	)
	if err != nil {
		t.Fatal(err)
	}
	if len(after) != 0 {
		t.Fatalf("GET inserted %d threads", len(after))
	}
	body := rec.Body.String()
	if !strings.Contains(body, `id="agent-chat-composer"`) {
		t.Fatalf("missing composer: %s", body)
	}
	if strings.Contains(body, "/thoughts/chat/freeform/send") {
		t.Fatal("N=0 composer must not wire freeform send")
	}
	if strings.Contains(body, `id="thread-chat"`) {
		t.Fatal("N=0 must not render SharedThreadChat")
	}
	if strings.Contains(body, "Freeform chat") {
		t.Fatal("bot N=0 Mode must not be Freeform chat")
	}
	if !strings.Contains(body, ">agent<") {
		t.Fatalf("bot N=0 Mode must be agent: %s", body)
	}
}

func TestServeAI470RoomBotTwoThreadsListsBoth(t *testing.T) {
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
	nova, err := svc.createAgent(context.Background(), createAgentInput{
		Slug: "nova", Name: "Nova", UserEmail: "t@example.com",
	})
	if err != nil {
		t.Fatal(err)
	}
	first, err := svc.ensureBotHomeThread(context.Background(), nova, "t@example.com")
	if err != nil {
		t.Fatal(err)
	}
	writeBotThreadUserMessage(t, dbSvc.Queries, root, "nova", first)
	second := uuid.NewString()
	if _, err := dbSvc.Queries.CreateAgentThread(
		context.Background(),
		db.CreateAgentThreadParams{
			ID:          second,
			UserEmail:   "t@example.com",
			Title:       "Second",
			Cwd:         filepath.Join(root, "agents", "nova"),
			LineageID:   uuid.NewString(),
			PiSessionID: uuid.NewString(),
		},
	); err != nil {
		t.Fatal(err)
	}
	if err := dbSvc.Queries.BindAgentThreadBotHome(
		context.Background(),
		db.BindAgentThreadBotHomeParams{
			AgentSlug: sql.NullString{String: "nova", Valid: true},
			Cwd:       filepath.Join(root, "agents", "nova"),
			Title:     "Second",
			ID:        second,
		},
	); err != nil {
		t.Fatal(err)
	}
	writeBotThreadUserMessage(t, dbSvc.Queries, root, "nova", second)
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
	if renderer.chatThreadID != "" {
		t.Fatalf("N=2 must not auto click-in, got %q", renderer.chatThreadID)
	}
	body := rec.Body.String()
	for _, want := range []string{
		`id="scoped-thread-list"`,
		`id="agent-chat-composer"`,
		`/rooms/dm/nova/threads`,
		`href="/threads/` + first + `"`,
		`href="/threads/` + second + `"`,
	} {
		if !strings.Contains(body, want) {
			t.Fatalf("missing %q: %s", want, body)
		}
	}
	if strings.Contains(body, `id="thread-chat"`) {
		t.Fatal("N=2 must not render SharedThreadChat")
	}
}

func TestServeAI470RoomLegacyHomeStaysListedAfterMigrate(t *testing.T) {
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
	nova, err := svc.createAgent(context.Background(), createAgentInput{
		Slug: "nova", Name: "Nova", UserEmail: "t@example.com",
	})
	if err != nil {
		t.Fatal(err)
	}
	threadID, err := svc.ensureBotHomeThread(
		context.Background(), nova, "t@example.com",
	)
	if err != nil {
		t.Fatal(err)
	}
	writeBotThreadUserMessage(t, dbSvc.Queries, root, "nova", threadID)
	svc.WithWorkbenchThreadRenderer(&threadWorkbenchTestRenderer{})
	get := func() string {
		t.Helper()
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
		return rec.Body.String()
	}
	first := get()
	if !strings.Contains(first, `href="/threads/`+threadID+`"`) {
		t.Fatalf("first GET missing row: %s", first)
	}
	row, err := dbSvc.Queries.GetAgentThread(context.Background(), threadID)
	if err != nil {
		t.Fatal(err)
	}
	if strings.TrimSpace(row.PiSessionID) == "" {
		t.Fatal("GET list did not persist pi_session_id after migrate")
	}
	second := get()
	if !strings.Contains(second, `href="/threads/`+threadID+`"`) {
		t.Fatalf("second GET dropped migrated row: %s", second)
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

func TestServeAI470RoomHonorsArtifactClosedCookieOnPlanViewChat(t *testing.T) {
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
	if !strings.Contains(window, `visible&#34;:false`) &&
		!strings.Contains(window, `"visible":false`) {
		t.Fatalf("plan View Chat must honor cookie=0: %s", window)
	}
	if !strings.Contains(body, `id="agent-chat-composer"`) {
		t.Fatalf("missing N=0 composer: %s", body)
	}
	if strings.Contains(body, `id="thread-chat"`) {
		t.Fatal("plan GET must not auto click-in")
	}
	for _, ck := range rec.Result().Cookies() {
		if ck.Name == "wb2_artifact_open" && ck.Value == "1" {
			t.Fatalf(
				"must not clobber cookie=0 with Set-Cookie 1, got %#v",
				rec.Result().Cookies(),
			)
		}
	}
}

func TestServeAI470RoomHonorsChatOpenCookieZero(t *testing.T) {
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
	req.AddCookie(&http.Cookie{Name: "wb2_chat_open", Value: "0"})
	rec := httptest.NewRecorder()
	c := echo.New().NewContext(req, rec)
	c.SetParamNames("kind", "id")
	c.SetParamValues("plan", "real-plan")
	c.Set("user_email", "t@example.com")
	if err := svc.ServeAI470Room(c); err != nil {
		t.Fatal(err)
	}
	body := rec.Body.String()
	needle := `&#34;workbenchV2Chat&#34;:{&#34;ratio&#34;`
	idx := strings.Index(body, needle)
	if idx < 0 {
		idx = strings.Index(body, `"workbenchV2Chat":{"ratio"`)
	}
	if idx < 0 {
		t.Fatalf("missing chat region signal: %s", body)
	}
	end := idx + 80
	if end > len(body) {
		end = len(body)
	}
	window := body[idx:end]
	if !strings.Contains(window, `visible&#34;:false`) &&
		!strings.Contains(window, `"visible":false`) {
		t.Fatalf("rooms GET must keep chat closed when cookie=0: %s", window)
	}
	if !strings.Contains(body, `id="workbench-v2-threads-reopen"`) {
		t.Fatalf("minimized chat must keep path-header hamburger: %s", body)
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

func TestServeAI470RoomDocsIndexHTMLCommentsPanel(t *testing.T) {
	t.Parallel()
	root := t.TempDir()
	docs := filepath.Join(root, "docs", "vamos")
	mustMkdirAll(t, docs)
	mustWriteFile(t, filepath.Join(docs, "AGENTS.md"), []byte("# Vamos docs\n"))
	mustWriteFile(
		t,
		filepath.Join(docs, "index.html"),
		[]byte("<html><body>docs index</body></html>"),
	)
	svc, err := NewService(root, nil, nil)
	if err != nil {
		t.Fatal(err)
	}
	svc.WithWorkbenchThreadRenderer(&threadWorkbenchTestRenderer{
		findID:        "thread-docs",
		threadPlanDir: "docs/vamos",
	})
	artifact := "thoughts/docs/vamos/index.html"
	rec := httptest.NewRecorder()
	c := echo.New().NewContext(
		httptest.NewRequest(
			http.MethodGet,
			"/rooms/plan/docs--vamos?artifact="+url.QueryEscape(artifact),
			http.NoBody,
		),
		rec,
	)
	c.SetParamNames("kind", "id")
	c.SetParamValues("plan", "docs--vamos")
	c.Set("user_email", "t@example.com")
	if err := svc.ServeAI470Room(c); err != nil {
		t.Fatal(err)
	}
	body := rec.Body.String()
	for _, unwanted := range []string{
		"Comments are unavailable for this artifact.",
		"Comments are unavailable for directories.",
		"Select an artifact to view comments.",
	} {
		if strings.Contains(body, unwanted) {
			t.Fatalf("comments unavailable: %q in %s", unwanted, body)
		}
	}
	if !strings.Contains(body, `id="comments-context-panel"`) {
		t.Fatalf("missing comments panel: %s", body)
	}
	if strings.Contains(body, `id="thread-chat"`) {
		t.Fatal("docs land must not auto click-in")
	}
	if !strings.Contains(body, `id="roster-row-doc-vamos"`) {
		t.Fatalf("missing docs band: %s", body)
	}
	if !strings.Contains(body, "roster-row-selected") {
		t.Fatalf("docs band not selected: %s", body)
	}
}

func TestServeAI470RoomPlanZeroThreadsDoesNotInsert(t *testing.T) {
	t.Parallel()
	root := t.TempDir()
	plan := filepath.Join(root, "owner-a", "plans", "plan-one")
	mustMkdirAll(t, plan)
	mustWriteFile(t, filepath.Join(plan, "design.md"), []byte("# Plan\n"))
	dbSvc, err := servicedb.NewService(
		filepath.Join(t.TempDir(), "plan-zero.db"),
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
	before, err := dbSvc.Queries.ListAgentThreadsByPlanDirRel(
		context.Background(),
		sql.NullString{String: "owner-a/plans/plan-one", Valid: true},
	)
	if err != nil {
		t.Fatal(err)
	}
	if len(before) != 0 {
		t.Fatalf("precondition threads = %d", len(before))
	}
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
	after, err := dbSvc.Queries.ListAgentThreadsByPlanDirRel(
		context.Background(),
		sql.NullString{String: "owner-a/plans/plan-one", Valid: true},
	)
	if err != nil {
		t.Fatal(err)
	}
	if len(after) != 0 {
		t.Fatalf("GET inserted %d threads", len(after))
	}
	body := rec.Body.String()
	if !strings.Contains(body, `id="agent-chat-composer"`) {
		t.Fatalf("missing composer: %s", body)
	}
	if strings.Contains(body, "/thoughts/chat/freeform/send") {
		t.Fatal("N=0 composer must not wire freeform send")
	}
	if strings.Contains(body, "Freeform chat") {
		t.Fatal("plan N=0 Mode must not be Freeform chat")
	}
	if !strings.Contains(body, ">plan<") {
		t.Fatalf("plan N=0 Mode must be plan: %s", body)
	}
}

func TestServeAI470RoomPlanListOmitsUnattachedQRSPI(t *testing.T) {
	t.Parallel()
	root := t.TempDir()
	plan := filepath.Join(root, "owner-a", "plans", "plan-one")
	mustMkdirAll(t, plan)
	mustWriteFile(t, filepath.Join(plan, "design.md"), []byte("# Plan\n"))
	dbSvc, err := servicedb.NewService(
		filepath.Join(t.TempDir(), "plan-list.db"),
		filepath.Join(t.TempDir(), "agents.yml"),
	)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = dbSvc.Close() })
	ctx := context.Background()
	if _, err := dbSvc.Queries.UpsertDiscoveredPlanWorkspace(
		ctx,
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
	threadID := uuid.NewString()
	piID := uuid.NewString()
	if _, err := dbSvc.Queries.CreateAgentThread(ctx, db.CreateAgentThreadParams{
		ID:          threadID,
		UserEmail:   "t@example.com",
		Title:       "Plan chat",
		Cwd:         "thoughts/owner-a/plans/plan-one",
		LineageID:   uuid.NewString(),
		PlanDirRel:  sql.NullString{String: "owner-a/plans/plan-one", Valid: true},
		PiSessionID: piID,
	}); err != nil {
		t.Fatal(err)
	}
	piPath := filepath.Join(plan, ".vamos", "sessions", "pi", piID+".jsonl")
	if err := os.MkdirAll(filepath.Dir(piPath), 0o755); err != nil {
		t.Fatal(err)
	}
	bodyJSONL := `{"type":"session","id":"` + piID + `"}` + "\n" +
		`{"type":"message","message":{"role":"user","content":"plan hi"}}` + "\n"
	if err := os.WriteFile(piPath, []byte(bodyJSONL), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := dbSvc.Queries.BindAgentThreadPlan(ctx, db.BindAgentThreadPlanParams{
		Cwd:   "thoughts/owner-a/plans/plan-one",
		Title: "Plan chat",
		ID:    threadID,
	}); err != nil {
		t.Fatal(err)
	}
	if _, err := dbSvc.Queries.UpsertAgentSessionIndex(
		ctx,
		db.UpsertAgentSessionIndexParams{
			ID:           uuid.NewString(),
			IdentityKind: "plan_owned",
			ArtifactPath: sql.NullString{
				String: "owner-a/plans/plan-one/.vamos/sessions/hermes/q.jsonl",
				Valid:  true,
			},
			PlanDir: sql.NullString{
				String: "owner-a/plans/plan-one",
				Valid:  true,
			},
			Agent:             "hermes",
			FileSize:          1,
			ProjectionState:   "hydrated",
			ProjectedThreadID: sql.NullString{},
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
	for _, want := range []string{
		`id="scoped-thread-list"`,
		`id="agent-chat-composer"`,
		`/rooms/plan/plan-one/threads`,
		`href="/threads/` + threadID + `"`,
	} {
		if !strings.Contains(body, want) {
			t.Fatalf("missing %q: %s", want, body)
		}
	}
	if strings.Contains(body, "hermes/q.jsonl") {
		t.Fatal("unattached QRSPI session leaked into list")
	}
	if strings.Contains(body, `id="thread-chat"`) {
		t.Fatal("N=1 must still show list, not SharedThreadChat")
	}
}

func writeBotThreadUserMessage(
	t *testing.T,
	q db.Querier,
	root, slug, threadID string,
) {
	t.Helper()
	thread, err := q.GetAgentThread(context.Background(), threadID)
	if err != nil {
		t.Fatal(err)
	}
	piID := strings.TrimSpace(thread.PiSessionID)
	var piPath string
	if piID != "" {
		piPath = filepath.Join(root, "agents", slug, "sessions", "pi", piID+".jsonl")
	} else {
		piID = threadID
		piPath = filepath.Join(root, "agents", slug, "sessions", "current.jsonl")
	}
	if err := os.MkdirAll(filepath.Dir(piPath), 0o755); err != nil {
		t.Fatal(err)
	}
	existing, _ := os.ReadFile(piPath)
	if len(existing) == 0 {
		existing = []byte(`{"type":"session","id":"` + piID + `"}` + "\n")
	}
	userLine := `{"type":"message","message":{"role":"user","content":"hello"}}` + "\n"
	if err := os.WriteFile(
		piPath,
		append(existing, []byte(userLine)...),
		0o644,
	); err != nil {
		t.Fatal(err)
	}
}
