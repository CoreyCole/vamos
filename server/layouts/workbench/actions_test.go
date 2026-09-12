package workbench

import (
	"bytes"
	"strings"
	"testing"
)

func TestOverflowActionsRendersLinksAndFormModes(t *testing.T) {
	t.Parallel()

	args := OverflowActionsArgs{
		Label: "Document actions",
		Groups: []OverflowActionGroup{{
			Label: "Document",
			Actions: []OverflowAction{
				{
					Label:        "Copy document",
					Kind:         OverflowActionButton,
					ClientAction: "navigator.clipboard.writeText('demo')",
				},
				{
					Label:      "Comment",
					Kind:       OverflowActionForm,
					FormAction: "/forms/comments/show",
					SubmitMode: OverflowActionSubmitDatastar,
					HiddenFields: map[string]string{
						"doc_path":      "thoughts/demo.md",
						"section_hint":  "document",
						"selected_text": "",
					},
				},
				{
					Label:      "Restart",
					Kind:       OverflowActionForm,
					FormAction: "/forms/applets/demo/restart",
					FormMethod: "post",
					SubmitMode: OverflowActionSubmitNative,
				},
				{
					Label:  "Open in new tab",
					Kind:   OverflowActionLink,
					Href:   "/thoughts/_render/app/demo/app/",
					Target: "_blank",
					Rel:    "noopener",
				},
			},
		}},
	}

	var body bytes.Buffer
	if err := OverflowActions(args).Render(t.Context(), &body); err != nil {
		t.Fatalf("OverflowActions.Render() error = %v", err)
	}
	html := body.String()

	for _, want := range []string{
		`data-testid="workbench-overflow-actions"`,
		`data-overflow-trigger`,
		`data-overflow-menu`,
		`data-on:click="navigator.clipboard.writeText(&#39;demo&#39;)"`,
		`data-on:submit__prevent="el.closest(&#39;[data-overflow-menu]&#39;)?.style.setProperty(&#39;display&#39;,&#39;none&#39;); @post(&#39;/forms/comments/show&#39;, {contentType: &#39;form&#39;})"`,
		`method="post" action="/forms/applets/demo/restart"`,
		`target="_blank" rel="noopener"`,
	} {
		if !strings.Contains(html, want) {
			t.Fatalf("OverflowActions html = %s, want %q", html, want)
		}
	}
	if strings.Contains(html, `@post(&#39;/forms/applets/demo/restart&#39;`) {
		t.Fatalf("native lifecycle form rendered as Datastar post: %s", html)
	}
	if strings.Contains(html, "<details") || strings.Contains(html, "<summary") {
		t.Fatalf("overflow actions must be a portaled dropdown, not details: %s", html)
	}
}

func TestChatHeaderShareOverflowLabels(t *testing.T) {
	var body strings.Builder
	if err := ChatHeaderShareOverflow().Render(t.Context(), &body); err != nil {
		t.Fatal(err)
	}
	html := body.String()
	for _, want := range []string{
		`data-testid="workbench-overflow-actions"`,
		"Share artifact",
		"Share chat",
		`aria-label="Share"`,
		"writeText",
		"clipboard_success",
	} {
		if !strings.Contains(html, want) {
			t.Fatalf("ChatHeaderShareOverflow missing %q in %s", want, html)
		}
	}
	if strings.Contains(html, ">Share</p>") || strings.Contains(html, ">Artifact</p>") {
		t.Fatalf("flat menu must not paint section headers: %s", html)
	}
}


func TestArtifactReloadButtonResetsIframeSrcOnly(t *testing.T) {
	var body strings.Builder
	if err := ArtifactReloadButton().Render(t.Context(), &body); err != nil {
		t.Fatal(err)
	}
	html := body.String()
	for _, want := range []string{
		`data-testid="artifact-reload"`,
		`aria-label="Reload"`,
		`thread-artifact-document`,
		`applet-frame-`,
		`.src=`,
	} {
		if !strings.Contains(html, want) {
			t.Fatalf("ArtifactReloadButton missing %q in %s", want, html)
		}
	}
	for _, bad := range []string{"/forms/applets/", "Restart", "pull-to-refresh", "PTR"} {
		if strings.Contains(html, bad) {
			t.Fatalf("Reload must not include %q: %s", bad, html)
		}
	}
}

func TestChatHeaderShareOverflowOmitsReload(t *testing.T) {
	var body strings.Builder
	if err := ChatHeaderShareOverflow().Render(t.Context(), &body); err != nil {
		t.Fatal(err)
	}
	html := body.String()
	if strings.Contains(html, `data-testid="artifact-reload"`) || strings.Contains(html, ">Reload<") {
		t.Fatalf("HARD LOCK A: chat Share overflow must omit Reload: %s", html)
	}
}

func TestArtifactReloadClickActionDebouncesBusy(t *testing.T) {
	js := ArtifactReloadClickAction()
	for _, want := range []string{
		"__vamosArtifactReloadBusy",
		`addEventListener("load"`,
		"setTimeout(done,2000)",
		"thread-artifact-document",
		".src=",
	} {
		if !strings.Contains(js, want) {
			t.Fatalf("ArtifactReloadClickAction missing %q in %s", want, js)
		}
	}
	for _, bad := range []string{"/forms/applets/", "Restart", "agent-chat-scroll-region"} {
		if strings.Contains(js, bad) {
			t.Fatalf("click reload must not include %q: %s", bad, js)
		}
	}
}

func TestArtifactReloadPTRBootstrapMessageAndOverscroll(t *testing.T) {
	js := ArtifactReloadPanePTRBootstrap()
	for _, want := range []string{
		"__vamosArtifactPTRBound",
		`e.data.type !== "vamos:ptr"`,
		`overscrollBehaviorY = "contain"`,
		"e.source !== f.contentWindow",
		"#thread-artifact-pane",
		"threshold = 56",
		"__vamosArtifactReloadBusy",
		"#agent-chat-scroll-region",
	} {
		if !strings.Contains(js, want) {
			t.Fatalf("PTR bootstrap missing %q in %s", want, js)
		}
	}
	// HARD LOCK A / soft parks: bootstrap must not invent chat Reload or /static promotion markers.
	for _, bad := range []string{"/static/", "allow-same-origin", "chat ⋯", "thread-artifact-reload-mobile"} {
		if strings.Contains(js, bad) {
			t.Fatalf("PTR bootstrap must not include %q: %s", bad, js)
		}
	}
}

func TestArtifactReloadPTRScriptRendersOnceGuard(t *testing.T) {
	var body strings.Builder
	if err := ArtifactReloadPTRScript().Render(t.Context(), &body); err != nil {
		t.Fatal(err)
	}
	html := body.String()
	for _, want := range []string{
		`data-vamos-artifact-ptr="1"`,
		"vamos:ptr",
		"overscrollBehaviorY",
		"__vamosArtifactPTRBound",
	} {
		if !strings.Contains(html, want) {
			t.Fatalf("ArtifactReloadPTRScript missing %q in %s", want, html)
		}
	}
}

