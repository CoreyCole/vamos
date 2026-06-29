package main

import (
	"bufio"
	"context"
	"errors"
	"fmt"
	"log"
	"math/rand"
	"net"
	"net/http"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"time"
)

const maxAttempts = 6
const wordLength = 5

type LetterMark string

const (
	MarkGreen  LetterMark = "green"
	MarkYellow LetterMark = "yellow"
	MarkGray   LetterMark = "gray"
)

type LetterResult struct {
	Letter string
	Mark   LetterMark
}

type Guess struct {
	Word    string
	Letters []LetterResult
}

type Game struct {
	Answer  string
	Guesses []Guess
	Message string
	Over    bool
	Won     bool
}

type App struct {
	root       string
	validWords map[string]bool
	answers    []string

	mu          sync.Mutex
	game        Game
	subscribers map[chan struct{}]struct{}
}

func main() {
	root := FilesRoot()
	if err := EnsureStarterFiles(root); err != nil {
		log.Fatalf("prepare files: %v", err)
	}

	app, err := NewApp(root)
	if err != nil {
		log.Fatalf("load word lists: %v", err)
	}

	addr := "127.0.0.1:" + strings.TrimSpace(os.Getenv("PORT"))
	if strings.HasSuffix(addr, ":") {
		addr += "8080"
	}

	ln, err := net.Listen("tcp", addr)
	if err != nil {
		log.Fatalf("listen: %v", err)
	}
	log.Printf("wordle app listening on http://%s", ln.Addr())
	if err := http.Serve(ln, app.Routes()); err != nil && !errors.Is(err, http.ErrServerClosed) {
		log.Fatalf("serve: %v", err)
	}
}

func FilesRoot() string {
	if root := strings.TrimSpace(os.Getenv("VAMOS_APP_FILES_ROOT")); root != "" {
		return filepath.Clean(root)
	}
	wd, err := os.Getwd()
	if err != nil {
		return filepath.Clean("../..")
	}
	return filepath.Clean(filepath.Join(wd, "..", ".."))
}

func NewApp(root string) (*App, error) {
	validWords, err := LoadWordSet(SafeJoin(root, "valid_words.txt"))
	if err != nil {
		return nil, err
	}
	answers, err := LoadWordList(SafeJoin(root, "answers.txt"))
	if err != nil {
		return nil, err
	}
	if len(answers) == 0 {
		return nil, errors.New("answers.txt has no five-letter words")
	}
	app := &App{
		root:        root,
		validWords:  validWords,
		answers:     answers,
		subscribers: make(map[chan struct{}]struct{}),
	}
	app.game = app.newGameLocked()
	return app, nil
}

func (a *App) Routes() http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("/healthz", func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte("ok\n"))
	})
	mux.HandleFunc("/events", a.HandleEvents)
	mux.HandleFunc("/guess", a.HandleGuess)
	mux.HandleFunc("/new", a.HandleNewGame)
	mux.HandleFunc("/", a.HandleHome)
	return mux
}

func (a *App) HandleHome(w http.ResponseWriter, r *http.Request) {
	if r.URL.Path != "/" {
		http.NotFound(w, r)
		return
	}
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	if err := Page(a.State()).Render(r.Context(), w); err != nil {
		http.Error(w, "I couldn't draw the game board safely.", http.StatusInternalServerError)
	}
}

func (a *App) HandleEvents(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache")
	w.Header().Set("Connection", "keep-alive")

	ch := a.subscribe()
	defer a.unsubscribe(ch)
	a.patchGame(w)

	for {
		select {
		case <-ch:
			a.patchGame(w)
		case <-r.Context().Done():
			return
		}
	}
}

func (a *App) HandleGuess(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	if err := r.ParseForm(); err != nil {
		http.Error(w, "invalid form", http.StatusBadRequest)
		return
	}
	a.SubmitGuess(r.FormValue("guess"))
	if wantsDatastar(r) {
		w.WriteHeader(http.StatusNoContent)
		return
	}
	http.Redirect(w, r, ".", http.StatusSeeOther)
}

func (a *App) HandleNewGame(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	a.ResetGame()
	if wantsDatastar(r) {
		w.WriteHeader(http.StatusNoContent)
		return
	}
	http.Redirect(w, r, ".", http.StatusSeeOther)
}

func wantsDatastar(r *http.Request) bool {
	return strings.Contains(r.Header.Get("Accept"), "text/event-stream")
}

