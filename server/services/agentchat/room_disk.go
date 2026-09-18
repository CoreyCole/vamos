package agentchat

import (
	"bufio"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"gopkg.in/yaml.v3"

	conversation "github.com/CoreyCole/vamos/pkg/agents/conversation"
	"github.com/CoreyCole/vamos/pkg/agents/roster"
	"github.com/CoreyCole/vamos/pkg/db"
)

const (
	RoomKindBotHome  = "bot_home"
	RoomKindPairwise = "pairwise"
	RoomKindPlan     = "plan"

	currentJSONLName = "current.jsonl"
	sessionsDirName  = "sessions"
	historyDirName   = "history"
	handoffsDirName  = "handoffs"
	agentDirMaxDesc  = 400
)

type RoomIdentity struct {
	Kind        string
	SpeakerSlug string
	PairA       string
	PairB       string
	PlanDirRel  string
}

func CanonicalPairSlugs(a, b string) (string, string, error) {
	a = strings.TrimSpace(a)
	b = strings.TrimSpace(b)
	if a == "" || b == "" {
		return "", "", fmt.Errorf("pairwise slugs are required")
	}
	if a == b {
		return "", "", fmt.Errorf("pairwise slugs must differ")
	}
	if a < b {
		return a, b, nil
	}
	return b, a, nil
}

func PairwiseDirName(a, b string) (string, error) {
	left, right, err := CanonicalPairSlugs(a, b)
	if err != nil {
		return "", err
	}
	return left + "__" + right, nil
}

func (id RoomIdentity) ThoughtsRelRoot() (string, error) {
	switch strings.TrimSpace(id.Kind) {
	case RoomKindBotHome:
		slug := strings.TrimSpace(id.SpeakerSlug)
		if err := validateSlug(slug); err != nil {
			return "", err
		}
		return "thoughts/agents/" + slug, nil
	case RoomKindPairwise:
		dir, err := PairwiseDirName(id.PairA, id.PairB)
		if err != nil {
			return "", err
		}
		return "thoughts/a2a/" + dir, nil
	case RoomKindPlan:
		plan, err := CanonicalThoughtsPath(id.PlanDirRel)
		if err != nil {
			return "", fmt.Errorf("plan dir: %w", err)
		}
		return plan, nil
	default:
		return "", fmt.Errorf("unknown room kind %q", id.Kind)
	}
}

func (id RoomIdentity) SessionsRel() (string, error) {
	root, err := id.ThoughtsRelRoot()
	if err != nil {
		return "", err
	}
	if id.Kind == RoomKindPlan {
		return root + "/.vamos/" + sessionsDirName, nil
	}
	return root + "/" + sessionsDirName, nil
}

func (id RoomIdentity) CurrentJSONLRel() (string, error) {
	sessions, err := id.SessionsRel()
	if err != nil {
		return "", err
	}
	return sessions + "/" + currentJSONLName, nil
}

func (id RoomIdentity) HandoffsRel() (string, error) {
	sessions, err := id.SessionsRel()
	if err != nil {
		return "", err
	}
	return sessions + "/" + handoffsDirName, nil
}

func (id RoomIdentity) HistoryRel() (string, error) {
	sessions, err := id.SessionsRel()
	if err != nil {
		return "", err
	}
	return sessions + "/" + historyDirName, nil
}

func AbsFromThoughtsRel(thoughtsRoot, thoughtsRel string) (string, error) {
	rel, err := CanonicalThoughtsPath(thoughtsRel)
	if err != nil {
		return "", err
	}
	root := strings.TrimSpace(thoughtsRoot)
	if root == "" {
		return "", fmt.Errorf("thoughts root is required")
	}
	under := strings.TrimPrefix(rel, "thoughts/")
	return filepath.Join(root, filepath.FromSlash(under)), nil
}

func EnsureRoomCurrentJSONL(
	thoughtsRoot string,
	id RoomIdentity,
) (absPath string, err error) {
	rel, err := id.CurrentJSONLRel()
	if err != nil {
		return "", err
	}
	absPath, err = AbsFromThoughtsRel(thoughtsRoot, rel)
	if err != nil {
		return "", err
	}
	if err := os.MkdirAll(filepath.Dir(absPath), 0o755); err != nil {
		return "", err
	}
	return absPath, nil
}

func RoomCwdAbs(thoughtsRoot string, id RoomIdentity) (string, error) {
	rel, err := id.ThoughtsRelRoot()
	if err != nil {
		return "", err
	}
	return AbsFromThoughtsRel(thoughtsRoot, rel)
}

