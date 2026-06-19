//go:build !integration || unit
// +build !integration unit

package commentui

import (
	"bytes"
	"strings"
	"testing"
	"time"
)

func TestSafeHelpersDoNotExposeRawPaths(t *testing.T) {
	t.Parallel()

	slug := SafeCommentTargetSlug("workspace", "plans/2026-05-04_a/b.md")
	if strings.Contains(slug, "/") || strings.Contains(slug, ".") {
		t.Fatalf("SafeCommentTargetSlug() = %q exposes unsafe chars", slug)
	}
	if got := SafeCommentTargetSlug("same"); got != SafeCommentTargetSlug("same") {
		t.Fatalf("SafeCommentTargetSlug not stable: %q", got)
	}
}

func TestSafeSelectionSignalsUseDotNotationIdentifiers(t *testing.T) {
	t.Parallel()

	prefix := SafeSelectionSignalPrefix("artifact", "workspace-1", "plans/raw/path.md")
	if strings.Contains(prefix, "-") || strings.Contains(prefix, "/") ||
		strings.Contains(prefix, ".") {
		t.Fatalf(
			"SafeSelectionSignalPrefix() = %q, want dot-notation-safe identifier",
			prefix,
		)
	}
	if got := SignalKey("artifact-workspace-1", "section-1"); strings.Contains(got, "-") {
		t.Fatalf("SignalKey() = %q, want dot-notation-safe identifier", got)
	}
}

func TestCommentFormSelectedTextIsPreviewOnly(t *testing.T) {
	t.Parallel()

	var buf bytes.Buffer
	target := CommentTargetView{
		ID: "comment-target-doc",
		Routes: CommentRoutes{
			Create: "/create",
			Cancel: "/cancel",
		},
		HiddenFields: map[string]string{
			"artifact_rel_path": "x.md",
			"section_hint":      "section-1",
		},
	}
	err := CommentForm(
		CommentFormView{ID: "comment-form-doc", Target: target, SelectedText: "quoted"},
	).Render(t.Context(), &buf)
	if err != nil {
		t.Fatalf("Render() error = %v", err)
	}
	html := buf.String()
	for _, want := range []string{`name="selected_text" value="quoted"`, "quoted", "contentType", `name="artifact_rel_path"`} {
		if !strings.Contains(html, want) {
			t.Fatalf("render missing %q in %s", want, html)
		}
	}
	if strings.Contains(html, `textarea name="selected_text"`) ||
		strings.Contains(html, `style="`) {
		t.Fatalf("render contains editable selected text or inline style: %s", html)
	}
}

func TestCommentableMarkdownRendersStableTargetsAndHiddenFields(t *testing.T) {
	t.Parallel()

	var buf bytes.Buffer
	args := CommentableMarkdownArgs{
		Surface: CommentSurfaceArtifact,
		IDPrefix: SafeCommentTargetSlug(
			"artifact",
			"workspace-1",
			"plans/raw/path.md",
		),
		DocPath: "plans/raw/path.md",
		Sections: []CommentSectionView{
			{
				ID:          "section-1",
				Title:       "Intro",
				HeadingHTML: "<h1>Intro</h1>",
				BodyHTML:    "<p>Hello</p>",
			},
		},
		Comments: []CommentThreadView{
			{
				ID:          "comment-1",
				AuthorEmail: "author@example.com",
				Body:        "body",
				SectionID:   "section-1",
			},
		},
		Routes: CommentRoutes{
			Show:   "/show",
			Create: "/create",
			Cancel: "/cancel",
			Expand: "/expand",
		},
		HiddenFields: map[string]string{"artifact_rel_path": "plans/raw/path.md"},
		SelectionSignals: SelectionSignalArgs{
			Prefix: SafeSelectionSignalPrefix(
				"artifact",
				"workspace-1",
				"plans/raw/path.md",
			),
			ShowRoute:    "/show",
			HiddenFields: map[string]string{"artifact_rel_path": "plans/raw/path.md"},
		},
	}
	if err := CommentableMarkdown(args).Render(t.Context(), &buf); err != nil {
		t.Fatalf("Render() error = %v", err)
	}
	html := buf.String()
	for _, want := range []string{`data-section-id="section-1"`, `data-comment-target="true"`, `name="artifact_rel_path"`, "Add comment", `contentType`} {
		if !strings.Contains(html, want) {
			t.Fatalf("render missing %q in %s", want, html)
		}
	}
	if strings.Contains(html, `id="plans/raw/path.md`) ||
		strings.Contains(html, ` style="`) {
		t.Fatalf("render exposes raw paths in IDs or inline styles: %s", html)
	}
}

