package markdown

import (
	"bytes"
	"strings"
	"testing"
)

func TestMarkdownSidebarRendersWorkbenchSectionAttrs(t *testing.T) {
	t.Parallel()

	var body bytes.Buffer
	if err := MarkdownSidebar(
		[]TocItem{{ID: "intro", Text: "Intro", Level: 2}},
	).Render(t.Context(), &body); err != nil {
		t.Fatalf("MarkdownSidebar.Render() error = %v", err)
	}
	html := body.String()
	for _, want := range []string{
		`href="#intro"`,
		`data-workbench-section-link`,
		`data-workbench-section-target="intro"`,
		`data-workbench-section-region="docWorkbenchCenter"`,
		`data-workbench-section-container="thoughts-markdown-scroll-region"`,
	} {
		if !strings.Contains(html, want) {
			t.Fatalf("MarkdownSidebar html = %s, want %q", html, want)
		}
	}
	for _, unwanted := range []string{`onclick=`, `scrollIntoView`, `setTimeout`, `data-doc-section-target`, `workbenchScrollDocumentSection`} {
		if strings.Contains(html, unwanted) {
			t.Fatalf("MarkdownSidebar html = %s, should not contain %q", html, unwanted)
		}
	}
}

func TestMarkdownSidebarMobileRendersWorkbenchSectionAttrs(t *testing.T) {
	t.Parallel()

	var body bytes.Buffer
	if err := MarkdownSidebarMobile(
		[]TocItem{{ID: "mobile-intro", Text: "Mobile Intro", Level: 1}},
	).Render(t.Context(), &body); err != nil {
		t.Fatalf("MarkdownSidebarMobile.Render() error = %v", err)
	}
	html := body.String()
	for _, want := range []string{
		`href="#mobile-intro"`,
		`data-workbench-section-link`,
		`data-workbench-section-target="mobile-intro"`,
		`data-workbench-section-region="docWorkbenchCenter"`,
		`data-workbench-section-container="thoughts-markdown-scroll-region"`,
	} {
		if !strings.Contains(html, want) {
			t.Fatalf("MarkdownSidebarMobile html = %s, want %q", html, want)
		}
	}
	for _, unwanted := range []string{`onclick=`, `scrollIntoView`, `setTimeout`, `data-doc-section-target`, `workbenchScrollDocumentSection`} {
		if strings.Contains(html, unwanted) {
			t.Fatalf(
				"MarkdownSidebarMobile html = %s, should not contain %q",
				html,
				unwanted,
			)
		}
	}
}
