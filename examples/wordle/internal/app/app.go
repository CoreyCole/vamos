package app

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"example.com/vamos-wordle/internal/store"
	"example.com/vamos-wordle/internal/store/dbgen"
	"example.com/vamos-wordle/internal/ui"
)

const maxAttempts = 6

type Clock interface {
	Now() time.Time
}

type realClock struct{}

func (realClock) Now() time.Time { return time.Now() }

type Config struct {
	FilesRoot string
	WordFile  string
	Location  *time.Location
	Clock     Clock
}

type Service struct {
	filesRoot string
	db        *sql.DB
	queries   *dbgen.Queries
	words     WordList
	clock     Clock
	location  *time.Location
	notifier  *notifier
}

type GameState struct {
	Username   string
	Date       string
	Answer     string
	Status     GameStatus
	Guesses    []ScoredGuess
	RawGuesses []dbgen.Guess
}

type GuessOutcome string

const (
	GuessAccepted GuessOutcome = "accepted"
	GuessRejected GuessOutcome = "rejected"
)

type GuessResult struct {
	Outcome GuessOutcome
	Message string
	Row     int
	Status  GameStatus
}

type renderEvent struct {
	ID       string
	Username string
	Kind     string
	RowIndex int
	Message  string
}

type notifierEvent struct {
	Username string
	Event    renderEvent
}

func New(cfg Config) (*Service, error) {
	root := strings.TrimSpace(cfg.FilesRoot)
	if root == "" {
		root = "./files"
	}
	if err := os.MkdirAll(root, 0o755); err != nil {
		return nil, fmt.Errorf("create files root: %w", err)
	}
	wordFile := strings.TrimSpace(cfg.WordFile)
	if wordFile == "" {
		wordFile = filepath.Join("internal", "words", "words.txt")
	}
	words, err := LoadWordList(wordFile)
	if err != nil {
		return nil, err
	}
	database, err := store.Open(filepath.Join(root, "app.db"))
	if err != nil {
		return nil, err
	}
	clock := cfg.Clock
	if clock == nil {
		clock = realClock{}
	}
	location := cfg.Location
	if location == nil {
		location = time.Local
	}
	return &Service{
		filesRoot: root,
		db:        database,
		queries:   dbgen.New(database),
		words:     words,
		clock:     clock,
		location:  location,
		notifier:  newNotifier(),
	}, nil
}

func (s *Service) Close() error {
	if s.db == nil {
		return nil
	}
	return s.db.Close()
}

func (s *Service) pageData(
	ctx context.Context,
	username, timezoneName, message string, event renderEvent,
) (ui.PageData, error) {
	if username == "" {
		return ui.PageData{Auth: ui.AuthView{LoggedIn: false}, Message: message}, nil
	}
	state, err := s.loadOrCreateToday(ctx, username, timezoneName)
	if err != nil {
		return ui.PageData{}, err
	}
	if message == "" {
		message = defaultMessage(state)
	}
	rows := boardRows(state.Guesses, event)
	keyboardRows := keyboardRows(state.Guesses, event)
	return ui.PageData{
		Auth: ui.AuthView{LoggedIn: true, Username: username, Timezone: timezoneName},
		Game: ui.GameView{
			PuzzleDate:   state.Date,
			Status:       string(state.Status),
			Rows:         rows,
			KeyboardRows: keyboardRows,
			Keyboard:     flattenKeyboard(keyboardRows),
			AttemptsUsed: len(state.Guesses),
			AttemptsMax:  maxAttempts,
			CanGuess:     state.Status == StatusActive,
			Answer:       revealAnswer(state),
			CurrentRow:   currentRowIndex(state),
			RenderEvent:  renderEventView(event),
		},
		Message: message,
	}, nil
}

func (s *Service) loadOrCreateToday(
	ctx context.Context,
	username, timezoneName string,
) (GameState, error) {
	loc := s.requestLocation(timezoneName)
	date := PuzzleDate(s.clock.Now(), loc)
	if err := s.queries.UpsertUser(
		ctx,
		dbgen.UpsertUserParams{Username: username, Timezone: timezoneName},
	); err != nil {
		return GameState{}, err
	}
	game, err := s.queries.GetDailyGame(
		ctx,
		dbgen.GetDailyGameParams{Username: username, PuzzleDate: date},
	)
	if errors.Is(err, sql.ErrNoRows) {
		answer, answerErr := DailyAnswer(s.words, date)
		if answerErr != nil {
			return GameState{}, answerErr
		}
		game, err = s.queries.CreateDailyGame(ctx, dbgen.CreateDailyGameParams{
			Username:        username,
			PuzzleDate:      date,
			Answer:          answer,
			WordListVersion: s.words.Version,
		})
	}
	if err != nil {
		return GameState{}, err
	}
	rows, err := s.queries.ListGuesses(
		ctx,
		dbgen.ListGuessesParams{Username: username, PuzzleDate: date},
	)
	if err != nil {
		return GameState{}, err
	}
	scored := make([]ScoredGuess, 0, len(rows))
	for _, row := range rows {
		var tiles []TileResult
		if err := json.Unmarshal([]byte(row.ResultJson), &tiles); err != nil {
			return GameState{}, err
		}
		scored = append(scored, ScoredGuess{Word: row.Guess, Tiles: tiles})
	}
	return GameState{
		Username:   username,
		Date:       date,
		Answer:     game.Answer,
		Status:     GameStatus(game.Status),
		Guesses:    scored,
		RawGuesses: rows,
	}, nil
}

