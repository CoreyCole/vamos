package main

import (
	"encoding/csv"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestLoadPlayersAndGenerateBalancedMatchups(t *testing.T) {
	root := t.TempDir()
	playersPath := filepath.Join(root, "players.csv")
	if err := os.WriteFile(playersPath, []byte("name,skill\nA,10\nB,8\nC,7\nD,1\nE,9\nF,5\nG,4\nH,3\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	players, err := LoadPlayers(playersPath)
	if err != nil {
		t.Fatalf("LoadPlayers() error = %v", err)
	}
	matchups := GenerateMatchups(players)
	if len(matchups) != 2 {
		t.Fatalf("matchups count = %d, want 2", len(matchups))
	}
	if got := TeamNames(matchups[0].TeamA); got != "A (10) + C (7)" {
		t.Fatalf("first team A = %q", got)
	}
	if got := TeamNames(matchups[0].TeamB); got != "E (9) + B (8)" {
		t.Fatalf("first team B = %q", got)
	}
}

func TestWritesStayInsideFilesRoot(t *testing.T) {
	root := t.TempDir()
	matchups := []Matchup{{
		Court:  1,
		TeamA:  []Player{{Name: "A", Skill: 9}, {Name: "D", Skill: 4}},
		TeamB:  []Player{{Name: "B", Skill: 8}, {Name: "C", Skill: 5}},
		Reason: "Balanced.",
	}}

	if err := WriteMatchupsCSV(root, matchups); err != nil {
		t.Fatalf("WriteMatchupsCSV() error = %v", err)
	}
	if err := WriteTournamentHTML(root, matchups); err != nil {
		t.Fatalf("WriteTournamentHTML() error = %v", err)
	}
	if _, err := os.Stat(filepath.Join(root, "matchups.csv")); err != nil {
		t.Fatalf("matchups.csv missing: %v", err)
	}
	if _, err := os.Stat(filepath.Join(root, "tournament.html")); err != nil {
		t.Fatalf("tournament.html missing: %v", err)
	}

	defer func() {
		if recover() == nil {
			t.Fatal("SafeJoin did not panic for escape path")
		}
	}()
	_ = SafeJoin(root, "../escape.csv")
}

func TestRoutesRenderAndServeFiles(t *testing.T) {
	root := t.TempDir()
	if err := os.WriteFile(filepath.Join(root, "players.csv"), []byte(defaultPlayersCSV), 0o644); err != nil {
		t.Fatal(err)
	}

	server := httptest.NewServer(Routes(root))
	defer server.Close()

	resp, err := http.Get(server.URL + "/")
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("GET / status = %d", resp.StatusCode)
	}
	if _, err := os.Stat(filepath.Join(root, "matchups.csv")); err != nil {
		t.Fatalf("GET / did not write matchups.csv: %v", err)
	}

	csvResp, err := http.Get(server.URL + "/matchups.csv")
	if err != nil {
		t.Fatal(err)
	}
	defer csvResp.Body.Close()
	if csvResp.StatusCode != http.StatusOK {
		t.Fatalf("GET /matchups.csv status = %d", csvResp.StatusCode)
	}
	records, err := csv.NewReader(csvResp.Body).ReadAll()
	if err != nil {
		t.Fatalf("read served csv: %v", err)
	}
	if len(records) < 2 || strings.Join(records[0], ",") != "court,team_a,team_b,reason" {
		t.Fatalf("unexpected csv records: %#v", records)
	}
}
