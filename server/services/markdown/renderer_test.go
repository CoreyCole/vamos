package markdown

import (
	"errors"
	"io"
	"path/filepath"
	"strings"
	"testing"

	"github.com/gomarkdown/markdown/ast"

	"github.com/CoreyCole/vamos/server"
)

type failingWriter struct{ err error }

func (w failingWriter) Write([]byte) (int, error) { return 0, w.err }

type shortWriter struct{}

func (shortWriter) Write(p []byte) (int, error) { return len(p) - 1, nil }

func TestMarkdownBytesToHTML_LinksInlineThoughtsPath(t *testing.T) {
	r, err := NewRenderer("github-dark")
	if err != nil {
		t.Fatalf("NewRenderer() error = %v", err)
	}

	md := []byte(
		"That matches (`thoughts/CoreyCole/plans/2026-04-14_15-28-42_eq-automated-holds/prds/2026-04-14_15-28-42_holds-v2-prd.md`).",
	)
	html, err := r.MarkdownBytesToHTML(md)
	if err != nil {
		t.Fatalf("MarkdownBytesToHTML() error = %v", err)
	}

	want := `<a class="` + thoughtsLinkClass + `" href="/thoughts/CoreyCole/plans/2026-04-14_15-28-42_eq-automated-holds/prds/2026-04-14_15-28-42_holds-v2-prd.md"><code>thoughts/CoreyCole/plans/2026-04-14_15-28-42_eq-automated-holds/prds/2026-04-14_15-28-42_holds-v2-prd.md</code></a>`
	if !strings.Contains(html, want) {
		t.Fatalf("expected inline thoughts path to be linked; html = %s", html)
	}
}

func TestMarkdownBytesToHTML_RendersFrontmatterAsYAMLCodeBlock(t *testing.T) {
	r, err := NewRenderer("github-dark")
	if err != nil {
		t.Fatalf("NewRenderer() error = %v", err)
	}

	md := []byte("---\ndate: 2026-04-19\ntopic: Renderer Test\n---\n\n# Heading")
	html, err := r.MarkdownBytesToHTML(md)
	if err != nil {
		t.Fatalf("MarkdownBytesToHTML() error = %v", err)
	}

	if strings.Contains(html, "<hr") {
		t.Fatalf(
			"expected frontmatter to render as a code block, not horizontal rules; html = %s",
			html,
		)
	}
	if !strings.Contains(html, "markdown-code-block") {
		t.Fatalf("expected frontmatter code block wrapper; html = %s", html)
	}
	if !strings.Contains(html, "2026-04-19") {
		t.Fatalf("expected frontmatter YAML content in rendered html; html = %s", html)
	}
	if !strings.Contains(html, "Heading") {
		t.Fatalf(
			"expected body markdown to still render after frontmatter; html = %s",
			html,
		)
	}
}

func TestRenderStatePreservesFirstWriteErrorAndTerminates(t *testing.T) {
	sentinel := errors.New("write failed")
	state := &renderState{}
	if got := state.write(
		failingWriter{err: sentinel},
		[]byte("x"),
	); got != ast.Terminate {
		t.Fatalf("write status = %v, want Terminate", got)
	}
	if !errors.Is(state.Err(), sentinel) {
		t.Fatalf("state error = %v, want sentinel", state.Err())
	}
	state.write(failingWriter{err: errors.New("later")}, []byte("x"))
	if !errors.Is(state.Err(), sentinel) {
		t.Fatalf("state error replaced: %v", state.Err())
	}
}

func TestRenderStateMapsShortWriteAndHookTerminates(t *testing.T) {
	state := &renderState{}
	if got := state.write(shortWriter{}, []byte("x")); got != ast.Terminate {
		t.Fatalf("short write status = %v, want Terminate", got)
	}
	if !errors.Is(state.Err(), io.ErrShortWrite) {
		t.Fatalf("state error = %v, want io.ErrShortWrite", state.Err())
	}

	r, err := NewRenderer(DefaultCodeStyle)
	if err != nil {
		t.Fatalf("NewRenderer() error = %v", err)
	}
	state = &renderState{}
	renderer := mdhtmlRenderer(r.highlightStyle, r.htmlFormatter, state)
	status := renderer.RenderNode(
		failingWriter{err: errors.New("hook failed")},
		&ast.CodeBlock{Leaf: ast.Leaf{Literal: []byte("x")}},
		true,
	)
	if status != ast.Terminate {
		t.Fatalf("RenderNode status = %v, want Terminate", status)
	}
	if state.Err() == nil {
		t.Fatal("hook failure was not retained")
	}
}

func TestStateWriterRecordsFormatterWriteFailures(t *testing.T) {
	sentinel := errors.New("formatter write failed")
	state := &renderState{}
	writer := stateWriter{state: state, writer: failingWriter{err: sentinel}}
	if _, err := writer.Write([]byte("x")); !errors.Is(err, sentinel) {
		t.Fatalf("Write() error = %v, want sentinel", err)
	}
	if !errors.Is(state.Err(), sentinel) {
		t.Fatalf("state error = %v, want sentinel", state.Err())
	}

	state = &renderState{}
	writer = stateWriter{state: state, writer: shortWriter{}}
	if _, err := writer.Write([]byte("x")); !errors.Is(err, io.ErrShortWrite) {
		t.Fatalf("Write() error = %v, want io.ErrShortWrite", err)
	}
	if !errors.Is(state.Err(), io.ErrShortWrite) {
		t.Fatalf("state error = %v, want io.ErrShortWrite", state.Err())
	}
}

func TestResolveGitHubRepoFromConfiguredProjects(t *testing.T) {
	projects := server.ProjectsConfig{Repos: map[string]server.RepoConfig{
		"monorepo": {
			GitHubURL:     "https://github.com/premiumlabs/monorepo",
			DefaultBranch: "develop",
		},
	}}
	gh := ResolveGitHubRepoFromProjects(projects, "monorepo")
	if gh == nil {
		t.Fatal("ResolveGitHubRepoFromProjects() = nil")
	}
	if gh.URL != "https://github.com/premiumlabs/monorepo" || gh.Branch != "develop" {
		t.Fatalf("GitHub repo = %+v", gh)
	}
}

func TestGitHubURLForPathUsesConfiguredCheckout(t *testing.T) {
	root := filepath.Join("home", "ruby", "cn", "chestnut-flake", "monorepo-main")
	projects := server.ProjectsConfig{Repos: map[string]server.RepoConfig{
		"monorepo": {
			GitHubURL:     "https://github.com/premiumlabs/monorepo",
			DefaultBranch: "develop",
			Checkouts: map[string]server.CheckoutConfig{
				"monorepo-main": {RootPath: "/" + root},
			},
		},
	}}
	url, ok := GitHubURLForPath(projects, "/"+root+"/pkg/example/file.go")
	if !ok {
		t.Fatal("GitHubURLForPath() ok = false")
	}
	want := "https://github.com/premiumlabs/monorepo/blob/develop/pkg/example/file.go"
	if url != want {
		t.Fatalf("GitHubURLForPath() = %q, want %q", url, want)
	}
}

func TestNormalizeThoughtsPath(t *testing.T) {
	path, ok := normalizeThoughtsPath("thoughts/foo/bar.md")
	if !ok {
		t.Fatal("expected thoughts path to match")
	}
	if path != "/thoughts/foo/bar.md" {
		t.Fatalf("path = %q, want %q", path, "/thoughts/foo/bar.md")
	}

	if _, ok := normalizeThoughtsPath("pkg/ledger/v2/commissions/post.go"); ok {
		t.Fatal("expected non-thoughts code path not to match")
	}
}
