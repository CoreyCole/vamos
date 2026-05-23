package agentchat

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"sort"
	"strings"

	wruntime "github.com/CoreyCole/vamos/pkg/agents/workflows/runtime"
	"github.com/CoreyCole/vamos/pkg/db"
)

type SlashCommandProjection struct {
	Name         string                 `json:"name"`
	Source       string                 `json:"source"`
	Description  string                 `json:"description,omitempty"`
	ArgumentHint string                 `json:"argument_hint,omitempty"`
	SourceInfo   SlashCommandSourceInfo `json:"sourceInfo"`
}

type SlashCommandSourceInfo struct {
	Path    string `json:"path,omitempty"`
	Scope   string `json:"scope,omitempty"`
	Source  string `json:"source,omitempty"`
	Origin  string `json:"origin,omitempty"`
	BaseDir string `json:"baseDir,omitempty"`
}

type ListSlashCommandsInput struct {
	Cwd    string
	Prefix string
}

type PiCommandDiscovery interface {
	ListSlashCommands(
		ctx context.Context,
		input ListSlashCommandsInput,
	) ([]SlashCommandProjection, error)
}

func (s *Service) WorkspaceSlashCommandCwd(workspace db.Workspace) string {
	if WorkspaceWorkflowType(workspace.WorkflowType) == WorkspaceWorkflowQRSPI &&
		workspace.WorkflowStateJson.Valid && strings.TrimSpace(workspace.WorkflowStateJson.String) != "" {
		var state wruntime.State
		if err := json.Unmarshal(
			[]byte(workspace.WorkflowStateJson.String),
			&state,
		); err == nil {
			projected := ProjectWorkspaceCwd(state, workspace)
			if !projected.Blocked && strings.TrimSpace(projected.Path) != "" {
				return projected.Path
			}
		}
	}
	return s.workspaceThreadCwd(workspace)
}

func (s *Service) WithPiCommandDiscovery(discovery PiCommandDiscovery) *Service {
	s.piCommandDiscovery = discovery
	return s
}

func (s *Service) ListSlashCommands(
	ctx context.Context,
	input ListSlashCommandsInput,
) ([]SlashCommandProjection, error) {
	cwd := strings.TrimSpace(input.Cwd)
	if cwd == "" {
		cwd = strings.TrimSpace(s.defaultCwd)
	}
	var commands []SlashCommandProjection
	if s.piCommandDiscovery != nil {
		listed, err := s.piCommandDiscovery.ListSlashCommands(
			ctx,
			ListSlashCommandsInput{Cwd: cwd, Prefix: input.Prefix},
		)
		if err != nil {
			return nil, err
		}
		commands = listed
	} else {
		commands = s.listSkillAndPromptCommandsFallback(cwd)
	}
	return filterSlashCommands(commands, input.Prefix), nil
}

func filterSlashCommands(
	commands []SlashCommandProjection,
	prefix string,
) []SlashCommandProjection {
	prefix = strings.TrimPrefix(strings.TrimSpace(prefix), "/")
	out := make([]SlashCommandProjection, 0, len(commands))
	for _, command := range commands {
		if command.Source == "builtin" {
			continue
		}
		name := strings.TrimPrefix(strings.TrimSpace(command.Name), "/")
		if name == "" {
			continue
		}
		command.Name = "/" + name
		if prefix == "" || strings.HasPrefix(name, prefix) ||
			strings.Contains(name, prefix) {
			out = append(out, command)
		}
	}
	sort.SliceStable(out, func(i, j int) bool {
		if out[i].Source != out[j].Source {
			return slashCommandSourceRank(
				out[i].Source,
			) < slashCommandSourceRank(
				out[j].Source,
			)
		}
		return out[i].Name < out[j].Name
	})
	return out
}

func slashCommandSourceRank(source string) int {
	switch source {
	case "extension":
		return 0
	case "prompt":
		return 1
	case "skill":
		return 2
	case "vamos":
		return 3
	default:
		return 4
	}
}

func (s *Service) listSkillAndPromptCommandsFallback(
	cwd string,
) []SlashCommandProjection {
	var commands []SlashCommandProjection
	home, _ := os.UserHomeDir()
	if home != "" {
		commands = append(
			commands,
			discoverSkillCommands(
				filepath.Join(home, ".pi", "agent", "skills"),
				"user",
				true,
			)...)
		commands = append(
			commands,
			discoverSkillCommands(
				filepath.Join(home, ".agents", "skills"),
				"user",
				false,
			)...)
		commands = append(
			commands,
			discoverPromptCommands(
				filepath.Join(home, ".pi", "agent", "prompts"),
				"user",
			)...)
	}
	for _, dir := range projectResourceDirs(cwd) {
		commands = append(
			commands,
			discoverSkillCommands(
				filepath.Join(dir, ".pi", "skills"),
				"project",
				true,
			)...)
		commands = append(
			commands,
			discoverSkillCommands(
				filepath.Join(dir, ".agents", "skills"),
				"project",
				false,
			)...)
		commands = append(
			commands,
			discoverPromptCommands(filepath.Join(dir, ".pi", "prompts"), "project")...)
	}
	return dedupeSlashCommands(commands)
}

func projectResourceDirs(cwd string) []string {
	cwd = strings.TrimSpace(cwd)
	if cwd == "" {
		return nil
	}
	abs, err := filepath.Abs(cwd)
	if err == nil {
		cwd = abs
	}
	if info, err := os.Stat(cwd); err == nil && !info.IsDir() {
		cwd = filepath.Dir(cwd)
	}
	var dirs []string
	for {
		dirs = append(dirs, cwd)
		if hasGitDir(cwd) {
			break
		}
		parent := filepath.Dir(cwd)
		if parent == cwd {
			break
		}
		cwd = parent
	}
	return dirs
}