func (s *Service) recordGuess(
	ctx context.Context,
	username, timezoneName, rawGuess string,
) (GuessResult, error) {
	state, err := s.loadOrCreateToday(ctx, username, timezoneName)
	if err != nil {
		return GuessResult{}, err
	}
	rowIndex := currentRowIndex(state)
	guess, err := NormalizeGuess(rawGuess)
	if err != nil {
		return rejectedGuess(err.Error(), rowIndex, state.Status), nil
	}
	if !s.words.Contains(guess) {
		return rejectedGuess("Not in word list.", rowIndex, state.Status), nil
	}
	if state.Status != StatusActive {
		return rejectedGuess("Game already finished.", rowIndex, state.Status), nil
	}
	if len(state.Guesses) >= maxAttempts {
		return rejectedGuess("No guesses left.", rowIndex, state.Status), nil
	}
	rowIndex = len(state.Guesses)
	tiles := ScoreGuess(state.Answer, guess)
	payload, err := json.Marshal(tiles)
	if err != nil {
		return GuessResult{}, err
	}
	_, err = s.queries.InsertGuess(ctx, dbgen.InsertGuessParams{
		Username:   username,
		PuzzleDate: state.Date,
		RowIndex:   int64(rowIndex),
		Guess:      guess,
		ResultJson: string(payload),
	})
	if err != nil {
		return rejectedGuess("Already guessed.", rowIndex, state.Status), nil
	}
	status := StatusActive
	if guess == state.Answer {
		status = StatusWon
	} else if len(state.Guesses)+1 >= maxAttempts {
		status = StatusLost
	}
	if status != StatusActive {
		if err := s.queries.UpdateGameStatus(ctx, dbgen.UpdateGameStatusParams{
			Status:     string(status),
			Column2:    string(status),
			Username:   username,
			PuzzleDate: state.Date,
		}); err != nil {
			return GuessResult{}, err
		}
	}
	return GuessResult{
		Outcome: GuessAccepted,
		Message: "Guess recorded.",
		Row:     rowIndex,
		Status:  status,
	}, nil
}

func rejectedGuess(message string, row int, status GameStatus) GuessResult {
	return GuessResult{Outcome: GuessRejected, Message: message, Row: row, Status: status}
}

func newRenderEvent(username string, result GuessResult) renderEvent {
	kind := "shake"
	if result.Outcome == GuessAccepted {
		kind = "reveal"
		if result.Status == StatusWon {
			kind = "win"
		}
	}
	return renderEvent{
		ID:       fmt.Sprintf("%d", time.Now().UnixNano()),
		Username: username,
		Kind:     kind,
		RowIndex: result.Row,
		Message:  result.Message,
	}
}

func (s *Service) requestLocation(timezoneName string) *time.Location {
	if timezoneName != "" {
		loc, err := time.LoadLocation(timezoneName)
		if err == nil {
			return loc
		}
	}
	return s.location
}

func currentRowIndex(state GameState) int {
	if state.Status != StatusActive || len(state.Guesses) >= maxAttempts {
		return -1
	}
	return len(state.Guesses)
}

func renderEventView(event renderEvent) ui.RenderEventView {
	return ui.RenderEventView{
		ID:       event.ID,
		Kind:     event.Kind,
		RowIndex: event.RowIndex,
		Message:  event.Message,
	}
}

func tileState(state TileState) string {
	switch state {
	case TileGreen:
		return "correct"
	case TileYellow:
		return "present"
	case TileGray:
		return "absent"
	case TileUnknown:
		return "empty"
	default:
		return "empty"
	}
}

func boardRows(guesses []ScoredGuess, event renderEvent) []ui.GuessRow {
	rows := make([]ui.GuessRow, 0, maxAttempts)
	for rowIndex := range maxAttempts {
		row := ui.GuessRow{Index: rowIndex}
		if rowIndex < len(guesses) {
			row.Submitted = true
			row.Animation = submittedRowAnimation(rowIndex, event)
			row.Tiles = submittedTiles(guesses[rowIndex], row.Animation)
		} else {
			row.Current = rowIndex == len(guesses)
			row.Animation = currentRowAnimation(row.Current, event)
			row.Tiles = emptyTiles(row.Current)
		}
		rows = append(rows, row)
	}
	return rows
}

