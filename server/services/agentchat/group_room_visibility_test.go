package agentchat

import (
	"database/sql"
	"errors"
	"net/http"
	"net/http/httptest"
	"net/url"
	"path/filepath"
	"strings"
	"testing"

	"github.com/labstack/echo/v4"

	"github.com/CoreyCole/vamos/pkg/db"
	serverdb "github.com/CoreyCole/vamos/server/services/db"
)

func TestSharedRoomsVisibleAndComposableByOtherUsers(t *testing.T) {
	t.Parallel()
	database, err := serverdb.NewService(filepath.Join(t.TempDir(), "share.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = database.Close() })
	q := database.Queries
	ctx := t.Context()
	agent, err := q.CreateAgent(ctx, db.CreateAgentParams{
		ID: "agent-nova", Slug: "nova", Name: "Nova",
	})
	if err != nil {
		t.Fatal(err)
	}
	home, err := q.CreateAgentThread(ctx, db.CreateAgentThreadParams{
		ID:        "thread-home",
		UserEmail: "owner@example.com",
		Title:     "Nova home",
		Cwd:       "thoughts/agents/nova",
		LineageID: "lin-home",
	})
	if err != nil {
		t.Fatal(err)
	}
	if err := q.BindAgentThreadBotHome(ctx, db.BindAgentThreadBotHomeParams{
		AgentID: sql.NullString{String: agent.ID, Valid: true},
		Cwd:     home.Cwd,
		Title:   home.Title,
		ID:      home.ID,
	}); err != nil {
		t.Fatal(err)
	}

	got, err := q.GetAgentThreadForUser(ctx, db.GetAgentThreadForUserParams{
		ID:        home.ID,
		UserEmail: "viewer@example.com",
	})
	if err != nil {
		t.Fatalf("user B cannot open user A bot home: %v", err)
	}
	if got.ID != home.ID {
		t.Fatalf("opened %q", got.ID)
	}

	listed, err := q.ListAgentThreads(ctx, db.ListAgentThreadsParams{
		UserEmail: "viewer@example.com",
		Limit:     50,
	})
	if err != nil {
		t.Fatal(err)
	}
	found := false
	for _, row := range listed {
		if row.ID == home.ID {
			found = true
			break
		}
	}
	if !found {
		t.Fatal("sidebar list filtered by viewer email")
	}

	svc := &Service{db: database.DB(), queries: q}
	_, _, err = svc.ResumeThread(ctx, "viewer@example.com", home.ID, "hello from B")
	if err == nil {
		t.Fatal("expected temporal-not-configured after compose allowed")
	}
	if errors.Is(err, sql.ErrNoRows) || errors.Is(err, ErrPairwiseViewOnly) {
		t.Fatalf("user B compose rejected: %v", err)
	}
	if !strings.Contains(err.Error(), "temporal not configured") {
		t.Fatalf("unexpected compose error: %v", err)
	}
}

func TestPairwiseHumanComposeForbidden(t *testing.T) {
	t.Parallel()
	database, err := serverdb.NewService(filepath.Join(t.TempDir(), "pair.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = database.Close() })
	q := database.Queries
	ctx := t.Context()
	a, err := q.CreateAgent(
		ctx,
		db.CreateAgentParams{ID: "a", Slug: "alpha", Name: "Alpha"},
	)
	if err != nil {
		t.Fatal(err)
	}
	b, err := q.CreateAgent(
		ctx,
		db.CreateAgentParams{ID: "b", Slug: "beta", Name: "Beta"},
	)
	if err != nil {
		t.Fatal(err)
	}
	thread, err := q.CreateAgentThread(ctx, db.CreateAgentThreadParams{
		ID:        "thread-pair",
		UserEmail: "owner@example.com",
		Title:     "a2a",
		Cwd:       "thoughts/a2a/alpha__beta",
		LineageID: "lin-pair",
	})
	if err != nil {
		t.Fatal(err)
	}
	if err := q.BindAgentThreadPairwise(ctx, db.BindAgentThreadPairwiseParams{
		PairAgentIDA: sql.NullString{String: a.ID, Valid: true},
		PairAgentIDB: sql.NullString{String: b.ID, Valid: true},
		Cwd:          thread.Cwd,
		Title:        thread.Title,
		ID:           thread.ID,
	}); err != nil {
		t.Fatal(err)
	}

	svc := &Service{db: database.DB(), queries: q}
	handler := NewHandler(svc, nil)
	form := url.Values{}
	form.Set("prompt", "human should not speak here")
	req := httptest.NewRequest(
		http.MethodPost,
		"/agent-chat/thread/"+thread.ID+"/resume",
		strings.NewReader(form.Encode()),
	)
	req.Header.Set(echo.HeaderContentType, echo.MIMEApplicationForm)
	rec := httptest.NewRecorder()
	c := echo.New().NewContext(req, rec)
	c.SetParamNames("thread_id")
	c.SetParamValues(thread.ID)
	c.Set("user_email", "viewer@example.com")
	err = handler.ResumeThreadByPath(c)
	if err == nil {
		t.Fatal("expected pairwise 403")
	}
	httpErr, ok := err.(*echo.HTTPError)
	if !ok || httpErr.Code != http.StatusForbidden {
		t.Fatalf("got %#v", err)
	}
}

func TestGuardEnqueueDestinationBotHomeAndPlan(t *testing.T) {
	t.Parallel()
	home := db.AgentThread{
		RoomKind: RoomKindBotHome,
		AgentID:  sql.NullString{String: "home-bot", Valid: true},
	}
	if err := GuardEnqueueDestination(home, EnqueueMail{
		FromKind:    EnqueueFromAgent,
		FromAgentID: "other-bot",
	}); !errors.Is(err, ErrBotHomeRejectsA2A) {
		t.Fatalf("bot home A2A: %v", err)
	}
	if err := GuardEnqueueDestination(
		home,
		EnqueueMail{FromKind: EnqueueFromUser},
	); err != nil {
		t.Fatalf("human bot home: %v", err)
	}
	if err := GuardEnqueueDestination(home, EnqueueMail{
		FromKind:    EnqueueFromAgent,
		FromAgentID: "home-bot",
	}); err != nil {
		t.Fatalf("home bot own turn: %v", err)
	}

	plan := db.AgentThread{
		RoomKind: RoomKindPlan,
		AgentID:  sql.NullString{String: "lead", Valid: true},
	}
	if err := GuardEnqueueDestination(plan, EnqueueMail{
		FromKind:    EnqueueFromAgent,
		FromAgentID: "other-bot",
	}); err != nil {
		t.Fatalf("plan A2A inbound: %v", err)
	}

	pair := db.AgentThread{
		RoomKind:     RoomKindPairwise,
		PairAgentIDA: sql.NullString{String: "a", Valid: true},
		PairAgentIDB: sql.NullString{String: "b", Valid: true},
	}
	if err := GuardEnqueueDestination(
		pair,
		EnqueueMail{FromKind: EnqueueFromUser},
	); !errors.Is(
		err,
		ErrPairwiseViewOnly,
	) {
		t.Fatalf("pairwise human: %v", err)
	}
	if err := GuardEnqueueDestination(pair, EnqueueMail{
		FromKind:    EnqueueFromAgent,
		FromAgentID: "a",
	}); err != nil {
		t.Fatalf("pairwise pair speaker: %v", err)
	}
	if err := GuardEnqueueDestination(pair, EnqueueMail{
		FromKind:    EnqueueFromAgent,
		FromAgentID: "c",
	}); !errors.Is(err, ErrPairwiseSpeakerNotInPair) {
		t.Fatalf("outsider pairwise: %v", err)
	}
}
