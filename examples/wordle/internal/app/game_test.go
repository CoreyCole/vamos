package app

import (
	"testing"
	"time"
)

func TestScoreGuessRepeatedLetters(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name   string
		answer string
		guess  string
		want   []TileState
	}{
		{
			name:   "exact",
			answer: "apple",
			guess:  "apple",
			want:   []TileState{TileGreen, TileGreen, TileGreen, TileGreen, TileGreen},
		},
		{
			name:   "do not over yellow duplicate",
			answer: "apple",
			guess:  "allee",
			want:   []TileState{TileGreen, TileYellow, TileGray, TileGray, TileGreen},
		},
		{
			name:   "green consumes before yellow",
			answer: "abbey",
			guess:  "bobby",
			want:   []TileState{TileYellow, TileGray, TileGreen, TileGray, TileGreen},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			got := ScoreGuess(tt.answer, tt.guess)
			for i, want := range tt.want {
				if got[i].State != want {
					t.Fatalf("tile %d state = %s, want %s", i, got[i].State, want)
				}
			}
		})
	}
}

func TestTileStateMapsInternalStates(t *testing.T) {
	t.Parallel()
	tests := []struct {
		state TileState
		want  string
	}{
		{state: TileGreen, want: "correct"},
		{state: TileYellow, want: "present"},
		{state: TileGray, want: "absent"},
		{state: TileUnknown, want: "empty"},
	}
	for _, tt := range tests {
		if got := tileState(tt.state); got != tt.want {
			t.Fatalf("tileState(%s) = %s, want %s", tt.state, got, tt.want)
		}
	}
}

func TestBoardRowsMarksCurrentAndSubmitted(t *testing.T) {
	t.Parallel()
	rows := boardRows([]ScoredGuess{{Tiles: ScoreGuess("apple", "alert")}}, renderEvent{})
	if len(rows) != maxAttempts {
		t.Fatalf("rows len = %d, want %d", len(rows), maxAttempts)
	}
	if !rows[0].Submitted || rows[0].Current {
		t.Fatalf(
			"row 0 submitted/current = %v/%v, want true/false",
			rows[0].Submitted,
			rows[0].Current,
		)
	}
	if rows[0].Index != 0 {
		t.Fatalf("row 0 index = %d, want 0", rows[0].Index)
	}
	if rows[0].Tiles[0].Index != 0 || rows[0].Tiles[4].Index != 4 {
		t.Fatalf(
			"tile indexes = %d/%d, want 0/4",
			rows[0].Tiles[0].Index,
			rows[0].Tiles[4].Index,
		)
	}
	if !rows[1].Current || rows[1].Submitted {
		t.Fatalf(
			"row 1 current/submitted = %v/%v, want true/false",
			rows[1].Current,
			rows[1].Submitted,
		)
	}
}

func TestBoardRowsAppliesTransientEvent(t *testing.T) {
	t.Parallel()
	guesses := []ScoredGuess{{Tiles: ScoreGuess("apple", "apple")}}
	rows := boardRows(guesses, renderEvent{Kind: "reveal", RowIndex: 0})
	if rows[0].Animation != "reveal" {
		t.Fatalf("animation = %q, want reveal", rows[0].Animation)
	}
	for index, tile := range rows[0].Tiles {
		if want := index * 100; tile.DelayMS != want {
			t.Fatalf("tile %d delay = %d, want %d", index, tile.DelayMS, want)
		}
	}
	rows = boardRows(guesses, renderEvent{})
	if rows[0].Animation != "" || rows[0].Tiles[4].DelayMS != 0 {
		t.Fatalf(
			"empty event replayed animation: %q delay %d",
			rows[0].Animation,
			rows[0].Tiles[4].DelayMS,
		)
	}
}

func TestKeyboardRowsUseWordleLayoutAndBestKnownState(t *testing.T) {
	t.Parallel()
	guesses := []ScoredGuess{
		{Tiles: []TileResult{{Letter: "a", State: TileGray}}},
		{Tiles: []TileResult{{Letter: "a", State: TileYellow}}},
		{Tiles: []TileResult{{Letter: "a", State: TileGreen}}},
	}
	rows := keyboardRows(guesses, renderEvent{})
	if len(rows) != 3 {
		t.Fatalf("keyboard rows = %d, want 3", len(rows))
	}
	lastRow := rows[2].Keys
	if lastRow[0].Value != "enter" || lastRow[len(lastRow)-1].Value != "backspace" {
		t.Fatalf(
			"last row edges = %q/%q, want enter/backspace",
			lastRow[0].Value,
			lastRow[len(lastRow)-1].Value,
		)
	}
	var aKeyFound bool
	for _, row := range rows {
		for _, key := range row.Keys {
			if key.Value == "a" {
				aKeyFound = true
				if key.State != "correct" {
					t.Fatalf("a key state = %q, want correct", key.State)
				}
			}
		}
	}
	if !aKeyFound {
		t.Fatal("a key not found")
	}
}

func TestPuzzleDateUsesLocation(t *testing.T) {
	t.Parallel()
	instant := time.Date(2026, 6, 30, 6, 30, 0, 0, time.UTC)
	losAngeles, err := time.LoadLocation("America/Los_Angeles")
	if err != nil {
		t.Fatal(err)
	}
	tokyo, err := time.LoadLocation("Asia/Tokyo")
	if err != nil {
		t.Fatal(err)
	}
	if got := PuzzleDate(instant, losAngeles); got != "2026-06-29" {
		t.Fatalf("Los Angeles date = %s", got)
	}
	if got := PuzzleDate(instant, tokyo); got != "2026-06-30" {
		t.Fatalf("Tokyo date = %s", got)
	}
}

func TestDailyAnswerStable(t *testing.T) {
	t.Parallel()
	words := WordList{
		Ordered: []string{"aback", "abase", "abate"},
		Words:   map[string]struct{}{"aback": {}, "abase": {}, "abate": {}},
	}
	first, err := DailyAnswer(words, "2026-06-29")
	if err != nil {
		t.Fatal(err)
	}
	second, err := DailyAnswer(words, "2026-06-29")
	if err != nil {
		t.Fatal(err)
	}
	if first != second {
		t.Fatalf("answer not stable: %s != %s", first, second)
	}
}

func TestNormalizeUsername(t *testing.T) {
	t.Parallel()
	got, err := NormalizeUsername(" Alice_1 ")
	if err != nil {
		t.Fatal(err)
	}
	if got != "alice_1" {
		t.Fatalf("username = %q", got)
	}
	if _, err := NormalizeUsername("al"); err == nil {
		t.Fatal("short username accepted")
	}
}