func TestCommentableMarkdownDoesNotRenderFrontmatterSummaryCard(t *testing.T) {
	t.Parallel()

	var buf bytes.Buffer
	err := CommentableMarkdown(CommentableMarkdownArgs{
		Surface:  CommentSurfaceThoughts,
		IDPrefix: SafeCommentTargetSlug("thoughts", "plan.md"),
		DocPath:  "plan.md",
		HTML:     "<p>Body</p>",
		Frontmatter: &CommentFrontmatterView{
			Status: "complete",
			Date:   time.Date(2026, 5, 19, 10, 0, 0, 0, time.UTC),
		},
		Routes: CommentRoutes{
			Show:   "/show",
			Create: "/create",
			Cancel: "/cancel",
		},
	}).Render(t.Context(), &buf)
	if err != nil {
		t.Fatalf("Render() error = %v", err)
	}
	html := buf.String()
	for _, unwanted := range []string{"complete", "Created 2026-05-19"} {
		if strings.Contains(html, unwanted) {
			t.Fatalf(
				"CommentableMarkdown rendered duplicate frontmatter card %q in %s",
				unwanted,
				html,
			)
		}
	}
	if !strings.Contains(html, "<p>Body</p>") {
		t.Fatalf("CommentableMarkdown did not render markdown body: %s", html)
	}
}

func TestCommentableMarkdownDoesNotRenderRightSidebar(t *testing.T) {
	t.Parallel()

	var buf bytes.Buffer
	err := CommentableMarkdown(CommentableMarkdownArgs{
		Surface:  CommentSurfaceArtifact,
		IDPrefix: SafeCommentTargetSlug("artifact", "workspace", "plan.md"),
		DocPath:  "plan.md",
		HTML:     "<p>Body</p>",
		Comments: []CommentThreadView{{
			ID:          "c1",
			AuthorEmail: "a@example.com",
			Body:        "open",
		}},
		Routes: CommentRoutes{
			Show:   "/show",
			Create: "/create",
			Cancel: "/cancel",
		},
	}).Render(t.Context(), &buf)
	if err != nil {
		t.Fatalf("Render() error = %v", err)
	}
	html := buf.String()
	for _, removed := range []string{`id="comment-sidebar"`, `$commentSidebarExpanded`, `commentSidebarExpanded`, `w-80`, `w-10`} {
		if strings.Contains(html, removed) {
			t.Fatalf("render kept removed sidebar marker %q in %s", removed, html)
		}
	}
	if strings.Contains(html, ` style="`) {
		t.Fatalf("render kept inline style: %s", html)
	}
}

func TestCommentableMarkdownMountsEmptyTargetsForPreambleSections(t *testing.T) {
	t.Parallel()

	var buf bytes.Buffer
	err := CommentableMarkdown(CommentableMarkdownArgs{
		Surface:  CommentSurfaceArtifact,
		IDPrefix: SafeCommentTargetSlug("artifact", "workspace", "plan.md"),
		DocPath:  "plan.md",
		Sections: []CommentSectionView{{
			ID:       "section-0",
			Title:    "Section 1",
			BodyHTML: `<div class="markdown-code-block">frontmatter</div>`,
		}},
		Routes: CommentRoutes{Show: "/show", Create: "/create", Cancel: "/cancel"},
	}).Render(t.Context(), &buf)
	if err != nil {
		t.Fatalf("Render() error = %v", err)
	}
	html := buf.String()
	if !strings.Contains(html, `markdown-code-block`) {
		t.Fatalf("render missing preamble body: %s", html)
	}
	for _, want := range []string{`data-comment-target="true"`, `aria-label="Section actions"`, `Add comment`} {
		if !strings.Contains(html, want) {
			t.Fatalf(
				"empty preamble section missing mounted comment target marker %q in %s",
				want,
				html,
			)
		}
	}
}

