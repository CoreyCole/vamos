package app

import (
	"crypto/sha256"
	"encoding/binary"
	"errors"
	"regexp"
	"strings"
	"time"
)

type GameStatus string

const (
	StatusActive GameStatus = "active"
	StatusWon    GameStatus = "won"
	StatusLost   GameStatus = "lost"
)

type TileState string

const (
	TileUnknown TileState = "unknown"
	TileGreen   TileState = "green"
	TileYellow  TileState = "yellow"
	TileGray    TileState = "gray"
)

type TileResult struct {
	Letter string    `json:"letter"`
	State  TileState `json:"state"`
}

type ScoredGuess struct {
	Word  string       `json:"word"`
	Tiles []TileResult `json:"tiles"`
}

const (
	rankGreen  = 3
	rankYellow = 2
	rankGray   = 1
	rankEmpty  = 0
)

var usernamePattern = regexp.MustCompile(`^[a-z0-9_-]{3,32}$`)

func NormalizeUsername(raw string) (string, error) {
	username := strings.ToLower(strings.TrimSpace(raw))
	if !usernamePattern.MatchString(username) {
		return "", errors.New(
			"username must be 3-32 letters, numbers, dash, or underscore",
		)
	}
	return username, nil
}

func NormalizeGuess(raw string) (string, error) {
	guess := strings.ToLower(strings.TrimSpace(raw))
	if len(guess) != wordLength {
		return "", errors.New("guess must be 5 letters")
	}
	for _, r := range guess {
		if r < 'a' || r > 'z' {
			return "", errors.New("guess must use letters only")
		}
	}
	return guess, nil
}

func PuzzleDate(now time.Time, loc *time.Location) string {
	if loc == nil {
		loc = time.Local
	}
	return now.In(loc).Format(time.DateOnly)
}

func DailyAnswer(words WordList, localPuzzleDate string) (string, error) {
	if len(words.Ordered) == 0 {
		return "", errors.New("word list is empty")
	}
	sum := sha256.Sum256([]byte(localPuzzleDate))
	index := binary.BigEndian.Uint64(sum[:8]) % uint64(len(words.Ordered))
	return words.Ordered[index], nil
}

func ScoreGuess(answer, guess string) []TileResult {
	answerRunes := []rune(answer)
	guessRunes := []rune(guess)
	results := make([]TileResult, len(guessRunes))
	remaining := map[rune]int{}

	for i, r := range guessRunes {
		results[i] = TileResult{Letter: string(r), State: TileGray}
		if i < len(answerRunes) && r == answerRunes[i] {
			results[i].State = TileGreen
			continue
		}
		if i < len(answerRunes) {
			remaining[answerRunes[i]]++
		}
	}

	for i, r := range guessRunes {
		if results[i].State == TileGreen {
			continue
		}
		if remaining[r] > 0 {
			results[i].State = TileYellow
			remaining[r]--
		}
	}
	return results
}

func KeyboardState(guesses []ScoredGuess) map[string]TileState {
	states := map[string]TileState{}
	for ch := 'a'; ch <= 'z'; ch++ {
		states[string(ch)] = TileUnknown
	}
	for _, guess := range guesses {
		for _, tile := range guess.Tiles {
			current := states[tile.Letter]
			if stateRank(tile.State) > stateRank(current) {
				states[tile.Letter] = tile.State
			}
		}
	}
	return states
}

func stateRank(state TileState) int {
	switch state {
	case TileGreen:
		return rankGreen
	case TileYellow:
		return rankYellow
	case TileGray:
		return rankGray
	case TileUnknown:
		return rankEmpty
	default:
		return rankEmpty
	}
}