func RoomIdentityFromThread(
	thoughtsRoot string,
	thread db.AgentThread,
	speakerSlug string,
) (RoomIdentity, error) {
	speakerSlug = strings.TrimSpace(speakerSlug)
	cwd := strings.TrimSpace(thread.Cwd)
	root := strings.TrimSpace(thoughtsRoot)

	if root != "" && cwd != "" {
		if rel, ok := thoughtsRelFromAbs(root, cwd); ok {
			if id, ok := parseAgentsOrA2A(rel, speakerSlug); ok {
				return id, nil
			}
		}
	}

	planRel := strings.TrimSpace(thread.PlanDirRel.String)
	if thread.PlanDirRel.Valid && planRel != "" {
		if !strings.HasPrefix(filepath.ToSlash(planRel), "thoughts/") {
			planRel = "thoughts/" + strings.TrimPrefix(filepath.ToSlash(planRel), "/")
		}
		canon, err := CanonicalThoughtsPath(planRel)
		if err != nil {
			return RoomIdentity{}, err
		}
		return RoomIdentity{
			Kind:        RoomKindPlan,
			SpeakerSlug: speakerSlug,
			PlanDirRel:  canon,
		}, nil
	}

	if root != "" && cwd != "" {
		if rel, ok := thoughtsRelFromAbs(root, cwd); ok {
			return RoomIdentity{
				Kind:        RoomKindPlan,
				SpeakerSlug: speakerSlug,
				PlanDirRel:  rel,
			}, nil
		}
	}

	return RoomIdentity{}, fmt.Errorf(
		"cannot classify agent chat room from thread %s",
		thread.ID,
	)
}

func parseAgentsOrA2A(thoughtsRel, speakerSlug string) (RoomIdentity, bool) {
	rel := strings.Trim(filepath.ToSlash(thoughtsRel), "/")
	const agentsPrefix = "thoughts/agents/"
	if strings.HasPrefix(rel, agentsPrefix) {
		rest := strings.TrimPrefix(rel, agentsPrefix)
		slug, _, _ := strings.Cut(rest, "/")
		if validateSlug(slug) != nil {
			return RoomIdentity{}, false
		}
		if speakerSlug == "" {
			speakerSlug = slug
		}
		return RoomIdentity{Kind: RoomKindBotHome, SpeakerSlug: speakerSlug}, true
	}
	const a2aPrefix = "thoughts/a2a/"
	if strings.HasPrefix(rel, a2aPrefix) {
		rest := strings.TrimPrefix(rel, a2aPrefix)
		dir, _, _ := strings.Cut(rest, "/")
		a, b, ok := strings.Cut(dir, "__")
		if !ok || validateSlug(a) != nil || validateSlug(b) != nil {
			return RoomIdentity{}, false
		}
		left, right, err := CanonicalPairSlugs(a, b)
		if err != nil {
			return RoomIdentity{}, false
		}
		return RoomIdentity{
			Kind:        RoomKindPairwise,
			SpeakerSlug: speakerSlug,
			PairA:       left,
			PairB:       right,
		}, true
	}
	return RoomIdentity{}, false
}

func thoughtsRelFromAbs(thoughtsRoot, abs string) (string, bool) {
	rel, err := filepath.Rel(thoughtsRoot, abs)
	if err != nil {
		return "", false
	}
	rel = filepath.ToSlash(rel)
	if rel == ".." || strings.HasPrefix(rel, "../") {
		return "", false
	}
	if rel == "." {
		return "thoughts", true
	}
	return "thoughts/" + rel, true
}

func ValidateAgentSlug(slug string) error {
	return validateSlug(slug)
}

func validateSlug(slug string) error {
	return roster.ValidateSlug(slug)
}

// SeedBotHomeTree writes thoughts/agents/{slug}/ role files and sessions dirs.
func SeedBotHomeTree(thoughtsRoot, slug, name string) error {
	if err := validateSlug(slug); err != nil {
		return err
	}
	id := RoomIdentity{Kind: RoomKindBotHome, SpeakerSlug: slug}
	root, err := RoomCwdAbs(thoughtsRoot, id)
	if err != nil {
		return err
	}
	skills := filepath.Join(root, "skills")
	history := filepath.Join(root, sessionsDirName, historyDirName)
	handoffs := filepath.Join(root, sessionsDirName, handoffsDirName)
	for _, dir := range []string{skills, history, handoffs} {
		if err := os.MkdirAll(dir, 0o755); err != nil {
			return err
		}
	}
	display := strings.TrimSpace(name)
	if display == "" {
		display = slug
	}
	files := map[string]string{
		"AGENTS.md": "# " + display + "\n\nRole memory lives in MEMORY.md. Do not embed the live roster here.\n",
		"MEMORY.md": "# Memory\n",
		"USER.md":   "# User\n",
	}
	for fileName, body := range files {
		path := filepath.Join(root, fileName)
		if _, err := os.Stat(path); err == nil {
			continue
		} else if !os.IsNotExist(err) {
			return err
		}
		if err := os.WriteFile(path, []byte(body), 0o644); err != nil {
			return err
		}
	}
	rel, err := id.CurrentJSONLRel()
	if err != nil {
		return err
	}
	current, err := AbsFromThoughtsRel(thoughtsRoot, rel)
	if err != nil {
		return err
	}
	if _, err := os.Stat(current); os.IsNotExist(err) {
		if err := os.WriteFile(current, nil, 0o644); err != nil {
			return err
		}
	} else if err != nil {
		return err
	}
	return nil
}