func TestCommentableMarkdownKeepsReplyTargetsForPreambleThreads(t *testing.T) {
	t.Parallel()

	var buf bytes.Buffer
	err := CommentableMarkdown(CommentableMarkdownArgs{
		Surface:  CommentSurfaceArtifact,
		IDPrefix: SafeCommentTargetSlug("artifact", "workspace", "plan.md"),
		DocPath:  "plan.md",
		Sections: []CommentSectionView{{
			ID:       "section-0",
			Title:    "Section 1",
			BodyHTML: `<div class="markdown-code-block">frontmatter</div>`,
		}},
		Comments: []CommentThreadView{{
			ID:          "frontmatter-comment",
			AuthorEmail: "a@example.com",
			Body:        "existing preamble thread",
			SectionID:   "section-0",
		}},
		Routes: CommentRoutes{Show: "/show", Create: "/create", Cancel: "/cancel"},
	}).Render(t.Context(), &buf)
	if err != nil {
		t.Fatalf("Render() error = %v", err)
	}
	html := buf.String()
	for _, want := range []string{`data-comment-target="true"`, `existing preamble thread`} {
		if !strings.Contains(html, want) {
			t.Fatalf("render missing %q: %s", want, html)
		}
	}
}

func TestCommentableMarkdownMovesThoughtsThreadReadingOutOfDocumentTargets(
	t *testing.T,
) {
	t.Parallel()

	var buf bytes.Buffer
	err := CommentableMarkdown(CommentableMarkdownArgs{
		Surface:  CommentSurfaceThoughts,
		IDPrefix: SafeCommentTargetSlug("thoughts", "thoughts/plan.md"),
		DocPath:  "thoughts/plan.md",
		Sections: []CommentSectionView{{
			ID:          "section-1",
			Title:       "Section 1",
			HeadingHTML: `<h2>Section 1</h2>`,
			BodyHTML:    `<p>Body</p>`,
		}},
		Comments: []CommentThreadView{{
			ID:          "document-comment",
			AuthorEmail: "a@example.com",
			Body:        "existing document thread",
			SectionID:   "document",
		}},
		Routes: CommentRoutes{Show: "/show", Create: "/create", Cancel: "/cancel"},
	}).Render(t.Context(), &buf)
	if err != nil {
		t.Fatalf("Render() error = %v", err)
	}
	html := buf.String()
	for _, want := range []string{`data-comment-target="true"`, `aria-label="1 comments"`, `section_hint" value="document"`} {
		if !strings.Contains(html, want) {
			t.Fatalf("render missing %q: %s", want, html)
		}
	}
	for _, forbidden := range []string{`existing document thread`, `commentui-popover-target`} {
		if strings.Contains(html, forbidden) {
			t.Fatalf(
				"thoughts document target rendered forbidden thread-reading content %q: %s",
				forbidden,
				html,
			)
		}
	}
}

func TestCommentSharedPatchTargetsRenderStableIDs(t *testing.T) {
	t.Parallel()

	args := BuildCommentsPanelArgs(CommentableMarkdownArgs{
		Surface:  CommentSurfaceThoughts,
		IDPrefix: SafeCommentTargetSlug("thoughts", "thoughts/plan.md"),
		DocPath:  "thoughts/plan.md",
		Routes:   CommentRoutes{Reply: func(string) string { return "/reply" }},
	}, "")

	var panel bytes.Buffer
	if err := CommentsContextPanel(args).Render(t.Context(), &panel); err != nil {
		t.Fatalf("CommentsContextPanel Render() error = %v", err)
	}
	panelHTML := panel.String()
	for _, want := range []string{
		`id="` + CommentsContextPanelID + `"`,
		`id="` + CommentsContextThreadListID + `"`,
	} {
		if !strings.Contains(panelHTML, want) {
			t.Fatalf("context panel missing %q: %s", want, panelHTML)
		}
	}

	var mobile bytes.Buffer
	if err := MobileSectionCommentTarget().Render(t.Context(), &mobile); err != nil {
		t.Fatalf("MobileSectionCommentTarget Render() error = %v", err)
	}
	mobileHTML := mobile.String()
	if !strings.Contains(mobileHTML, `id="`+MobileSectionCommentContentID+`"`) {
		t.Fatalf("mobile target missing shared ID: %s", mobileHTML)
	}
}

