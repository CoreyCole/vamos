package agentchat

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/CoreyCole/vamos/pkg/db"
)

func TestWritePiSessionHeaderUsesSessionID(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	path := filepath.Join(dir, "pi", "abc.jsonl")
	if err := WritePiSessionHeader(path, "abc", dir); err != nil {
		t.Fatal(err)
	}
	raw, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	var header piSessionHeader
	if err := json.Unmarshal(raw, &header); err != nil {
		t.Fatal(err)
	}
	if header.Type != "session" || header.ID != "abc" {
		t.Fatalf("header = %+v", header)
	}
	if header.ID == "run_id" {
		t.Fatal("header used run_id")
	}
}

func TestPrepareRoomSessionUsesPiJSONL(t *testing.T) {
	t.Parallel()
	root := t.TempDir()
	cwd := filepath.Join(root, "agents", "nova")
	if err := os.MkdirAll(cwd, 0o755); err != nil {
		t.Fatal(err)
	}
	svc := &Service{thoughtsRoot: root}
	ctx := t.Context()
	_, sessionFile, _, _, err := svc.prepareRoomSession(ctx, db.AgentThread{
		ID:          "thread-home",
		Cwd:         cwd,
		PiSessionID: "sess-pi",
		RoomKind:    RoomKindBotHome,
	}, "nova")
	if err != nil {
		t.Fatal(err)
	}
	want := filepath.Join(root, "agents", "nova", "sessions", "pi", "sess-pi.jsonl")
	if sessionFile != want {
		t.Fatalf("sessionFile = %q want %q", sessionFile, want)
	}
}

func TestPrepareRoomSessionEmptyPiUsesCurrentJSONL(t *testing.T) {
	t.Parallel()
	root := t.TempDir()
	cwd := filepath.Join(root, "agents", "nova")
	if err := os.MkdirAll(cwd, 0o755); err != nil {
		t.Fatal(err)
	}
	svc := &Service{thoughtsRoot: root}
	_, sessionFile, _, _, err := svc.prepareRoomSession(t.Context(), db.AgentThread{
		ID:       "thread-legacy",
		Cwd:      cwd,
		RoomKind: RoomKindBotHome,
	}, "nova")
	if err != nil {
		t.Fatal(err)
	}
	if !strings.HasSuffix(sessionFile, filepath.Join("sessions", "current.jsonl")) {
		t.Fatalf("legacy sessionFile = %q", sessionFile)
	}
}

func TestPrepareRoomSessionPairwiseKeepsCurrentJSONL(t *testing.T) {
	t.Parallel()
	root := t.TempDir()
	cwd := filepath.Join(root, "a2a", "aa__bb")
	if err := os.MkdirAll(cwd, 0o755); err != nil {
		t.Fatal(err)
	}
	svc := &Service{thoughtsRoot: root}
	_, sessionFile, _, _, err := svc.prepareRoomSession(t.Context(), db.AgentThread{
		ID:          "thread-pair",
		Cwd:         cwd,
		PiSessionID: "should-not-use",
		RoomKind:    RoomKindPairwise,
	}, "aa")
	if err != nil {
		t.Fatal(err)
	}
	if !strings.HasSuffix(sessionFile, filepath.Join("sessions", "current.jsonl")) {
		t.Fatalf("pairwise sessionFile = %q", sessionFile)
	}
	if strings.Contains(sessionFile, "/pi/") {
		t.Fatalf("pairwise used pi jsonl: %s", sessionFile)
	}
}