func submittedRowAnimation(rowIndex int, event renderEvent) string {
	if event.RowIndex != rowIndex {
		return ""
	}
	if event.Kind == "reveal" || event.Kind == "win" {
		return event.Kind
	}
	return ""
}

func currentRowAnimation(current bool, event renderEvent) string {
	if current && event.Kind == "shake" {
		return "shake"
	}
	return ""
}

func submittedTiles(guess ScoredGuess, animation string) []ui.TileView {
	tiles := make([]ui.TileView, 0, 5)
	for index, tile := range guess.Tiles {
		view := ui.TileView{
			Index:  index,
			Letter: strings.ToUpper(tile.Letter),
			State:  tileState(tile.State),
		}
		if animation == "reveal" || animation == "win" {
			view.DelayMS = index * 100
			view.Animation = "flip"
		}
		tiles = append(tiles, view)
	}
	return tiles
}

func emptyTiles(current bool) []ui.TileView {
	state := "empty"
	if current {
		state = "tbd"
	}
	tiles := make([]ui.TileView, 0, 5)
	for index := range 5 {
		tiles = append(tiles, ui.TileView{Index: index, State: state})
	}
	return tiles
}

func keyboardRows(guesses []ScoredGuess, event renderEvent) []ui.KeyboardRow {
	states := KeyboardState(guesses)
	delays := keyboardDelays(guesses, event)
	layouts := [][]ui.KeyboardKey{
		keysFromLetters("qwertyuiop"),
		keysFromLetters("asdfghjkl"),
		append(
			[]ui.KeyboardKey{{Label: "Enter", Value: "enter", Wide: true}},
			append(
				keysFromLetters("zxcvbnm"),
				ui.KeyboardKey{Label: "⌫", Value: "backspace", Wide: true},
			)...),
	}
	rows := make([]ui.KeyboardRow, 0, len(layouts))
	for _, keys := range layouts {
		row := ui.KeyboardRow{Keys: make([]ui.KeyboardKey, 0, len(keys))}
		for _, key := range keys {
			if len(key.Value) == 1 {
				key.State = tileState(states[key.Value])
				key.DelayMS = delays[key.Value]
			}
			row.Keys = append(row.Keys, key)
		}
		rows = append(rows, row)
	}
	return rows
}

func keysFromLetters(letters string) []ui.KeyboardKey {
	keys := make([]ui.KeyboardKey, 0, len(letters))
	for _, letter := range letters {
		value := string(letter)
		keys = append(keys, ui.KeyboardKey{
			Label:  strings.ToUpper(value),
			Letter: strings.ToUpper(value),
			Value:  value,
		})
	}
	return keys
}

func keyboardDelays(guesses []ScoredGuess, event renderEvent) map[string]int {
	delays := map[string]int{}
	if event.Kind != "reveal" && event.Kind != "win" {
		return delays
	}
	if event.RowIndex < 0 || event.RowIndex >= len(guesses) {
		return delays
	}
	for index, tile := range guesses[event.RowIndex].Tiles {
		letter := strings.ToLower(tile.Letter)
		if _, exists := delays[letter]; !exists {
			delays[letter] = index * 100
		}
	}
	return delays
}

func flattenKeyboard(rows []ui.KeyboardRow) []ui.KeyboardKey {
	keys := []ui.KeyboardKey{}
	for _, row := range rows {
		keys = append(keys, row.Keys...)
	}
	return keys
}

func revealAnswer(state GameState) string {
	if state.Status == StatusActive {
		return ""
	}
	return strings.ToUpper(state.Answer)
}

func defaultMessage(state GameState) string {
	switch state.Status {
	case StatusWon:
		return "You solved today's word."
	case StatusLost:
		return "Answer: " + strings.ToUpper(state.Answer)
	case StatusActive:
		return "Daily word for " + state.Date + "."
	default:
		return "Daily Wordle."
	}
}

type notifier struct {
	mu   sync.Mutex
	subs map[chan notifierEvent]struct{}
}

func newNotifier() *notifier {
	return &notifier{subs: map[chan notifierEvent]struct{}{}}
}

func (n *notifier) subscribe() chan notifierEvent {
	ch := make(chan notifierEvent, 1)
	n.mu.Lock()
	n.subs[ch] = struct{}{}
	n.mu.Unlock()
	return ch
}

func (n *notifier) unsubscribe(ch chan notifierEvent) {
	n.mu.Lock()
	delete(n.subs, ch)
	close(ch)
	n.mu.Unlock()
}

func (n *notifier) notify(event notifierEvent) {
	n.mu.Lock()
	defer n.mu.Unlock()
	for ch := range n.subs {
		select {
		case ch <- event:
		default:
		}
	}
}