func TestCommentsContextPanelShowsAllSectionsWithSectionTitles(t *testing.T) {
	t.Parallel()

	args := BuildCommentsPanelArgs(CommentableMarkdownArgs{
		Surface:  CommentSurfaceThoughts,
		IDPrefix: SafeCommentTargetSlug("thoughts", "thoughts/plan.md"),
		DocPath:  "thoughts/plan.md",
		Comments: []CommentThreadView{
			{
				ID:          "comment-1",
				AuthorEmail: "a@example.com",
				Body:        "first",
				SectionID:   "section-13",
				HeadingHint: "Launch Readiness",
			},
			{
				ID:          "comment-2",
				AuthorEmail: "b@example.com",
				Body:        "second",
				SectionID:   "section-14",
				HeadingHint: "Follow-up",
			},
		},
	}, "section-13")

	var buf bytes.Buffer
	if err := CommentsContextPanel(args).Render(t.Context(), &buf); err != nil {
		t.Fatalf("Render() error = %v", err)
	}
	html := buf.String()
	for _, want := range []string{`first`, `second`, `Launch Readiness`, `Follow-up`} {
		if !strings.Contains(html, want) {
			t.Fatalf("context panel missing %q: %s", want, html)
		}
	}
	if strings.Contains(html, `>section-13<`) || strings.Contains(html, `>section-14<`) {
		t.Fatalf("context panel rendered section ids instead of titles: %s", html)
	}
}

func TestCommentsContextPanelRendersFullSelectedQuote(t *testing.T) {
	t.Parallel()

	longQuote := "selected quote line one\nselected quote line two that should stay visible"
	args := BuildCommentsPanelArgs(CommentableMarkdownArgs{
		Surface:   CommentSurfaceThoughts,
		IDPrefix:  SafeCommentTargetSlug("thoughts", "thoughts/plan.md"),
		DocPath:   "thoughts/plan.md",
		UserEmail: "user@example.com",
		Routes: CommentRoutes{
			Reply:   func(string) string { return "/reply" },
			Resolve: func(string) string { return "/resolve" },
		},
		HiddenFields: map[string]string{"doc_path": "thoughts/plan.md"},
		Comments: []CommentThreadView{{
			ID:           "comment-1",
			AuthorEmail:  "a@example.com",
			Body:         "body",
			SelectedText: longQuote,
			SectionID:    "section-1",
		}},
	}, "")

	var buf bytes.Buffer
	if err := CommentsContextPanel(args).Render(t.Context(), &buf); err != nil {
		t.Fatalf("Render() error = %v", err)
	}
	html := buf.String()
	for _, want := range []string{`id="comments-context-panel"`, `id="comments-context-thread-list"`, longQuote, `commentui-thread-quote`} {
		if !strings.Contains(html, want) {
			t.Fatalf("context panel missing %q: %s", want, html)
		}
	}
	if strings.Contains(html, `max-h-20`) {
		t.Fatalf("context panel quote is clamped: %s", html)
	}
}

