package pickleball

import (
	"context"
	"fmt"
	"strings"

	servercfg "github.com/CoreyCole/vamos/server"
	"github.com/CoreyCole/vamos/server/services/agentchat"
	serverauth "github.com/CoreyCole/vamos/server/services/auth"
)

const defaultPickleballProjectID = "github.com/CoreyCole/vamos"

// AgentChatRunStarter is the Agent Chat machine-run subset used by the
// pickleball applet editor. agentchat.Service satisfies this interface.
type AgentChatRunStarter interface {
	StartCLIChatRun(context.Context, serverauth.MachineAPIActor, servercfg.ProjectCheckoutResolution, agentchat.ChatStartRequest, string) (agentchat.ChatRunRef, error)
}

type AgentChatEditor struct {
	Starter       AgentChatRunStarter
	ActorEmail    string
	ProjectID     string
	PublicBaseURL string
}

func (e AgentChatEditor) ApplyPrompt(ctx context.Context, input AppletEditInput) (AppletEditResult, error) {
	if e.Starter == nil {
		return AppletEditResult{FailureUserMessage: unchangedFailureMessage()}, fmt.Errorf("agent chat editor is not configured")
	}
	actorEmail := firstNonEmptyString(input.UserEmail, e.ActorEmail, "pickleball-agent@example.invalid")
	projectID := firstNonEmptyString(e.ProjectID, defaultPickleballProjectID)
	prompt := BuildAgentChatAppletPrompt(input)
	ref, err := e.Starter.StartCLIChatRun(
		ctx,
		serverauth.MachineAPIActor{ActorEmail: actorEmail},
		servercfg.ProjectCheckoutResolution{ProjectID: projectID, RootPath: input.IterationDir},
		agentchat.ChatStartRequest{ProjectID: projectID, Prompt: prompt},
		e.PublicBaseURL,
	)
	if err != nil {
		return AppletEditResult{FailureUserMessage: unchangedFailureMessage()}, fmt.Errorf("start agent chat applet edit: %w", err)
	}
	return AppletEditResult{
		ChatSessionID:      ref.ChatSessionID,
		ThreadID:           ref.ThreadID,
		RunID:              ref.RunID,
		WebURL:             ref.WebURL,
		ChangedFiles:       nil,
		UserSummary:        "I'm working on that change with the app editor.",
		FailureUserMessage: unchangedFailureMessage(),
	}, nil
}

func BuildAgentChatAppletPrompt(input AppletEditInput) string {
	var b strings.Builder
	b.WriteString("You are editing the Vamos pickleball applet for a non-technical user.\n")
	b.WriteString("\nUser request:\n")
	b.WriteString(strings.TrimSpace(input.Prompt))
	b.WriteString("\n\nEdit target:\n")
	fmt.Fprintf(&b, "- Applet source dir: %s\n", strings.TrimSpace(input.IterationDir))
	fmt.Fprintf(&b, "- Files root: %s\n", strings.TrimSpace(input.FilesRoot))
	fmt.Fprintf(&b, "- Current app dir: %s\n", strings.TrimSpace(input.CurrentAppDir))
	b.WriteString("\nRules:\n")
	b.WriteString("- Edit only the applet source dir and safe files under the Files root.\n")
	b.WriteString("- Preserve a friendly pickleball organizer experience.\n")
	b.WriteString("- Hide code, builds, branches, run IDs, manifests, promotion, and filesystem paths from the normal user.\n")
	b.WriteString("- Run `go test ./...` from the applet source dir before reporting success.\n")
	b.WriteString("- Return a short non-technical summary and list changed files.\n")
	b.WriteString("- If unsafe or failing, explain only that the app is unchanged.\n")
	return b.String()
}

func firstNonEmptyString(values ...string) string {
	for _, value := range values {
		if trimmed := strings.TrimSpace(value); trimmed != "" {
			return trimmed
		}
	}
	return ""
}