func (a *App) SubmitGuess(raw string) {
	a.mu.Lock()
	guess := normalizeWord(raw)
	switch {
	case a.game.Over:
		a.game.Message = "This game is finished. Start a new game to keep playing."
	case len(guess) != wordLength:
		a.game.Message = "Enter exactly five letters."
	case !a.validWords[guess]:
		a.game.Message = strings.ToUpper(guess) + " is not in the Wordle Helper dictionary."
	default:
		result := ScoreGuess(guess, a.game.Answer)
		a.game.Guesses = append(a.game.Guesses, Guess{Word: strings.ToUpper(guess), Letters: result})
		if guess == a.game.Answer {
			a.game.Won = true
			a.game.Over = true
			a.game.Message = fmt.Sprintf("Solved in %d!", len(a.game.Guesses))
		} else if len(a.game.Guesses) >= maxAttempts {
			a.game.Over = true
			a.game.Message = "Game over. The word was " + strings.ToUpper(a.game.Answer) + "."
		} else {
			a.game.Message = fmt.Sprintf("%d guesses left.", maxAttempts-len(a.game.Guesses))
		}
	}
	a.mu.Unlock()
	a.notify()
}

func (a *App) ResetGame() {
	a.mu.Lock()
	a.game = a.newGameLocked()
	a.mu.Unlock()
	a.notify()
}

func (a *App) State() Game {
	a.mu.Lock()
	defer a.mu.Unlock()
	game := a.game
	game.Guesses = append([]Guess(nil), a.game.Guesses...)
	return game
}

func (a *App) newGameLocked() Game {
	answer := a.answers[rand.New(rand.NewSource(time.Now().UnixNano())).Intn(len(a.answers))]
	return Game{Answer: answer, Message: "Guess the hidden five-letter word."}
}

func (a *App) subscribe() chan struct{} {
	ch := make(chan struct{}, 1)
	a.mu.Lock()
	a.subscribers[ch] = struct{}{}
	a.mu.Unlock()
	return ch
}

func (a *App) unsubscribe(ch chan struct{}) {
	a.mu.Lock()
	delete(a.subscribers, ch)
	close(ch)
	a.mu.Unlock()
}

func (a *App) notify() {
	a.mu.Lock()
	defer a.mu.Unlock()
	for ch := range a.subscribers {
		select {
		case ch <- struct{}{}:
		default:
		}
	}
}

func (a *App) patchGame(w http.ResponseWriter) {
	var b strings.Builder
	if err := GameView(a.State()).Render(context.Background(), &b); err != nil {
		return
	}
	fmt.Fprintf(w, "event: datastar-patch-elements\ndata: elements %s\n\n", oneLine(b.String()))
	if f, ok := w.(http.Flusher); ok {
		f.Flush()
	}
}

func ScoreGuess(guess, answer string) []LetterResult {
	guess = normalizeWord(guess)
	answer = normalizeWord(answer)
	result := make([]LetterResult, wordLength)
	remaining := make(map[byte]int)

	for i := 0; i < wordLength; i++ {
		letter := guess[i]
		result[i] = LetterResult{Letter: strings.ToUpper(string(letter)), Mark: MarkGray}
		if letter == answer[i] {
			result[i].Mark = MarkGreen
		} else {
			remaining[answer[i]]++
		}
	}

	for i := 0; i < wordLength; i++ {
		if result[i].Mark == MarkGreen {
			continue
		}
		letter := guess[i]
		if remaining[letter] > 0 {
			result[i].Mark = MarkYellow
			remaining[letter]--
		}
	}
	return result
}

func LoadWordSet(path string) (map[string]bool, error) {
	words, err := LoadWordList(path)
	if err != nil {
		return nil, err
	}
	set := make(map[string]bool, len(words))
	for _, word := range words {
		set[word] = true
	}
	return set, nil
}

func LoadWordList(path string) ([]string, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer f.Close()

	seen := map[string]bool{}
	words := []string{}
	scanner := bufio.NewScanner(f)
	for scanner.Scan() {
		word := normalizeWord(scanner.Text())
		if len(word) != wordLength || seen[word] {
			continue
		}
		seen[word] = true
		words = append(words, word)
	}
	if err := scanner.Err(); err != nil {
		return nil, err
	}
	sort.Strings(words)
	return words, nil
}

func EnsureStarterFiles(root string) error {
	if err := os.MkdirAll(root, 0o755); err != nil {
		return err
	}
	files := map[string]string{
		"valid_words.txt": defaultValidWords,
		"answers.txt":     defaultAnswers,
	}
	for name, contents := range files {
		path := SafeJoin(root, name)
		if _, err := os.Stat(path); errors.Is(err, os.ErrNotExist) {
			if err := os.WriteFile(path, []byte(contents), 0o644); err != nil {
				return err
			}
		} else if err != nil {
			return err
		}
	}
	return nil
}

