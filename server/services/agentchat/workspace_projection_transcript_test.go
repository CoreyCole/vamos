package agentchat

import (
	"strings"
	"testing"

	"github.com/CoreyCole/vamos/pkg/agents/chatsession"
)

func TestTranscriptFromChatProjectionRendersToolWriteEditRows(t *testing.T) {
	service := newTestAgentChatService(t)
	items := service.transcriptFromChatProjection(chatsession.ChatProjection{
		SessionID: "session-1",
		Messages:  []chatsession.ProjectedMessage{{ID: "prompt", Seq: 1, Role: "user", Content: "prompt visible"}},
		Tools:     []chatsession.ProjectedToolEvent{{ID: "tool-1", Seq: 2, Name: "bash", Status: "complete", Summary: "ran command"}},
		Artifacts: []chatsession.ProjectedArtifact{
			{Path: "e2e/created.txt", Kind: "written", Seq: 3},
			{Path: "e2e/updated.txt", Kind: "edited", Seq: 4},
		},
	})
	joined := make([]string, 0, len(items))
	for _, item := range items {
		joined = append(joined, item.Title+" "+item.Content+" "+item.HeaderCode+" "+item.HeaderSummary)
	}
	body := strings.Join(joined, "\n")
	for _, want := range []string{"prompt visible", "bash", "ran command", "file write", "e2e/created.txt", "file edit", "e2e/updated.txt"} {
		if !strings.Contains(body, want) {
			t.Fatalf("projection transcript missing %q in:\n%s", want, body)
		}
	}
}