func TestCommentTargetsRenderCompactAvatarsAndPopoverThreads(t *testing.T) {
	t.Parallel()

	var buf bytes.Buffer
	args := CommentableMarkdownArgs{
		Surface:  CommentSurfaceArtifact,
		IDPrefix: SafeCommentTargetSlug("artifact", "workspace", "plan.md"),
		DocPath:  "plan.md",
		Sections: []CommentSectionView{
			{
				ID:          "section-1",
				Title:       "Section 1",
				HeadingHTML: "<h2>Section 1</h2>",
				BodyHTML:    "<p>Body</p>",
			},
		},
		Comments: []CommentThreadView{
			{
				ID:          "open",
				AuthorEmail: "alice@example.com",
				Body:        "open body",
				SectionID:   "section-1",
				Replies: []CommentReplyView{
					{AuthorEmail: "bob@example.com", Body: "reply body"},
				},
			},
			{
				ID:          "done",
				AuthorEmail: "done@example.com",
				Body:        "resolved body must stay compact",
				SectionID:   "resolved-section",
				Resolved:    true,
			},
		},
		Routes: CommentRoutes{
			Show:    "/show",
			Create:  "/create",
			Cancel:  "/cancel",
			Reply:   func(string) string { return "/reply" },
			Resolve: func(string) string { return "/resolve" },
			Reopen:  func(string) string { return "/reopen" },
		},
	}
	if err := CommentableMarkdown(args).Render(t.Context(), &buf); err != nil {
		t.Fatalf("Render() error = %v", err)
	}
	html := buf.String()
	for _, want := range []string{
		`h-6 w-6`,
		`data-attr:aria-expanded`,
		`comment-expanded-`,
		`open body`,
		`reply body`,
		`data-comment-target="true"`,
		`contentType: &#39;form&#39;`,
	} {
		if !strings.Contains(html, want) {
			t.Fatalf("render missing %q in %s", want, html)
		}
	}
	if strings.Contains(html, `resolved body must stay compact`) {
		t.Fatalf("resolved chip rendered full resolved body: %s", html)
	}
}

func TestCommentPopoverAndSelectionPlacementClasses(t *testing.T) {
	t.Parallel()

	var buf bytes.Buffer
	args := CommentableMarkdownArgs{
		Surface:  CommentSurfaceArtifact,
		IDPrefix: SafeCommentTargetSlug("artifact", "workspace", "plan.md"),
		DocPath:  "plan.md",
		Sections: []CommentSectionView{{
			ID:          "section-1",
			Title:       "Section 1",
			HeadingHTML: "<h2>Section 1</h2>",
			BodyHTML:    "<p>Body</p>",
		}},
		Comments: []CommentThreadView{{
			ID:          "comment-1",
			AuthorEmail: "author@example.com",
			Body:        "body",
			SectionID:   "section-1",
		}},
		Routes: CommentRoutes{
			Show:   "/show",
			Create: "/create",
			Cancel: "/cancel",
			Expand: "/expand",
		},
		SelectionSignals: SelectionSignalArgs{
			Prefix:       "comment_selection",
			ShowRoute:    "/show",
			HiddenFields: map[string]string{"doc_path": "plan.md"},
			ContainerID:  "scroll-region",
		},
	}
	if err := CommentableMarkdown(args).Render(t.Context(), &buf); err != nil {
		t.Fatalf("Render() error = %v", err)
	}
	html := buf.String()
	for _, want := range []string{
		`data-commentui-container="true"`,
		`commentui-anchor relative`,
		`commentui-popover-target`,
		`commentui-selection-trigger`,
		`--commentui-selection-top`,
		`--commentui-selection-left`,
		`name="selected_text"`,
		`name="section_hint"`,
		`name="heading_hint"`,
		`element.closest(&#39;[data-commentui-container=true]&#39;)`,
	} {
		if !strings.Contains(html, want) {
			t.Fatalf("render missing %q in %s", want, html)
		}
	}
	for _, forbidden := range []string{
		`fixed right-4 md:right-[400px]`,
		`left-full top-0 ml-12`,
		` style="`,
	} {
		if strings.Contains(html, forbidden) {
			t.Fatalf("render contains forbidden %q in %s", forbidden, html)
		}
	}
}

func TestCommentToggleWithFormUsesPopoverPlacementClass(t *testing.T) {
	t.Parallel()

	var buf bytes.Buffer
	target := CommentTargetView{
		ID:        "comment-target-doc",
		SignalKey: "doc",
		Routes: CommentRoutes{
			Create: "/create",
			Cancel: "/cancel",
		},
		HiddenFields: map[string]string{"doc_path": "plan.md"},
	}
	view := CommentFormView{ID: "comment-form-doc", Target: target, SelectedText: "quote"}
	if err := CommentTargetWithForm(target, view).Render(t.Context(), &buf); err != nil {
		t.Fatalf("Render() error = %v", err)
	}
	html := buf.String()
	for _, want := range []string{`commentui-anchor relative commentui-target`, `commentui-popover-target`, `ring-primary/30`, `data-on:click__window`, `@post(&#39;/cancel?doc_path=plan.md&#39;)`, `name="selected_text" value="quote"`} {
		if !strings.Contains(html, want) {
			t.Fatalf("render missing %q in %s", want, html)
		}
	}
	for _, forbidden := range []string{`left-full top-0 ml-12`, ` style="`, `New comment`, `>New<`, `border-primary/50`} {
		if strings.Contains(html, forbidden) {
			t.Fatalf("render contains forbidden %q in %s", forbidden, html)
		}
	}
}