// SeedPairwiseTree writes thoughts/a2a/{a}__{b}/sessions only (no standing notebook).
func SeedPairwiseTree(thoughtsRoot, a, b string) error {
	left, right, err := CanonicalPairSlugs(a, b)
	if err != nil {
		return err
	}
	if err := validateSlug(left); err != nil {
		return err
	}
	if err := validateSlug(right); err != nil {
		return err
	}
	id := RoomIdentity{Kind: RoomKindPairwise, PairA: left, PairB: right}
	root, err := RoomCwdAbs(thoughtsRoot, id)
	if err != nil {
		return err
	}
	history := filepath.Join(root, sessionsDirName, historyDirName)
	handoffs := filepath.Join(root, sessionsDirName, handoffsDirName)
	for _, dir := range []string{history, handoffs} {
		if err := os.MkdirAll(dir, 0o755); err != nil {
			return err
		}
	}
	rel, err := id.CurrentJSONLRel()
	if err != nil {
		return err
	}
	current, err := AbsFromThoughtsRel(thoughtsRoot, rel)
	if err != nil {
		return err
	}
	if _, err := os.Stat(current); os.IsNotExist(err) {
		if err := os.WriteFile(current, nil, 0o644); err != nil {
			return err
		}
	} else if err != nil {
		return err
	}
	return nil
}

type AgentRosterRow struct {
	Slug        string
	Name        string
	Label       string
	Description string
}

func BuildWindowInjectFiles(
	thoughtsRoot string,
	id RoomIdentity,
	roster []AgentRosterRow,
) ([]conversation.InjectFile, error) {
	var files []conversation.InjectFile

	if strings.TrimSpace(id.SpeakerSlug) != "" {
		agentsMDRel := "thoughts/agents/" + id.SpeakerSlug + "/AGENTS.md"
		if content, ok := readThoughtsFile(thoughtsRoot, agentsMDRel); ok {
			files = append(
				files,
				conversation.InjectFile{Path: agentsMDRel, Content: content},
			)
		}
	}

	files = append(files, conversation.InjectFile{
		Path:    "agent-directory.md",
		Content: renderAgentDirectory(id.SpeakerSlug, roster),
	})

	handoffRel, handoffBody, listed, err := latestRoomHandoff(thoughtsRoot, id)
	if err != nil {
		return nil, err
	}
	if handoffRel != "" {
		files = append(
			files,
			conversation.InjectFile{Path: handoffRel, Content: handoffBody},
		)
		for _, listedPath := range listed {
			content, ok := readThoughtsFile(thoughtsRoot, listedPath)
			if !ok {
				continue
			}
			files = append(
				files,
				conversation.InjectFile{Path: listedPath, Content: content},
			)
		}
	}
	return files, nil
}

func latestRoomHandoff(
	thoughtsRoot string,
	id RoomIdentity,
) (rel, body string, files []string, err error) {
	handoffsRel, err := id.HandoffsRel()
	if err != nil {
		return "", "", nil, err
	}
	abs, err := AbsFromThoughtsRel(thoughtsRoot, handoffsRel)
	if err != nil {
		return "", "", nil, err
	}
	entries, err := os.ReadDir(abs)
	if err != nil {
		if os.IsNotExist(err) {
			return "", "", nil, nil
		}
		return "", "", nil, err
	}
	var names []string
	for _, entry := range entries {
		if entry.IsDir() {
			continue
		}
		name := entry.Name()
		if strings.HasSuffix(name, ".md") {
			names = append(names, name)
		}
	}
	if len(names) == 0 {
		return "", "", nil, nil
	}
	sort.Strings(names)
	name := names[len(names)-1]
	rel = handoffsRel + "/" + name
	raw, ok := readThoughtsFile(thoughtsRoot, rel)
	if !ok {
		return "", "", nil, nil
	}
	listed, body := parseHandoffFiles(raw)
	return rel, body, listed, nil
}

