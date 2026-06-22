package main

import (
	"context"
	"encoding/csv"
	"errors"
	"fmt"
	"html/template"
	"log"
	"net"
	"net/http"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"time"
)

type Player struct {
	Name  string
	Skill int
}

type Matchup struct {
	Court  int
	TeamA  []Player
	TeamB  []Player
	Reason string
}

type PageData struct {
	Players  []Player
	Matchups []Matchup
	Message  string
}

func main() {
	root := FilesRoot()
	if err := EnsureStarterFiles(root); err != nil {
		log.Fatalf("prepare files: %v", err)
	}

	mux := Routes(root)
	addr := "127.0.0.1:" + strings.TrimSpace(os.Getenv("PORT"))
	if strings.HasSuffix(addr, ":") {
		addr += "8080"
	}

	ln, err := net.Listen("tcp", addr)
	if err != nil {
		log.Fatalf("listen: %v", err)
	}
	log.Printf("pickleball app listening on http://%s", ln.Addr())
	if err := http.Serve(ln, mux); err != nil && !errors.Is(err, http.ErrServerClosed) {
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

func Routes(root string) http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("/healthz", func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte("ok\n"))
	})
	mux.HandleFunc("/events", func(w http.ResponseWriter, r *http.Request) {
		ServeEvents(r.Context(), w, root)
	})
	mux.HandleFunc("/matchups.csv", func(w http.ResponseWriter, r *http.Request) {
		http.ServeFile(w, r, SafeJoin(root, "matchups.csv"))
	})
	mux.HandleFunc("/tournament.html", func(w http.ResponseWriter, r *http.Request) {
		http.ServeFile(w, r, SafeJoin(root, "tournament.html"))
	})
	mux.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/" {
			http.NotFound(w, r)
			return
		}
		if err := RenderTournament(w, root); err != nil {
			http.Error(w, "I couldn't update the tournament safely. Your files are unchanged.", http.StatusInternalServerError)
		}
	})
	return mux
}

func RenderTournament(w http.ResponseWriter, root string) error {
	html, err := TournamentHTML(root)
	if err != nil {
		return err
	}
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	_, err = w.Write([]byte(html))
	return err
}

func TournamentHTML(root string) (string, error) {
	players, err := LoadPlayers(SafeJoin(root, "players.csv"))
	if err != nil {
		return "", err
	}
	matchups := GenerateMatchups(players)
	if err := WriteMatchupsCSV(root, matchups); err != nil {
		return "", err
	}
	if err := WriteTournamentHTML(root, matchups); err != nil {
		return "", err
	}
	var b strings.Builder
	if err := pageTemplate.Execute(&b, PageData{
		Players:  players,
		Matchups: matchups,
		Message:  "Balanced schedule ready. Edit players.csv or ask Chat to change the tournament rules.",
	}); err != nil {
		return "", err
	}
	return b.String(), nil
}

func ServeEvents(ctx context.Context, w http.ResponseWriter, root string) {
	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache")
	w.Header().Set("Connection", "keep-alive")

	html, err := TournamentHTML(root)
	if err != nil {
		fmt.Fprintf(w, "event: datastar-patch-elements\ndata: <div id=\"tournament-status\">Your app is unchanged.</div>\n\n")
		return
	}
	fmt.Fprintf(w, "event: datastar-patch-elements\ndata: %s\n\n", strings.ReplaceAll(html, "\n", ""))
	if f, ok := w.(http.Flusher); ok {
		f.Flush()
	}
	select {
	case <-ctx.Done():
	case <-time.After(25 * time.Millisecond):
	}
}

func LoadPlayers(path string) ([]Player, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer f.Close()

	records, err := csv.NewReader(f).ReadAll()
	if err != nil {
		return nil, err
	}
	players := make([]Player, 0, len(records))
	for i, row := range records {
		if i == 0 && len(row) >= 2 && strings.EqualFold(strings.TrimSpace(row[0]), "name") {
			continue
		}
		if len(row) < 2 {
			continue
		}
		name := strings.TrimSpace(row[0])
		if name == "" {
			continue
		}
		skill, err := strconv.Atoi(strings.TrimSpace(row[1]))
		if err != nil {
			return nil, fmt.Errorf("skill for %s: %w", name, err)
		}
		players = append(players, Player{Name: name, Skill: skill})
	}
	return players, nil
}

func GenerateMatchups(players []Player) []Matchup {
	sorted := append([]Player(nil), players...)
	sort.SliceStable(sorted, func(i, j int) bool {
		if sorted[i].Skill == sorted[j].Skill {
			return sorted[i].Name < sorted[j].Name
		}
		return sorted[i].Skill > sorted[j].Skill
	})

	matchups := make([]Matchup, 0, len(sorted)/4)
	for court := 1; len(sorted) >= 4; court++ {
		group := sorted[:4]
		sorted = sorted[4:]
		matchups = append(matchups, Matchup{
			Court:  court,
			TeamA:  []Player{group[0], group[3]},
			TeamB:  []Player{group[1], group[2]},
			Reason: "Balanced total skill by pairing high+lower players.",
		})
	}
	return matchups
}

func WriteMatchupsCSV(root string, matchups []Matchup) error {
	path := SafeJoin(root, "matchups.csv")
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	f, err := os.Create(path)
	if err != nil {
		return err
	}
	defer f.Close()

	w := csv.NewWriter(f)
	defer w.Flush()
	if err := w.Write([]string{"court", "team_a", "team_b", "reason"}); err != nil {
		return err
	}
	for _, matchup := range matchups {
		if err := w.Write([]string{
			strconv.Itoa(matchup.Court),
			TeamNames(matchup.TeamA),
			TeamNames(matchup.TeamB),
			matchup.Reason,
		}); err != nil {
			return err
		}
	}
	return w.Error()
}

