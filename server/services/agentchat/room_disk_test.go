package agentchat

import (
	"database/sql"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/CoreyCole/vamos/pkg/db"
)

func TestPairwiseDirNameLexicographic(t *testing.T) {
	t.Parallel()
	got, err := PairwiseDirName("zeta", "alpha")
	if err != nil {
		t.Fatal(err)
	}
	if got != "alpha__zeta" {
		t.Fatalf("PairwiseDirName = %q", got)
	}
}

func TestRoomPathsBotHomePlanPairwise(t *testing.T) {
	t.Parallel()

	home := RoomIdentity{Kind: RoomKindBotHome, SpeakerSlug: "hermes"}
	got, err := home.CurrentJSONLRel()
	if err != nil {
		t.Fatal(err)
	}
	if got != "thoughts/agents/hermes/sessions/current.jsonl" {
		t.Fatalf("home current = %q", got)
	}

	plan := RoomIdentity{Kind: RoomKindPlan, PlanDirRel: "thoughts/acme/plans/demo"}
	got, err = plan.CurrentJSONLRel()
	if err != nil {
		t.Fatal(err)
	}
	if got != "thoughts/acme/plans/demo/.vamos/sessions/current.jsonl" {
		t.Fatalf("plan current = %q", got)
	}
	if sessions, _ := plan.SessionsRel(); strings.Contains(sessions, "/sessions/pi") {
		t.Fatalf("plan sessions reused pi dir: %s", sessions)
	}

	pair := RoomIdentity{Kind: RoomKindPairwise, PairA: "b", PairB: "a"}
	got, err = pair.CurrentJSONLRel()
	if err != nil {
		t.Fatal(err)
	}
	if got != "thoughts/a2a/a__b/sessions/current.jsonl" {
		t.Fatalf("pairwise current = %q", got)
	}
}

func TestRoomIdentityFromThreadClassifiesHomes(t *testing.T) {
	t.Parallel()
	root := t.TempDir()
	cwd := filepath.Join(root, "agents", "nova")
	id, err := RoomIdentityFromThread(root, db.AgentThread{
		ID:  "t1",
		Cwd: cwd,
	}, "")
	if err != nil {
		t.Fatal(err)
	}
	if id.Kind != RoomKindBotHome || id.SpeakerSlug != "nova" {
		t.Fatalf("got %+v", id)
	}

	pairCwd := filepath.Join(root, "a2a", "aa__bb")
	id, err = RoomIdentityFromThread(root, db.AgentThread{ID: "t2", Cwd: pairCwd}, "aa")
	if err != nil {
		t.Fatal(err)
	}
	if id.Kind != RoomKindPairwise || id.PairA != "aa" || id.PairB != "bb" {
		t.Fatalf("got %+v", id)
	}

	id, err = RoomIdentityFromThread(root, db.AgentThread{
		ID: "t3",
		PlanDirRel: sql.NullString{
			String: "owner/plans/job",
			Valid:  true,
		},
	}, "lead")
	if err != nil {
		t.Fatal(err)
	}
	if id.Kind != RoomKindPlan || id.PlanDirRel != "thoughts/owner/plans/job" {
		t.Fatalf("got %+v", id)
	}
}

func TestEnsureRoomCurrentJSONLCreatesParent(t *testing.T) {
	t.Parallel()
	root := t.TempDir()
	id := RoomIdentity{Kind: RoomKindBotHome, SpeakerSlug: "nova"}
	path, err := EnsureRoomCurrentJSONL(root, id)
	if err != nil {
		t.Fatal(err)
	}
	want := filepath.Join(root, "agents", "nova", "sessions", "current.jsonl")
	if path != want {
		t.Fatalf("path = %q want %q", path, want)
	}
	if _, err := os.Stat(filepath.Dir(path)); err != nil {
		t.Fatal(err)
	}
}

func TestBuildWindowInjectFilesOrder(t *testing.T) {
	t.Parallel()
	root := t.TempDir()
	id := RoomIdentity{Kind: RoomKindBotHome, SpeakerSlug: "nova"}
	agentsDir := filepath.Join(root, "agents", "nova")
	if err := os.MkdirAll(
		filepath.Join(agentsDir, "sessions", "handoffs"),
		0o755,
	); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(
		filepath.Join(agentsDir, "AGENTS.md"),
		[]byte("# Nova\n"),
		0o644,
	); err != nil {
		t.Fatal(err)
	}
	listed := filepath.Join(root, "agents", "nova", "notes.md")
	if err := os.WriteFile(listed, []byte("note body"), 0o644); err != nil {
		t.Fatal(err)
	}
	handoff := "---\nfiles:\n  - thoughts/agents/nova/notes.md\n---\nroom handoff body\n"
	if err := os.WriteFile(
		filepath.Join(agentsDir, "sessions", "handoffs", "2026-09-09_16-04-47.md"),
		[]byte(handoff),
		0o644,
	); err != nil {
		t.Fatal(err)
	}

	files, err := BuildWindowInjectFiles(root, id, []AgentRosterRow{
		{Slug: "nova", Name: "Nova", Label: "lead", Description: "does work"},
		{Slug: "other", Name: "Other"},
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(files) != 4 {
		t.Fatalf("len = %d files=%+v", len(files), files)
	}
	if files[0].Path != "thoughts/agents/nova/AGENTS.md" ||
		files[0].Content != "# Nova\n" {
		t.Fatalf("first = %+v", files[0])
	}
	if files[1].Path != "agent-directory.md" {
		t.Fatalf("second = %+v", files[1])
	}
	if !strings.Contains(files[1].Content, "`nova` (current speaker)") {
		t.Fatalf("directory missing speaker mark: %s", files[1].Content)
	}
	if !strings.Contains(files[1].Content, "Never a thread UUID") {
		t.Fatalf("directory missing addressing reminder")
	}
	if !strings.Contains(files[2].Content, "room handoff body") {
		t.Fatalf("handoff = %+v", files[2])
	}
	if files[3].Path != "thoughts/agents/nova/notes.md" ||
		files[3].Content != "note body" {
		t.Fatalf("listed file = %+v", files[3])
	}
}

func TestBuildWindowInjectSkipsBotHomeHandoffOnPlan(t *testing.T) {
	t.Parallel()
	root := t.TempDir()
	homeHandoffs := filepath.Join(root, "agents", "nova", "sessions", "handoffs")
	if err := os.MkdirAll(homeHandoffs, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(
		filepath.Join(homeHandoffs, "2026-09-09_16-04-47.md"),
		[]byte("home only"),
		0o644,
	); err != nil {
		t.Fatal(err)
	}
	plan := RoomIdentity{
		Kind:        RoomKindPlan,
		SpeakerSlug: "nova",
		PlanDirRel:  "thoughts/acme/plans/job",
	}
	if err := os.MkdirAll(filepath.Join(root, "agents", "nova"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(
		filepath.Join(root, "agents", "nova", "AGENTS.md"),
		[]byte("agents"),
		0o644,
	); err != nil {
		t.Fatal(err)
	}

	files, err := BuildWindowInjectFiles(root, plan, nil)
	if err != nil {
		t.Fatal(err)
	}
	for _, f := range files {
		if strings.Contains(f.Content, "home only") {
			t.Fatalf("plan inject consumed bot-home handoff: %+v", files)
		}
	}
}