func parseHandoffFiles(raw string) (files []string, body string) {
	body = raw
	trimmed := strings.TrimSpace(raw)
	if !strings.HasPrefix(trimmed, "---") {
		return nil, raw
	}
	rest := strings.TrimPrefix(trimmed, "---")
	rest = strings.TrimPrefix(rest, "\n")
	end := strings.Index(rest, "\n---")
	if end < 0 {
		return nil, raw
	}
	front := rest[:end]
	body = strings.TrimPrefix(rest[end+len("\n---"):], "\n")
	var meta struct {
		Files []string `yaml:"files"`
	}
	if err := yaml.Unmarshal([]byte(front), &meta); err != nil {
		return nil, raw
	}
	for _, f := range meta.Files {
		canon, err := CanonicalThoughtsPath(f)
		if err != nil {
			continue
		}
		files = append(files, canon)
	}
	return files, body
}

func readThoughtsFile(thoughtsRoot, thoughtsRel string) (string, bool) {
	abs, err := AbsFromThoughtsRel(thoughtsRoot, thoughtsRel)
	if err != nil {
		return "", false
	}
	data, err := os.ReadFile(abs)
	if err != nil {
		return "", false
	}
	return string(data), true
}

func renderAgentDirectory(speaker string, roster []AgentRosterRow) string {
	var b strings.Builder
	b.WriteString("# Agent directory\n\n")
	b.WriteString(
		"message_room `to` is a roster slug or a thoughts-relative plan directory. ",
	)
	b.WriteString("Never a thread UUID. Never a bot-home URL.\n\n")
	if len(roster) == 0 {
		b.WriteString("No agents are registered yet.\n")
		return b.String()
	}
	for _, row := range roster {
		slug := strings.TrimSpace(row.Slug)
		if slug == "" {
			continue
		}
		mark := ""
		if slug == speaker {
			mark = " (current speaker)"
		}
		desc := strings.TrimSpace(row.Description)
		if len(desc) > agentDirMaxDesc {
			desc = desc[:agentDirMaxDesc] + "…"
		}
		fmt.Fprintf(&b, "- slug: `%s`%s\n", slug, mark)
		if name := strings.TrimSpace(row.Name); name != "" {
			fmt.Fprintf(&b, "  name: %s\n", name)
		}
		if label := strings.TrimSpace(row.Label); label != "" {
			fmt.Fprintf(&b, "  label: %s\n", label)
		}
		if desc != "" {
			fmt.Fprintf(&b, "  description: %s\n", desc)
		}
	}
	return b.String()
}

// WorkingContextPreview is last user/assistant text from current.jsonl.
type WorkingContextPreview struct {
	Text string
	Time time.Time
}

// LastWorkingContextPreview reads thoughts/agents/{slug}/sessions/current.jsonl.
// Empty or missing file yields empty Text (no fixture copy).
func LastWorkingContextPreview(thoughtsRoot, slug string) WorkingContextPreview {
	id := RoomIdentity{Kind: RoomKindBotHome, SpeakerSlug: strings.TrimSpace(slug)}
	rel, err := id.CurrentJSONLRel()
	if err != nil {
		return WorkingContextPreview{}
	}
	abs, err := AbsFromThoughtsRel(thoughtsRoot, rel)
	if err != nil {
		return WorkingContextPreview{}
	}
	info, err := os.Stat(abs)
	if err != nil {
		return WorkingContextPreview{}
	}
	out := WorkingContextPreview{Time: info.ModTime()}
	file, err := os.Open(abs)
	if err != nil {
		return out
	}
	defer file.Close()
	scanner := bufio.NewScanner(file)
	scanner.Buffer(make([]byte, 0, 64*1024), 1024*1024)
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line == "" {
			continue
		}
		var envelope struct {
			Type      string `json:"type"`
			Timestamp string `json:"timestamp"`
			Message   struct {
				Role    string `json:"role"`
				Content any    `json:"content"`
			} `json:"message"`
		}
		if err := json.Unmarshal([]byte(line), &envelope); err != nil {
			continue
		}
		if envelope.Type != "message" {
			continue
		}
		switch strings.TrimSpace(envelope.Message.Role) {
		case "user", "assistant":
		default:
			continue
		}
		text := previewTextFromContent(envelope.Message.Content)
		if text == "" {
			continue
		}
		out.Text = text
		if ts := parseJSONLTime(envelope.Timestamp); !ts.IsZero() {
			out.Time = ts
		}
	}
	return out
}

func previewTextFromContent(content any) string {
	switch v := content.(type) {
	case string:
		return strings.Join(strings.Fields(v), " ")
	default:
		return ""
	}
}

func parseJSONLTime(raw string) time.Time {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return time.Time{}
	}
	for _, layout := range []string{
		time.RFC3339Nano,
		time.RFC3339,
	} {
		if ts, err := time.Parse(layout, raw); err == nil {
			return ts
		}
	}
	return time.Time{}
}
