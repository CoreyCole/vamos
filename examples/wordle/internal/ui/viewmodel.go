package ui

type PageData struct {
	Auth    AuthView
	Game    GameView
	Message string
}

type AuthView struct {
	LoggedIn bool
	Username string
	Timezone string
}

type GameView struct {
	PuzzleDate   string
	Status       string
	Rows         []GuessRow
	KeyboardRows []KeyboardRow
	Keyboard     []KeyboardKey
	AttemptsUsed int
	AttemptsMax  int
	CanGuess     bool
	Answer       string
	CurrentRow   int
	RenderEvent  RenderEventView
}

type GuessRow struct {
	Index     int
	Tiles     []TileView
	Submitted bool
	Current   bool
	Animation string
}

type TileView struct {
	Index     int
	Letter    string
	State     string
	DelayMS   int
	Animation string
}

type KeyboardRow struct {
	Keys []KeyboardKey
}

type KeyboardKey struct {
	Label   string
	Value   string
	State   string
	Wide    bool
	Spacer  bool
	DelayMS int
	Letter  string
}

type RenderEventView struct {
	ID       string
	Kind     string
	RowIndex int
	Message  string
}
