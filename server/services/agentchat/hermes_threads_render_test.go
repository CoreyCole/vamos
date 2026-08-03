package agentchat

import (
	"bytes"
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/CoreyCole/vamos/server/services/markdown"
)

func writeHermesRenderFixture(
	t *testing.T,
	planDir string,
	identity HermesPlanIdentity,
	threadID, creator, authority, content string,
) {
	t.Helper()
	metadata := hermesMetadataFixture(identity, threadID)
	metadata.CreatorEmail = creator
	metadata.PromptAuthority.PrincipalValue = authority
	metadata.Title = "Thread " + threadID
	if err := AppendHermesTranscript(planDir, metadata); err != nil {
		t.Fatal(err)
	}
	if err := AppendHermesTranscript(planDir, HermesTranscriptEvent{
		ID: "user_" + threadID, At: time.Now().UTC(), Type: "user",
		ThreadID: threadID, PlanDir: identity, Content: content,
	}); err != nil {
		t.Fatal(err)
	}
}

func TestRenderHermesThreadsPanelListsSelectedTranscriptAndReadOnlyAuthority(t *testing.T) {
	root := t.TempDir()
	identity := HermesPlanIdentity("owner/plans/alpha")
	planDir := filepath.Join(root, filepath.FromSlash(string(identity)))
	if err := os.MkdirAll(planDir, 0o700); err != nil {
		t.Fatal(err)
	}
	writeHermesRenderFixture(t, planDir, identity, "older", "creator@example.com", "authority@example.com", "older transcript")
	olderPath, err := HermesTranscriptPath(planDir, "older")
	if err != nil {
		t.Fatal(err)
	}
	if err := os.Chtimes(olderPath, time.Unix(1, 0), time.Unix(1, 0)); err != nil {
		t.Fatal(err)
	}
	writeHermesRenderFixture(t, planDir, identity, "newer", "other@example.com", "other@example.com", "newer transcript")
	service := &Service{thoughtsRoot: root}
	request := markdown.HermesThreadsRenderRequest{
		UserEmail: "reader@example.com", DocPath: "owner/plans/alpha/design.md",
		SelectedPlanPath: "owner/plans/alpha", PlanHint: "thoughts/owner/plans/alpha",
		CurrentURL: "/thoughts/owner/plans/alpha/design.md?context=threads&hermes_thread=older",
		LinkState: markdown.ThoughtsWorkbenchLinkState{
			Context: "threads", ChatWorkspaceID: "ws", ChatThreadID: "chat", ChatRunID: "run",
			HermesThreadID: "older", SelectedPlanPath: "owner/plans/alpha",
		},
	}

	component, replacement, err := service.RenderHermesThreadsPanel(t.Context(), request)
	if err != nil {
		t.Fatal(err)
	}
	if replacement.URL != "" {
		t.Fatalf("unexpected replacement = %q", replacement.URL)
	}
	var buffer bytes.Buffer
	if err := component.Render(context.Background(), &buffer); err != nil {
		t.Fatal(err)
	}
	html := buffer.String()
	if strings.Index(html, "Thread newer") > strings.Index(html, "Thread older") {
		t.Fatalf("threads are not in updated order: %s", html)
	}
	for _, want := range []string{"older transcript", "Shared read-only transcript.", "chat_workspace=ws", "thread=chat", "run=run"} {
		if !strings.Contains(html, want) {
			t.Fatalf("render missing %q: %s", want, html)
		}
	}
	for _, unwanted := range []string{"newer transcript", "<form", "<textarea", "New Thread", "threadFreeform"} {
		if strings.Contains(html, unwanted) {
			t.Fatalf("render contains %q: %s", unwanted, html)
		}
	}
}

