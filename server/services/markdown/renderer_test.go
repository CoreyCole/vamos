package markdown

import (
	"errors"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/gomarkdown/markdown/ast"

	"github.com/CoreyCole/vamos/server"
)

func TestResolveThoughtsDocumentRequestClassifiesSafeClientErrors(t *testing.T) {
	t.Parallel()

	root := t.TempDir()
	service := &Service{basePath: root}

	if _, err := service.resolveThoughtsDocumentRequest(
		"missing.md",
	); !errors.Is(
		err,
		errThoughtsDocumentNotFound,
	) {
		t.Fatalf("missing document error = %v", err)
	}
	if err := os.Mkdir(filepath.Join(root, "directory"), 0o755); err != nil {
		t.Fatal(err)
	}
	if _, err := service.resolveThoughtsDocumentRequest(
		"directory",
	); !errors.Is(
		err,
		errThoughtsDocumentIsDirectory,
	) {
		t.Fatalf("directory error = %v", err)
	}
	if _, err := service.resolveThoughtsDocumentRequest(
		"../outside.md",
	); !errors.Is(
		err,
		errInvalidThoughtsDocumentPath,
	) {
		t.Fatalf("traversal error = %v", err)
	}
	outside := t.TempDir()
	if err := os.WriteFile(
		filepath.Join(outside, "secret.md"),
		[]byte("secret"),
		0o600,
	); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(outside, filepath.Join(root, "escape")); err != nil {
		t.Fatal(err)
	}
	if _, err := service.resolveThoughtsDocumentRequest(
		"escape/secret.md",
	); !errors.Is(
		err,
		errInvalidThoughtsDocumentPath,
	) {
		t.Fatalf("symlink escape error = %v", err)
	}
}

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

func TestMarkdownBytesToHTML_FencedCodeSyntaxHighlight(t *testing.T) {
	r, err := NewRenderer("github-dark")
	if err != nil {
		t.Fatalf("NewRenderer() error = %v", err)
	}

	md := []byte(
		"```go\npackage main\n\nimport \"fmt\"\n\nfunc main() {\n\tfmt.Println(\"hi\")\n}\n```\n",
	)
	html, err := r.MarkdownBytesToHTML(md)
	if err != nil {
		t.Fatalf("MarkdownBytesToHTML() error = %v", err)
	}
	if !strings.Contains(html, `class="chroma"`) && !strings.Contains(html, "chroma") {
		t.Fatalf(
			"expected chroma syntax-highlight classes in fenced go block; html = %s",
			html,
		)
	}
	// Token classes vary by style; require at least one highlighted token span.
	if !strings.Contains(html, "<span") {
		t.Fatalf("expected chroma token spans inside fenced code; html = %s", html)
	}
	if !strings.Contains(html, "fmt") || !strings.Contains(html, "Println") {
		t.Fatalf("expected go source text preserved in highlighted html; html = %s", html)
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

func TestMarkdownBytesToHTML_KeepsHeadersWhenDelimiterHasExtraColumns(t *testing.T) {
	t.Parallel()

	r, err := NewRenderer(DefaultCodeStyle)
	if err != nil {
		t.Fatalf("NewRenderer() error = %v", err)
	}
	md := []byte(strings.Join([]string{
		"## Review follow-ups",
		"",
		"| Age | Owner | Follow-up | Source review |",
		"| --- | --- | --- | --- | --- |",
		"| 57 days | Nick Hilem | [fix(onboarding)](https://linear.app/chestnut/issue/PRO-9835) | [AI-113](https://linear.app/chestnut/issue/AI-113) |",
	}, "\n"))
	html, err := r.MarkdownBytesToHTML(md)
	if err != nil {
		t.Fatalf("MarkdownBytesToHTML() error = %v", err)
	}
	if strings.Contains(html, "| Age | Owner |") {
		t.Fatalf("header leaked as raw markdown: %s", html)
	}
	for _, want := range []string{
		"<th",
		"Age",
		"Owner",
		"Follow-up",
		"Source review",
		"Nick Hilem",
		"AI-113",
	} {
		if !strings.Contains(html, want) {
			t.Fatalf("missing %q: %s", want, html)
		}
	}
}

func TestMarkdownBytesToHTML_KeepsExtraHeaderCellsWhenDelimiterIsShort(t *testing.T) {
	t.Parallel()

	r, err := NewRenderer(DefaultCodeStyle)
	if err != nil {
		t.Fatalf("NewRenderer() error = %v", err)
	}
	md := []byte("| A | B | C |\n| --- | --- |\n| 1 | 2 | 3 |\n")
	html, err := r.MarkdownBytesToHTML(md)
	if err != nil {
		t.Fatalf("MarkdownBytesToHTML() error = %v", err)
	}
	if strings.Contains(html, "| A | B | C |") {
		t.Fatalf("header leaked as raw markdown: %s", html)
	}
	for _, want := range []string{"<th", ">A</th>", ">B</th>", ">C</th>", ">1</td>", ">2</td>", ">3</td>"} {
		if !strings.Contains(html, want) {
			t.Fatalf("missing %q: %s", want, html)
		}
	}
}
