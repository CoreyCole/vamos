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
		`data-on:submit__prevent="el.closest(&#39;details&#39;)?.removeAttribute(&#39;open&#39;); @post(&#39;/forms/comments/show&#39;, {contentType: &#39;form&#39;})"`,
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
}
