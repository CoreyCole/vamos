package main

import (
	"encoding/csv"
	"encoding/json"
	"fmt"
	"html/template"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
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

type Manifest struct {
	SchemaVersion int       `json:"schema_version"`
	BuildID       string    `json:"build_id"`
	Mode          string    `json:"mode"`
	PromptSummary string    `json:"prompt_summary"`
	Artifacts     Artifacts `json:"artifacts"`
}

type Artifacts struct {
	HTML string `json:"html"`
	CSV  string `json:"csv"`
}

func main() {
	outputDir := os.Getenv("VAMOS_GENERATED_OUTPUT_DIR")
	if outputDir == "" {
		outputDir = "out"
	}
	if err := os.MkdirAll(outputDir, 0o755); err != nil {
		panic(err)
	}

	players, err := LoadPlayers("players.csv")
	if err != nil {
		panic(err)
	}
	matchups := GenerateMatchups(players)

	if err := WriteHTML(filepath.Join(outputDir, "app.html"), matchups); err != nil {
		panic(err)
	}
	if err := WriteCSV(filepath.Join(outputDir, "results.csv"), matchups); err != nil {
		panic(err)
	}
	if err := WriteManifest(filepath.Join(outputDir, "manifest.json")); err != nil {
		panic(err)
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
	players := make([]Player, 0, len(records)-1)
	for _, row := range records[1:] {
		if len(row) < 2 {
			continue
		}
		skill, err := strconv.Atoi(strings.TrimSpace(row[1]))
		if err != nil {
			return nil, fmt.Errorf("skill for %s: %w", row[0], err)
		}
		players = append(players, Player{Name: strings.TrimSpace(row[0]), Skill: skill})
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
			Reason: "Balanced total skill by pairing high+low.",
		})
	}
	return matchups
}

func WriteHTML(path string, matchups []Matchup) error {
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	f, err := os.Create(path)
	if err != nil {
		return err
	}
	defer f.Close()
	return pageTemplate.Execute(f, matchups)
}

func WriteCSV(path string, matchups []Matchup) error {
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
			teamNames(matchup.TeamA),
			teamNames(matchup.TeamB),
			matchup.Reason,
		}); err != nil {
			return err
		}
	}
	return w.Error()
}

func WriteManifest(path string) error {
	buildID := os.Getenv("VAMOS_GENERATED_BUILD_ID")
	if buildID == "" {
		buildID = "seed-build"
	}
	manifest := Manifest{
		SchemaVersion: 1,
		BuildID:       buildID,
		Mode:          "one_shot",
		PromptSummary: "Seed balanced matchup generator",
		Artifacts:     Artifacts{HTML: "app.html", CSV: "results.csv"},
	}
	data, err := json.MarshalIndent(manifest, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(path, append(data, '\n'), 0o644)
}

func teamNames(players []Player) string {
	names := make([]string, len(players))
	for i, player := range players {
		names[i] = fmt.Sprintf("%s (%d)", player.Name, player.Skill)
	}
	return strings.Join(names, " + ")
}

var pageTemplate = template.Must(template.New("pickleball").Funcs(template.FuncMap{"team": teamNames}).Parse(`<!doctype html>
<html lang="en">
<head>
  <meta charset="utf-8">
  <meta name="viewport" content="width=device-width, initial-scale=1">
  <title>Pickleball Matchups</title>
  <style>
    :root { color-scheme: light; font-family: Inter, ui-sans-serif, system-ui, -apple-system, BlinkMacSystemFont, "Segoe UI", sans-serif; }
    body { margin: 0; background: linear-gradient(135deg, #ecfeff, #f0fdf4); color: #0f172a; }
    main { max-width: 920px; margin: 0 auto; padding: 24px 16px 40px; }
    header { display: grid; gap: 8px; margin-bottom: 20px; }
    h1 { margin: 0; font-size: clamp(2rem, 7vw, 4rem); line-height: .95; letter-spacing: -0.05em; }
    p { margin: 0; color: #475569; }
    .grid { display: grid; gap: 14px; grid-template-columns: repeat(auto-fit, minmax(240px, 1fr)); }
    .card { border: 1px solid rgba(15, 23, 42, .12); border-radius: 24px; background: rgba(255, 255, 255, .82); box-shadow: 0 18px 45px rgba(15, 23, 42, .08); padding: 18px; }
    .court { display: inline-flex; align-items: center; border-radius: 999px; background: #0f766e; color: white; font-weight: 700; padding: 6px 12px; font-size: .85rem; }
    .teams { display: grid; gap: 10px; margin: 16px 0; }
    .team { border-radius: 18px; background: #f8fafc; padding: 12px; }
    .label { color: #64748b; font-size: .75rem; font-weight: 700; text-transform: uppercase; letter-spacing: .08em; }
    .names { margin-top: 4px; font-weight: 750; }
    .reason { border-left: 4px solid #14b8a6; padding-left: 10px; font-size: .92rem; }
  </style>
</head>
<body>
  <main>
    <header>
      <h1>Tonight's pickleball courts</h1>
      <p>Generated by a one-shot Go bundle. Prompt me to change the matching logic, CSV, or presentation.</p>
    </header>
    <section class="grid" aria-label="Generated matchup cards">
      {{range .}}
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