func TestBuildThreadViewsCopiesThreadsAndReplies(t *testing.T) {
	t.Parallel()

	createdAt := time.Date(2026, 5, 15, 12, 0, 0, 0, time.UTC)
	hiddenFields := map[string]string{"doc_path": "plan.md"}
	views := BuildThreadViews([]ThreadSource{{
		ID:           "comment-1",
		AuthorEmail:  "alice@example.com",
		ActorLabel:   "user",
		CreatedAt:    createdAt,
		Body:         "body",
		SelectedText: "quote",
		SectionID:    "",
		HeadingHint:  "Intro",
		Resolved:     true,
		HiddenFields: hiddenFields,
		Replies: []ReplySource{{
			AuthorEmail: "bot@example.com",
			ActorLabel:  "assistant",
			CreatedAt:   createdAt.Add(time.Minute),
			Body:        "reply",
		}},
	}})

	if len(views) != 1 || len(views[0].Replies) != 1 {
		t.Fatalf("BuildThreadViews() = %#v", views)
	}
	view := views[0]
	if view.SectionID != "document" || view.HeadingHint != "Intro" || !view.Resolved ||
		view.SelectedText != "quote" {
		t.Fatalf("thread fields not preserved: %#v", view)
	}
	if view.Replies[0].ActorLabel != "assistant" || view.Replies[0].Body != "reply" {
		t.Fatalf("reply fields not preserved: %#v", view.Replies[0])
	}
	views[0].HiddenFields["doc_path"] = "changed.md"
	if hiddenFields["doc_path"] != "plan.md" {
		t.Fatalf("BuildThreadViews did not copy hidden fields: %#v", hiddenFields)
	}
}

func TestBuildTargetViewMergesHiddenFieldsAndStableIDs(t *testing.T) {
	t.Parallel()

	target := BuildTargetView(TargetInput{
		Surface:      CommentSurfaceThoughts,
		IDPrefix:     SafeCommentTargetSlug("thoughts", "plan.md"),
		DocPath:      "plan.md",
		SectionID:    "",
		HeadingHint:  "Heading",
		UserEmail:    "user@example.com",
		Threads:      []CommentThreadView{{ID: "comment-1"}},
		Routes:       CommentRoutes{Create: "/create"},
		HiddenFields: map[string]string{"doc_path": "plan.md"},
	})

	if target.Surface != CommentSurfaceThoughts || target.SectionID != "document" ||
		target.HeadingHint != "Heading" {
		t.Fatalf("target fields not preserved: %#v", target)
	}
	if target.ID == "" || target.SignalKey == "" ||
		strings.Contains(target.SignalKey, "-") {
		t.Fatalf("target IDs not stable/safe: %#v", target)
	}
	for key, want := range map[string]string{
		"doc_path":     "plan.md",
		"section_hint": "document",
		"heading_hint": "Heading",
	} {
		if got := target.HiddenFields[key]; got != want {
			t.Fatalf(
				"HiddenFields[%q] = %q, want %q in %#v",
				key,
				got,
				want,
				target.HiddenFields,
			)
		}
	}
}

func TestMergeCommentHiddenFieldsOverlaysPairs(t *testing.T) {
	t.Parallel()

	base := map[string]string{"doc_path": "plan.md", "section_hint": "old"}
	merged := MergeCommentHiddenFields(base,
		map[string]string{"section_hint": "document"},
		map[string]string{"heading_hint": "Intro"},
	)
	base["doc_path"] = "changed.md"

	for key, want := range map[string]string{
		"doc_path":     "plan.md",
		"section_hint": "document",
		"heading_hint": "Intro",
	} {
		if got := merged[key]; got != want {
			t.Fatalf("merged[%q] = %q, want %q in %#v", key, got, want, merged)
		}
	}
}
