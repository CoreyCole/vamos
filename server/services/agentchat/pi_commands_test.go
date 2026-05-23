package agentchat

import (
	"context"
	"database/sql"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/labstack/echo/v4"

	wruntime "github.com/CoreyCole/vamos/pkg/agents/workflows/runtime"
	"github.com/CoreyCole/vamos/pkg/db"
)

func TestFilterSlashCommandsNormalizesSlashAndFiltersPrefix(t *testing.T) {
	commands := []SlashCommandProjection{
		{Name: "skill:datastar", Source: "skill"},
		{Name: "/q-design", Source: "prompt"},
		{Name: "/deploy", Source: "extension"},
		{Name: "model", Source: "vamos"},
	}

	got := filterSlashCommands(commands, "/q")
	if len(got) != 1 || got[0].Name != "/q-design" {
		t.Fatalf("filterSlashCommands(/q) = %#v, want only /q-design", got)
	}

	got = filterSlashCommands(commands, "data")
	if len(got) != 1 || got[0].Name != "/skill:datastar" {
		t.Fatalf("filterSlashCommands(data) = %#v, want /skill:datastar", got)
	}
}

func TestSkillPromptFallbackDiscoversProjectAndGlobalCommands(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	writeTestFile(
		t,
		filepath.Join(home, ".pi", "agent", "skills", "global-skill", "SKILL.md"),
		"---\nname: global-skill\ndescription: Global skill\n---\n",
	)
	writeTestFile(
		t,
		filepath.Join(home, ".pi", "agent", "prompts", "global-review.md"),
		"---\ndescription: Global review\nargument-hint: '[scope]'\n---\nBody",
	)

	project := t.TempDir()
	writeTestFile(t, filepath.Join(project, ".git", "keep"), "")
	writeTestFile(
		t,
		filepath.Join(project, ".agents", "skills", "datastar", "SKILL.md"),
		"---\nname: datastar\ndescription: Datastar UI\n---\n",
	)
	writeTestFile(
		t,
		filepath.Join(project, ".agents", "skills", "ignored-root.md"),
		"---\nname: ignored-root\ndescription: ignored\n---\n",
	)
	writeTestFile(
		t,
		filepath.Join(project, ".pi", "prompts", "q-design.md"),
		"---\ndescription: Design prompt\n---\nPrompt body",
	)

	service := &Service{defaultCwd: project}
	got, err := service.ListSlashCommands(
		context.Background(),
		ListSlashCommandsInput{Cwd: project},
	)
	if err != nil {
		t.Fatalf("ListSlashCommands() error = %v", err)
	}

	assertCommandPresent(t, got, "/skill:datastar", "skill", "project")
	assertCommandPresent(t, got, "/skill:global-skill", "skill", "user")
	assertCommandPresent(t, got, "/q-design", "prompt", "project")
	assertCommandPresent(t, got, "/global-review", "prompt", "user")
	assertCommandAbsent(t, got, "/skill:ignored-root")
	assertCommandAbsent(t, got, "/model")
}

func TestListWorkspaceSlashCommandsUsesWorkflowCwdAndExcludesBuiltins(t *testing.T) {
	service := newTestAgentChatService(t)
	discovery := &recordingCommandDiscovery{commands: []SlashCommandProjection{
		{
			Name:        "skill:datastar",
			Source:      "skill",
			Description: "Datastar",
			SourceInfo:  SlashCommandSourceInfo{Scope: "project", Source: "skill"},
		},
		{Name: "model", Source: "builtin"},
	}}
	service.WithPiCommandDiscovery(discovery)
	handler := NewHandler(service, nil)

	implementationCwd := t.TempDir()
	workspace := mustCreateWorkspaceForHandlerTest(t, service, "user@example.com")
	state := wruntime.State{
		Type:          string(WorkspaceWorkflowQRSPI),
		CurrentNodeID: "implement",
		ExecutionCwd:  implementationCwd,
		Status:        wruntime.WorkspaceStatusRunning,
	}
	stateJSON, err := json.Marshal(state)
	if err != nil {
		t.Fatalf("Marshal(state) error = %v", err)
	}
	workspace.WorkflowType = string(WorkspaceWorkflowQRSPI)
	workspace.WorkflowStateJson = sql.NullString{String: string(stateJSON), Valid: true}
	if err := service.queries.UpdateWorkspaceWorkflowState(
		t.Context(),
		db.UpdateWorkspaceWorkflowStateParams{
			ID:                workspace.ID,
			WorkflowType:      workspace.WorkflowType,
			WorkflowStateJson: workspace.WorkflowStateJson,
		},
	); err != nil {
		t.Fatalf("UpdateWorkspaceWorkflowState() error = %v", err)
	}

	req := httptest.NewRequest(
		http.MethodGet,
		"/agent-chat/"+workspace.ID+"/slash-commands?prefix=/skill",
		nil,
	)
	rec := httptest.NewRecorder()
	c := echo.New().NewContext(req, rec)
	c.Set("user_email", "user@example.com")
	c.SetParamNames("workspace_id")
	c.SetParamValues(workspace.ID)

	if err := handler.ListWorkspaceSlashCommands(c); err != nil {
		t.Fatalf("ListWorkspaceSlashCommands() error = %v", err)
	}
	if discovery.lastCwd != implementationCwd {
		t.Fatalf(
			"discovery cwd = %q, want implementation cwd %q",
			discovery.lastCwd,
			implementationCwd,
		)
	}
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, body = %s", rec.Code, rec.Body.String())
	}
	if body := rec.Body.String(); !strings.Contains(body, "/skill:datastar") ||
		strings.Contains(body, "/model") {
		t.Fatalf("body = %s, want skill command and no built-in /model", body)
	}
}

type recordingCommandDiscovery struct {
	commands []SlashCommandProjection
	lastCwd  string
}

func (d *recordingCommandDiscovery) ListSlashCommands(
	_ context.Context,
	input ListSlashCommandsInput,
) ([]SlashCommandProjection, error) {
	d.lastCwd = input.Cwd
	return d.commands, nil
}

func writeTestFile(t *testing.T, path, content string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatalf("MkdirAll(%s) error = %v", filepath.Dir(path), err)
	}
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatalf("WriteFile(%s) error = %v", path, err)
	}
}

func assertCommandPresent(
	t *testing.T,
	commands []SlashCommandProjection,
	name, source, scope string,
) {
	t.Helper()
	for _, command := range commands {
		if command.Name == name && command.Source == source &&
			command.SourceInfo.Scope == scope {
			return
		}
	}
	t.Fatalf(
		"missing command name=%s source=%s scope=%s in %#v",
		name,
		source,
		scope,
		commands,
	)
}

func assertCommandAbsent(t *testing.T, commands []SlashCommandProjection, name string) {
	t.Helper()
	for _, command := range commands {
		if command.Name == name {
			t.Fatalf("unexpected command %s in %#v", name, commands)
		}
	}
}
