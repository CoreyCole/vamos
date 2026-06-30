package app

import (
	"os"
	"path/filepath"
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
	rows := boardRows(guesses, renderEvent{Kind: renderReveal, RowIndex: 0})
	if rows[0].Animation != renderReveal {
		t.Fatalf("animation = %q, want reveal", rows[0].Animation)
	}
	for index, tile := range rows[0].Tiles {
		if want := index * animationDelayMS; tile.DelayMS != want {
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
			if key.Value != "a" {
				continue
			}
			aKeyFound = true
			if key.State != uiStateCorrect {
				t.Fatalf("a key state = %q, want correct", key.State)
			}
		}
	}
	if !aKeyFound {
		t.Fatal("a key not found")
	}
}

func TestRecordGuessRejectsInvalidWithoutAttempt(t *testing.T) {
	t.Parallel()
	service := newTestService(t)
	result, err := service.recordGuess(t.Context(), "alice", "UTC", "zzzzz")
	if err != nil {
		t.Fatal(err)
	}
	if result.Outcome != GuessRejected || result.Message != "Not in word list." ||
		result.Row != 0 {
		t.Fatalf("result = %#v, want rejected not-in-list row 0", result)
	}
	state, err := service.loadOrCreateToday(t.Context(), "alice", "UTC")
	if err != nil {
		t.Fatal(err)
	}
	if len(state.Guesses) != 0 {
		t.Fatalf("guesses len = %d, want 0", len(state.Guesses))
	}
}

func TestRecordGuessAcceptedConsumesAttempt(t *testing.T) {
	t.Parallel()
	service := newTestService(t)
	result, err := service.recordGuess(t.Context(), "alice", "UTC", "alert")
	if err != nil {
		t.Fatal(err)
	}
	if result.Outcome != GuessAccepted || result.Message != "Guess recorded." ||
		result.Row != 0 {
		t.Fatalf("result = %#v, want accepted row 0", result)
	}
	state, err := service.loadOrCreateToday(t.Context(), "alice", "UTC")
	if err != nil {
		t.Fatal(err)
	}
	if len(state.Guesses) != 1 || state.Guesses[0].Word != "alert" {
		t.Fatalf("guesses = %#v, want one alert", state.Guesses)
	}
}

func TestRecordGuessDuplicateRejected(t *testing.T) {
	t.Parallel()
	service := newTestService(t)
	if _, err := service.recordGuess(
		t.Context(),
		"alice",
		"UTC",
		"alert",
	); err != nil {
		t.Fatal(err)
	}
	result, err := service.recordGuess(t.Context(), "alice", "UTC", "alert")
	if err != nil {
		t.Fatal(err)
	}
	if result.Outcome != GuessRejected || result.Message != "Already guessed." ||
		result.Row != 1 {
		t.Fatalf("result = %#v, want duplicate rejected row 1", result)
	}
	state, err := service.loadOrCreateToday(t.Context(), "alice", "UTC")
	if err != nil {
		t.Fatal(err)
	}
	if len(state.Guesses) != 1 {
		t.Fatalf("guesses len = %d, want 1", len(state.Guesses))
	}
}

func TestRenderEventMapsOutcome(t *testing.T) {
	t.Parallel()
	rejected := newRenderEvent(
		"alice",
		GuessResult{Outcome: GuessRejected, Row: 2},
		"Zzzzz",
	)
	if rejected.Kind != renderShake {
		t.Fatalf("rejected kind = %q, want shake", rejected.Kind)
	}
	if rejected.Guess != "zzzzz" {
		t.Fatalf("rejected guess = %q, want zzzzz", rejected.Guess)
	}
	if got := newRenderEvent(
		"alice",
		GuessResult{Outcome: GuessAccepted, Status: StatusActive},
		"alert",
	).Kind; got != renderReveal {
		t.Fatalf("accepted kind = %q, want reveal", got)
	}
	if got := newRenderEvent(
		"alice",
		GuessResult{Outcome: GuessAccepted, Status: StatusWon},
		"alert",
	).Kind; got != renderWin {
		t.Fatalf("won kind = %q, want win", got)
	}
}

type fixedClock struct{ now time.Time }

func (c fixedClock) Now() time.Time { return c.now }

func newTestService(t *testing.T) *Service {
	t.Helper()
	wordFile := filepath.Join(t.TempDir(), "words.txt")
	if err := os.WriteFile(wordFile, []byte("apple\nalert\ncrane\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	service, err := New(Config{
		FilesRoot: t.TempDir(),
		WordFile:  wordFile,
		Location:  time.UTC,
		Clock:     fixedClock{now: time.Date(2026, 6, 29, 12, 0, 0, 0, time.UTC)},
	})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = service.Close() })
	return service
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