func WriteTournamentHTML(root string, matchups []Matchup) error {
	path := SafeJoin(root, "tournament.html")
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	f, err := os.Create(path)
	if err != nil {
		return err
	}
	defer f.Close()
	return tournamentFileTemplate.Execute(f, matchups)
}

func EnsureStarterFiles(root string) error {
	if err := os.MkdirAll(root, 0o755); err != nil {
		return err
	}
	players := SafeJoin(root, "players.csv")
	if _, err := os.Stat(players); errors.Is(err, os.ErrNotExist) {
		return os.WriteFile(players, []byte(defaultPlayersCSV), 0o644)
	}
	return nil
}

func SafeJoin(root, rel string) string {
	root = filepath.Clean(root)
	candidate := filepath.Clean(filepath.Join(root, rel))
	relToRoot, err := filepath.Rel(root, candidate)
	if err != nil || relToRoot == ".." || strings.HasPrefix(relToRoot, ".."+string(filepath.Separator)) || filepath.IsAbs(rel) {
		panic("path escapes pickleball files root")
	}
	return candidate
}

func TeamNames(players []Player) string {
	names := make([]string, len(players))
	for i, player := range players {
		names[i] = fmt.Sprintf("%s (%d)", player.Name, player.Skill)
	}
	return strings.Join(names, " + ")
}

const defaultPlayersCSV = `name,skill
Avery,9
Blake,6
Casey,7
Devon,5
Emerson,8
Finley,4
Gray,6
Harper,7
`

var pageTemplate = template.Must(template.New("page").Funcs(template.FuncMap{"team": TeamNames}).Parse(`<!doctype html>
<html lang="en">
<head>
  <meta charset="utf-8">
  <meta name="viewport" content="width=device-width, initial-scale=1">
  <title>Pickleball tournament</title>
  <style>
    :root { color-scheme: light; font-family: Inter, ui-sans-serif, system-ui, -apple-system, BlinkMacSystemFont, "Segoe UI", sans-serif; }
    body { margin: 0; min-height: 100vh; background: linear-gradient(135deg, #ecfeff, #f7fee7); color: #0f172a; }
    main { max-width: 980px; margin: 0 auto; padding: 24px 16px 44px; }
    header { display: grid; gap: 10px; margin-bottom: 20px; }
    h1 { margin: 0; font-size: clamp(2.25rem, 8vw, 4.5rem); line-height: .92; letter-spacing: -0.06em; }
    p { margin: 0; color: #475569; }
    .actions { display: flex; flex-wrap: wrap; gap: 10px; margin-top: 12px; }
    .pill { border-radius: 999px; background: #0f766e; color: white; font-weight: 750; padding: 8px 13px; text-decoration: none; }
    .grid { display: grid; gap: 14px; grid-template-columns: repeat(auto-fit, minmax(245px, 1fr)); }
    .card { border: 1px solid rgba(15, 23, 42, .12); border-radius: 24px; background: rgba(255, 255, 255, .86); box-shadow: 0 18px 45px rgba(15, 23, 42, .08); padding: 18px; }
    .court { display: inline-flex; border-radius: 999px; background: #14b8a6; color: #042f2e; font-weight: 850; padding: 6px 12px; font-size: .86rem; }
    .teams { display: grid; gap: 10px; margin: 16px 0; }
    .team { border-radius: 18px; background: #f8fafc; padding: 12px; }
    .label { color: #64748b; font-size: .75rem; font-weight: 800; text-transform: uppercase; letter-spacing: .08em; }
    .names { margin-top: 4px; font-weight: 750; }
    .reason { border-left: 4px solid #14b8a6; padding-left: 10px; font-size: .92rem; }
  </style>
</head>
<body>
  <main id="pickleball-app" data-signals="{regenerating:false}">
    <header>
      <h1>Tonight's pickleball courts</h1>
      <p id="tournament-status">{{.Message}}</p>
      <div class="actions">
        <a class="pill" href="/events" data-on-click="@get('/events')">Refresh schedule</a>
        <a class="pill" href="/matchups.csv">Download matchups</a>
      </div>
    </header>
    <section class="grid" aria-label="Pickleball matchup cards">
      {{range .Matchups}}
      <article class="card">
        <span class="court">Court {{.Court}}</span>
        <div class="teams">
          <div class="team"><div class="label">Team A</div><div class="names">{{team .TeamA}}</div></div>
          <div class="team"><div class="label">Team B</div><div class="names">{{team .TeamB}}</div></div>
        </div>
        <p class="reason">{{.Reason}}</p>
      </article>
      {{else}}
      <article class="card"><p>No complete four-player courts yet. Add more players to players.csv.</p></article>
      {{end}}
    </section>
  </main>
</body>
</html>`))

var tournamentFileTemplate = template.Must(template.New("tournament-file").Funcs(template.FuncMap{"team": TeamNames}).Parse(`<!doctype html>
<html lang="en">
<head><meta charset="utf-8"><title>Pickleball tournament</title></head>
<body>
<h1>Pickleball tournament</h1>
{{range .}}
<section>
  <h2>Court {{.Court}}</h2>
  <p>Team A: {{team .TeamA}}</p>
  <p>Team B: {{team .TeamB}}</p>
  <p>{{.Reason}}</p>
</section>
{{else}}
<p>No complete four-player courts yet.</p>
{{end}}
</body>
</html>`))