func SafeJoin(root, rel string) string {
	root = filepath.Clean(root)
	candidate := filepath.Clean(filepath.Join(root, rel))
	relToRoot, err := filepath.Rel(root, candidate)
	if err != nil || relToRoot == ".." || strings.HasPrefix(relToRoot, ".."+string(filepath.Separator)) || filepath.IsAbs(rel) {
		panic("path escapes wordle files root")
	}
	return candidate
}

func normalizeWord(word string) string {
	word = strings.ToLower(strings.TrimSpace(word))
	var b strings.Builder
	for _, r := range word {
		if r >= 'a' && r <= 'z' {
			b.WriteRune(r)
		}
	}
	return b.String()
}

func oneLine(html string) string {
	return strings.Join(strings.Fields(html), " ")
}

var defaultAnswers = `apple
berry
cigar
crane
delta
eagle
flame
ghost
grace
house
index
jelly
knock
lemon
mango
noble
ocean
piano
queen
raven
solar
trace
ultra
vivid
whale
yacht
zebra
`

var defaultValidWords = defaultAnswers + `
about
above
abuse
actor
acute
admit
adopt
after
again
agent
agree
ahead
alarm
album
alert
alien
align
alike
alive
allow
alone
along
alter
amber
angel
anger
angle
angry
apart
arena
argue
arise
armed
arose
aside
asset
audio
audit
avoid
award
aware
badly
baker
bases
basic
beach
began
begin
being
below
bench
birth
black
blame
blank
blend
bless
blind
block
blood
board
boost
booth
bound
brain
brand
brave
bread
break
brick
bride
brief
bring
broad
broke
brown
build
built
buyer
cable
carry
catch
cause
chain
chair
chart
chase
cheap
check
chest
chief
child
claim
class
clean
clear
click
climb
clock
close
cloud
coach
coast
could
count
court
cover
craft
crash
crazy
cream
crime
cross
crowd
crown
curve
cycle
daily
dance
dated
dealt
death
debut
delay
depth
doing
doubt
dozen
draft
drama
dream
dress
drink
drive
drove
dying
early
earth
eight
elite
empty
enemy
enjoy
enter
entry
equal
error
event
every
exact
exist
extra
faith
false
fault
fiber
field
fifth
fifty
fight
final
first
fixed
flash
fleet
floor
fluid
focus
force
forth
forty
forum
found
frame
fresh
front
fruit
fully
funny
giant
given
glass
globe
going
grand
grant
grass
great
green
gross
group
grown
guard
guess
guest
guide
happy
heart
heavy
hence
horse
hotel
human
ideal
image
imply
input
issue
joint
judge
known
label
large
laser
later
laugh
layer
learn
lease
least
leave
legal
level
light
limit
local
logic
loose
lucky
lunch
major
maker
march
match
maybe
mayor
meant
media
metal
might
minor
model
money
month
moral
motor
mount
mouse
mouth
movie
music
needs
never
night
noise
north
noted
novel
nurse
occur
offer
often
order
other
ought
paint
panel
paper
party
peace
phase
phone
photo
piece
pilot
pitch
place
plain
plane
plant
plate
point
pound
power
press
price
pride
prime
print
prior
prize
proof
proud
prove
quick
quiet
quite
radio
raise
range
rapid
ratio
reach
ready
refer
right
rival
river
rough
round
route
royal
rural
scale
scene
scope
score
sense
serve
seven
shall
shape
share
sharp
sheet
shelf
shell
shift
shirt
shock
shoot
short
shown
sight
silly
since
sixth
sixty
skill
sleep
slide
small
smart
smile
solid
solve
sorry
sound
south
space
spare
speak
speed
spend
spent
split
spoke
sport
staff
stage
stake
stand
start
state
steam
steel
stick
still
stock
stone
stood
store
storm
story
strip
stuck
study
stuff
style
sugar
suite
super
sweet
table
taken
taste
teach
thank
theme
there
thick
thing
think
third
those
three
throw
tight
times
tired
title
today
topic
total
touch
tough
tower
track
trade
train
treat
trend
trial
tried
truck
truly
trust
truth
twice
under
union
unity
until
upper
upset
urban
usage
usual
valid
value
video
visit
voice
waste
watch
water
wheel
where
which
while
white
whole
whose
woman
world
worry
worth
would
write
wrong
young
`
