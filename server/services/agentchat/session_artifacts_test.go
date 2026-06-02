package agentchat

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"github.com/CoreyCole/vamos/pkg/db"
)

func TestPlanAgentSessionDirAndWorkspaceConfig(t *testing.T) {
	planDir := filepath.Join(t.TempDir(), "plan")
	workspaceDir := filepath.Join(t.TempDir(), "workspace")
	if err := os.MkdirAll(workspaceDir, 0o755); err != nil {
		t.Fatal(err)
	}

	sessionDir, err := PlanAgentSessionDir(planDir, "")
	if err != nil {
		t.Fatalf("PlanAgentSessionDir default: %v", err)
	}
	if want := filepath.Join(planDir, ".sessions", "pi"); sessionDir != want {
		t.Fatalf("sessionDir = %q, want %q", sessionDir, want)
	}
	if _, err := PlanAgentSessionDir(planDir, "../escape"); err == nil {
		t.Fatal("PlanAgentSessionDir accepted path-like agent")
	}

	if err := ConfigureWorkspaceAgentSessionDir(workspaceDir, planDir, "pi"); err != nil {
		t.Fatalf("ConfigureWorkspaceAgentSessionDir: %v", err)
	}
	data, err := os.ReadFile(filepath.Join(workspaceDir, ".pi", "settings.json"))
	if err != nil {
		t.Fatalf("read settings: %v", err)
	}
	var settings map[string]string
	if err := json.Unmarshal(data, &settings); err != nil {
		t.Fatalf("settings json: %v", err)
	}
	if settings["sessionDir"] != sessionDir {
		t.Fatalf("settings sessionDir = %q, want %q", settings["sessionDir"], sessionDir)
	}
}

func TestDiscoverPlanAgentSessionsIndexesTopLevelAndReviewPlans(t *testing.T) {
	root := t.TempDir()
	planDir := filepath.Join(root, "thoughts", "agent", "plans", "2026-06-02_plan")
	topSession := filepath.Join(planDir, ".sessions", "pi", "top.jsonl")
	reviewSession := filepath.Join(planDir, "reviews", "2026-06-02_plan_implementation-review", ".sessions", "codex", "review.jsonl")
	writeSessionHeader(t, topSession, `{"type":"session","id":"top","cwd":"/repo","workflow_id":"wf","workflow_node_id":"design"}`)
	writeSessionHeader(t, reviewSession, `{"type":"session","id":"review","cwd":"/repo2","parentSession":"top","forked_from_session_id":"fork"}`)

	items, err := DiscoverPlanAgentSessions(planDir)
	if err != nil {
		t.Fatalf("DiscoverPlanAgentSessions: %v", err)
	}
	if len(items) != 2 {
		t.Fatalf("items len = %d, want 2: %#v", len(items), items)
	}
	byID := map[string]SessionArtifactIndex{}
	for _, item := range items {
		byID[item.SessionID] = item
		if item.Hash == "" || item.Size == 0 || item.LastOffset == 0 || !item.NeedsHydration {
			t.Fatalf("incomplete fingerprint for %#v", item)
		}
	}
	if byID["top"].Agent != "pi" || byID["top"].PlanDir != planDir || byID["top"].WorkflowID != "wf" || byID["top"].NodeID != "design" {
		t.Fatalf("top item = %#v", byID["top"])
	}
	if byID["review"].Agent != "codex" || byID["review"].ParentPlanDir != planDir || byID["review"].SourceReviewDir == "" || byID["review"].ContinuedFromSessionID != "top" || byID["review"].ForkedFromSessionID != "fork" {
		t.Fatalf("review item = %#v", byID["review"])
	}
}

func TestHydrateSessionArtifactImportsColdPlanOwnedSession(t *testing.T) {
	service := newTestAgentChatService(t)
	planDir := filepath.Join(service.thoughtsRoot, "user@example.com", "plans", "2026-06-02_plan")
	if err := ensureDir(planDir); err != nil {
		t.Fatalf("ensureDir(planDir): %v", err)
	}
	artifactPath := filepath.Join(planDir, "plan.md")
	sessionPath := filepath.Join(planDir, ".sessions", "pi", "session.jsonl")
	writePiSessionFile(t, sessionPath, piImportFixtureLines(artifactPath)...)

	_, err := service.queries.UpsertAgentSessionIndex(context.Background(), db.UpsertAgentSessionIndexParams{
		ID:                "indexed-session",
		UserEmail:         nullableString("user@example.com"),
		Source:            string(AgentSessionSourceTerminal),
		SessionPath:       nullableString(sessionPath),
		SessionID:         nullableString("session-1"),
		Agent:             "pi",
		FileSize:          int64(len([]byte("x"))),
		LastIndexedOffset: int64(len([]byte("x"))),
		NeedsHydration:    1,
		Status:            "pending",
		InferredPlanDir:   nullableString(planDir),
	})
	if err != nil {
		t.Fatalf("UpsertAgentSessionIndex: %v", err)
	}

	projection, err := service.HydrateSessionArtifact(context.Background(), sessionPath)
	if err != nil {
		t.Fatalf("HydrateSessionArtifact: %v", err)
	}
	if len(projection.Messages) == 0 {
		t.Fatalf("projection messages = 0, want imported messages")
	}
	row, err := service.queries.GetAgentSessionByPath(context.Background(), nullableString(sessionPath))
	if err != nil {
		t.Fatalf("GetAgentSessionByPath: %v", err)
	}
	if row.NeedsHydration != 0 || !row.ThreadID.Valid {
		t.Fatalf("row hydration/thread = %d/%v, want hydrated linked thread", row.NeedsHydration, row.ThreadID)
	}
}

func writeSessionHeader(t *testing.T, path string, header string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(header+"\n"), 0o644); err != nil {
		t.Fatal(err)
	}
}