func TestRenderHermesThreadsPanelShowsAuthorityWithoutComposer(t *testing.T) {
	root := t.TempDir()
	identity := HermesPlanIdentity("owner/plans/alpha")
	planDir := filepath.Join(root, filepath.FromSlash(string(identity)))
	if err := os.MkdirAll(planDir, 0o700); err != nil {
		t.Fatal(err)
	}
	writeHermesRenderFixture(t, planDir, identity, "owned", "creator@example.com", "authority@example.com", "visible")
	service := &Service{thoughtsRoot: root}
	component, _, err := service.RenderHermesThreadsPanel(t.Context(), markdown.HermesThreadsRenderRequest{
		UserEmail: "AUTHORITY@example.com", DocPath: "owner/plans/alpha/design.md",
		SelectedPlanPath: "owner/plans/alpha",
		LinkState: markdown.ThoughtsWorkbenchLinkState{
			Context: "threads", HermesThreadID: "owned", SelectedPlanPath: "owner/plans/alpha",
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	var buffer bytes.Buffer
	if err := component.Render(context.Background(), &buffer); err != nil {
		t.Fatal(err)
	}
	html := buffer.String()
	if !strings.Contains(html, "You have prompt authority") || strings.Contains(html, "<form") {
		t.Fatalf("authority read-only render = %s", html)
	}
}

func TestRenderHermesThreadsPanelRejectsMalformedMissingAndCrossPlanSelection(t *testing.T) {
	root := t.TempDir()
	for _, identity := range []HermesPlanIdentity{"owner/plans/alpha", "owner/plans/beta"} {
		planDir := filepath.Join(root, filepath.FromSlash(string(identity)))
		if err := os.MkdirAll(planDir, 0o700); err != nil {
			t.Fatal(err)
		}
		writeHermesRenderFixture(t, planDir, identity, "same", "creator@example.com", "authority@example.com", string(identity))
	}
	service := &Service{thoughtsRoot: root}
	for _, selected := range []string{"bad/id", "missing"} {
		request := markdown.HermesThreadsRenderRequest{
			DocPath: "owner/plans/alpha/design.md", SelectedPlanPath: "owner/plans/alpha",
			CurrentURL: "/thoughts/owner/plans/alpha/design.md?context=threads&thread=chat&hermes_thread=" + selected,
			LinkState: markdown.ThoughtsWorkbenchLinkState{
				Context: "threads", ChatThreadID: "chat", HermesThreadID: selected,
				SelectedPlanPath: "owner/plans/alpha",
			},
		}
		component, replacement, err := service.RenderHermesThreadsPanel(t.Context(), request)
		if err != nil {
			t.Fatal(err)
		}
		if component == nil || strings.Contains(replacement.URL, "hermes_thread=") || !strings.Contains(replacement.URL, "thread=chat") {
			t.Fatalf("selected %q replacement = %q", selected, replacement.URL)
		}
	}
	component, replacement, err := service.RenderHermesThreadsPanel(t.Context(), markdown.HermesThreadsRenderRequest{
		DocPath: "owner/plans/beta/design.md", SelectedPlanPath: "owner/plans/beta",
		CurrentURL: "/thoughts/owner/plans/beta/design.md?context=threads&hermes_thread=same",
		LinkState: markdown.ThoughtsWorkbenchLinkState{
			Context: "threads", HermesThreadID: "same", SelectedPlanPath: "owner/plans/beta",
		},
	})
	if err != nil || replacement.URL != "" {
		t.Fatalf("valid equal-ID target error/replacement = %v/%q", err, replacement.URL)
	}
	var buffer bytes.Buffer
	if err := component.Render(context.Background(), &buffer); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(buffer.String(), "owner/plans/beta") || strings.Contains(buffer.String(), "owner/plans/alpha") {
		t.Fatalf("equal-ID target crossed plans: %s", buffer.String())
	}
}

func TestRenderHermesThreadsPanelRejectsDifferentContainedPlanHint(t *testing.T) {
	root := t.TempDir()
	for _, plan := range []string{"owner/plans/alpha", "owner/plans/beta"} {
		if err := os.MkdirAll(filepath.Join(root, filepath.FromSlash(plan)), 0o700); err != nil {
			t.Fatal(err)
		}
	}
	service := &Service{thoughtsRoot: root}
	_, _, err := service.RenderHermesThreadsPanel(t.Context(), markdown.HermesThreadsRenderRequest{
		DocPath: "owner/plans/alpha/design.md", SelectedPlanPath: "owner/plans/alpha",
		PlanHint: "thoughts/owner/plans/beta",
	})
	if err == nil || !strings.Contains(err.Error(), "does not identify selected plan") {
		t.Fatalf("different plan hint error = %v", err)
	}
}

func TestRenderHermesThreadsPanelNoPlanDoesNotDiscoverRoot(t *testing.T) {
	service := &Service{thoughtsRoot: filepath.Join(t.TempDir(), "missing")}
	component, replacement, err := service.RenderHermesThreadsPanel(t.Context(), markdown.HermesThreadsRenderRequest{
		DocPath: "notes.md", NoPlan: true,
		CurrentURL: "/thoughts/notes.md?context=threads&hermes_thread=stale",
		LinkState:  markdown.ThoughtsWorkbenchLinkState{Context: "threads", HermesThreadID: "stale"},
	})
	if err != nil || component == nil || strings.Contains(replacement.URL, "hermes_thread") {
		t.Fatalf("component/replacement/error = %v/%q/%v", component, replacement.URL, err)
	}
}