func hasGitDir(dir string) bool {
	info, err := os.Stat(filepath.Join(dir, ".git"))
	return err == nil && info.IsDir()
}

func discoverSkillCommands(
	root, scope string,
	includeRootMarkdown bool,
) []SlashCommandProjection {
	info, err := os.Stat(root)
	if err != nil || !info.IsDir() {
		return nil
	}
	var commands []SlashCommandProjection
	if includeRootMarkdown {
		entries, _ := os.ReadDir(root)
		for _, entry := range entries {
			if entry.IsDir() || filepath.Ext(entry.Name()) != ".md" {
				continue
			}
			path := filepath.Join(root, entry.Name())
			if command, ok := skillCommandFromFile(path, scope, root); ok {
				commands = append(commands, command)
			}
		}
	}
	_ = filepath.WalkDir(root, func(path string, d os.DirEntry, err error) error {
		if err != nil || !d.IsDir() {
			return nil
		}
		if d.Name() == ".git" || d.Name() == "node_modules" {
			return filepath.SkipDir
		}
		skillPath := filepath.Join(path, "SKILL.md")
		if _, err := os.Stat(skillPath); err != nil {
			return nil
		}
		if command, ok := skillCommandFromFile(skillPath, scope, root); ok {
			commands = append(commands, command)
		}
		return nil
	})
	return commands
}

func skillCommandFromFile(path, scope, baseDir string) (SlashCommandProjection, bool) {
	frontmatter, body := readMarkdownFrontmatter(path)
	name := strings.TrimSpace(frontmatter["name"])
	if name == "" {
		if filepath.Base(path) == "SKILL.md" {
			name = filepath.Base(filepath.Dir(path))
		} else {
			name = strings.TrimSuffix(filepath.Base(path), filepath.Ext(path))
		}
	}
	if name == "" {
		return SlashCommandProjection{}, false
	}
	description := strings.TrimSpace(frontmatter["description"])
	if description == "" {
		description = firstMarkdownTextLine(body)
	}
	return SlashCommandProjection{
		Name:        "/skill:" + name,
		Source:      "skill",
		Description: description,
		SourceInfo: SlashCommandSourceInfo{
			Path:    path,
			Scope:   scope,
			Source:  "skill",
			Origin:  "top-level",
			BaseDir: baseDir,
		},
	}, true
}

func discoverPromptCommands(root, scope string) []SlashCommandProjection {
	entries, err := os.ReadDir(root)
	if err != nil {
		return nil
	}
	commands := make([]SlashCommandProjection, 0, len(entries))
	for _, entry := range entries {
		if entry.IsDir() || filepath.Ext(entry.Name()) != ".md" {
			continue
		}
		path := filepath.Join(root, entry.Name())
		frontmatter, body := readMarkdownFrontmatter(path)
		name := strings.TrimSuffix(entry.Name(), filepath.Ext(entry.Name()))
		description := strings.TrimSpace(frontmatter["description"])
		if description == "" {
			description = firstMarkdownTextLine(body)
		}
		commands = append(commands, SlashCommandProjection{
			Name:         "/" + name,
			Source:       "prompt",
			Description:  description,
			ArgumentHint: strings.TrimSpace(frontmatter["argument-hint"]),
			SourceInfo: SlashCommandSourceInfo{
				Path:    path,
				Scope:   scope,
				Source:  "prompt",
				Origin:  "top-level",
				BaseDir: root,
			},
		})
	}
	return commands
}

func readMarkdownFrontmatter(path string) (map[string]string, string) {
	content, err := os.ReadFile(path)
	if err != nil {
		return map[string]string{}, ""
	}
	text := strings.ReplaceAll(string(content), "\r\n", "\n")
	if !strings.HasPrefix(text, "---\n") {
		return map[string]string{}, text
	}
	end := strings.Index(text[len("---\n"):], "\n---")
	if end < 0 {
		return map[string]string{}, text
	}
	raw := text[len("---\n") : len("---\n")+end]
	body := strings.TrimPrefix(text[len("---\n")+end:], "\n---")
	body = strings.TrimPrefix(body, "\n")
	frontmatter := make(map[string]string)
	var currentKey string
	for _, line := range strings.Split(raw, "\n") {
		trimmed := strings.TrimSpace(line)
		if trimmed == "" || strings.HasPrefix(trimmed, "#") {
			continue
		}
		if strings.HasPrefix(line, " ") || strings.HasPrefix(line, "\t") {
			if currentKey != "" {
				frontmatter[currentKey] = strings.TrimSpace(
					frontmatter[currentKey] + " " + trimmed,
				)
			}
			continue
		}
		key, value, ok := strings.Cut(trimmed, ":")
		if !ok {
			continue
		}
		currentKey = strings.TrimSpace(key)
		frontmatter[currentKey] = strings.Trim(strings.TrimSpace(value), `"'`)
	}
	return frontmatter, body
}

func firstMarkdownTextLine(body string) string {
	for _, line := range strings.Split(body, "\n") {
		trimmed := strings.TrimSpace(line)
		if trimmed == "" || strings.HasPrefix(trimmed, "#") ||
			strings.HasPrefix(trimmed, "```") {
			continue
		}
		return trimmed
	}
	return ""
}

func dedupeSlashCommands(commands []SlashCommandProjection) []SlashCommandProjection {
	seen := make(map[string]bool, len(commands))
	out := make([]SlashCommandProjection, 0, len(commands))
	for _, command := range commands {
		key := command.Source + "\x00" + strings.TrimPrefix(command.Name, "/")
		if seen[key] {
			continue
		}
		seen[key] = true
		out = append(out, command)
	}
	return out
}
