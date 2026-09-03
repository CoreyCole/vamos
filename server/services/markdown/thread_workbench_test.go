package markdown

import (
	"context"
	"database/sql"
	"errors"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/a-h/templ"
	"github.com/labstack/echo/v4"
)

type threadWorkbenchTestRenderer struct {
	artifact      string
	listThreadID  string
	chatThreadID  string
	threadPlanDir string
	threadPlanErr error
}

func (r *threadWorkbenchTestRenderer) RenderWorkbenchThreadList(
	_ context.Context,
	threadID, artifact string,
) (templ.Component, error) {
	r.listThreadID = threadID
	r.artifact = artifact
	return templ.Raw(
		`<a href="/threads/thread-1?artifact=thoughts%2Fowner%2Fplans%2Falpha%2Fdesign.md">Thread</a>`,
	), nil
}

func (r *threadWorkbenchTestRenderer) ResolveSharedThreadPlanDir(
	context.Context,
	string,
) (string, error) {
	if r.threadPlanErr != nil {
		return "", r.threadPlanErr
	}
	if r.threadPlanDir != "" {
		return r.threadPlanDir, nil
	}
	return "owner/plans/alpha", nil
}

func (r *threadWorkbenchTestRenderer) FindSharedThreadForDoc(
	context.Context,
	string,
) (string, error) {
	return "", nil
}

func (r *threadWorkbenchTestRenderer) RenderSharedThreadChat(
	_ context.Context,
	threadID,
	_ string,
) (templ.Component, error) {
	r.chatThreadID = threadID
	return templ.Raw(`<div id="thread-chat">original thread chat</div>`), nil
}

func TestServeMarkdownLegacyChatWithoutThreadRedirectsToThreadIndex(t *testing.T) {
	root := t.TempDir()
	mustMkdirAll(t, filepath.Join(root, "owner", "plans", "alpha"))
	mustWriteFile(
		t,
		filepath.Join(root, "owner", "plans", "alpha", "design.md"),
		[]byte("# Design"),
	)
	svc, err := NewService(root, nil, nil)
	if err != nil {
		t.Fatal(err)
	}
	svc.WithWorkbenchThreadRenderer(&threadWorkbenchTestRenderer{})
	e := echo.New()
	rec := httptest.NewRecorder()
	c := e.NewContext(
		httptest.NewRequest(
			"GET",
			"/thoughts/owner/plans/alpha/design.md?context=chat",
			nil,
		),
		rec,
	)
	c.SetParamNames("*")
	c.SetParamValues("owner/plans/alpha/design.md")
	if err := svc.ServeMarkdown(c); err != nil {
		t.Fatal(err)
	}
	if got := rec.Header().
		Get("Location"); got != "/threads?artifact=thoughts%2Fowner%2Fplans%2Falpha%2Fdesign.md" {
		t.Fatalf("Location = %q", got)
	}
}

func TestServeThreadsHydratesArtifactAndCarriesIt(t *testing.T) {
	root := t.TempDir()
	mustMkdirAll(t, filepath.Join(root, "owner", "plans", "alpha"))
	mustWriteFile(
		t,
		filepath.Join(root, "owner", "plans", "alpha", "design.md"),
		[]byte("# Distinctive artifact"),
	)
	svc, err := NewService(root, nil, nil)
	if err != nil {
		t.Fatal(err)
	}
	r := &threadWorkbenchTestRenderer{}
	svc.WithWorkbenchThreadRenderer(r)
	e := echo.New()
	rec := httptest.NewRecorder()
	c := e.NewContext(
		httptest.NewRequest(
			"GET",
			"/threads?artifact=thoughts/owner/plans/alpha/design.md",
			nil,
		),
		rec,
	)
	if err := svc.ServeThreads(c); err != nil {
		t.Fatal(err)
	}
	if r.artifact != "owner/plans/alpha/design.md" {
		t.Fatalf("artifact = %q", r.artifact)
	}
	for _, want := range []string{"Distinctive artifact", "/threads/thread-1?artifact=thoughts%2Fowner%2Fplans%2Falpha%2Fdesign.md"} {
		if !strings.Contains(rec.Body.String(), want) {
			t.Fatalf("missing %q", want)
		}
	}
}

func TestServeThreadDisplaysExplicitCrossPlanArtifactsWithoutChangingThread(
	t *testing.T,
) {
	root := t.TempDir()
	mustMkdirAll(t, filepath.Join(root, "owner", "plans", "alpha"))
	mustMkdirAll(t, filepath.Join(root, "other", "plans", "beta", "docs"))
	mustWriteFile(
		t,
		filepath.Join(root, "owner", "plans", "alpha", "design.md"),
		[]byte("# Alpha"),
	)
	mustWriteFile(
		t,
		filepath.Join(root, "other", "plans", "beta", "review.md"),
		[]byte("# Beta review"),
	)
	mustWriteFile(
		t,
		filepath.Join(root, "other", "plans", "beta", "docs", "note.md"),
		[]byte("# Beta note"),
	)

	for _, tc := range []struct {
		name, doc, want string
	}{
		{name: "file", doc: "thoughts/other/plans/beta/review.md", want: "Beta review"},
		{name: "directory", doc: "thoughts/other/plans/beta/docs", want: "note"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			svc, err := NewService(root, nil, nil)
			if err != nil {
				t.Fatal(err)
			}
			renderer := &threadWorkbenchTestRenderer{}
			svc.WithWorkbenchThreadRenderer(renderer)
			rec := httptest.NewRecorder()
			c := echo.New().NewContext(
				httptest.NewRequest("GET", "/threads/thread-alpha?artifact="+tc.doc, nil),
				rec,
			)
			c.SetParamNames("threadID")
			c.SetParamValues("thread-alpha")
			if err := svc.ServeThread(c); err != nil {
				t.Fatal(err)
			}
			if renderer.listThreadID != "thread-alpha" ||
				renderer.chatThreadID != "thread-alpha" {
				t.Fatalf(
					"thread identity changed: list=%q chat=%q",
					renderer.listThreadID,
					renderer.chatThreadID,
				)
			}
			if !strings.Contains(rec.Body.String(), tc.want) {
				t.Fatalf(
					"missing cross-plan artifact %q in %s",
					tc.want,
					rec.Body.String(),
				)
			}
			html := rec.Body.String()
			if strings.Contains(html, "Alpha") {
				t.Fatalf(
					"default plan artifact replaced cross-plan artifact: %s",
					html,
				)
			}
			if tc.name == "file" {
				for _, want := range []string{
					`comment-target-`,
					`name="workbench_v2" value="1"`,
					`name="doc_path" value="thoughts/other/plans/beta/review.md"`,
				} {
					if !strings.Contains(html, want) {
						t.Fatalf("missing wrapped comment UI %q in %s", want, html)
					}
				}
				for _, unwanted := range []string{"rightRailActiveTab", "docWorkbenchRight", "doc-right-comments-panel"} {
					if strings.Contains(html, unwanted) {
						t.Fatalf("render retained legacy chrome %q in %s", unwanted, html)
					}
				}
			}
		})
	}
}

func TestServeThreadFileRowsAreRealThreadGetsWithoutIntercept(t *testing.T) {
	t.Parallel()

	root := t.TempDir()
	mustMkdirAll(t, filepath.Join(root, "owner", "plans", "alpha"))
	mustMkdirAll(t, filepath.Join(root, "other", "plans", "beta", "docs"))
	mustWriteFile(
		t,
		filepath.Join(root, "owner", "plans", "alpha", "design.md"),
		[]byte("# Alpha"),
	)
	mustWriteFile(
		t,
		filepath.Join(root, "other", "plans", "beta", "design.md"),
		[]byte("# Beta design"),
	)
	mustWriteFile(
		t,
		filepath.Join(root, "other", "plans", "beta", "docs", "review.md"),
		[]byte("# Beta review\n\n[Fullscreen](/thoughts/owner/plans/alpha/design.md)"),
	)
	svc, err := NewService(root, nil, nil)
	if err != nil {
		t.Fatal(err)
	}
	svc.WithWorkbenchThreadRenderer(&threadWorkbenchTestRenderer{})
	rec := httptest.NewRecorder()
	c := echo.New().NewContext(
		httptest.NewRequest(
			http.MethodGet,
			"/threads/thread-alpha?artifact=thoughts/other/plans/beta/docs/review.md&artifact_dir=thoughts/other/plans/beta",
			http.NoBody,
		),
		rec,
	)
	c.SetParamNames("threadID")
	c.SetParamValues("thread-alpha")
	c.Set("user_email", "owner@example.com")
	if err := svc.ServeThread(c); err != nil {
		t.Fatal(err)
	}
	body := rec.Body.String()
	fileHref := `/threads/thread-alpha?artifact=thoughts%2Fother%2Fplans%2Fbeta%2Fdesign.md&amp;artifact_dir=thoughts%2Fother%2Fplans%2Fbeta`
	for _, want := range []string{
		"original thread chat",
		"Beta review",
		`href="/thoughts/owner/plans/alpha/design.md"`,
		`data-thread-artifact-cwd="thoughts/other/plans/beta"`,
		`data-thread-artifact-file`,
		fileHref,
		`id="thread-chat"`,
	} {
		if !strings.Contains(body, want) {
			t.Fatalf("ServeThread missing %q: %s", want, body)
		}
	}
	if strings.Contains(body, "threadArtifactFileClickAction") ||
		strings.Contains(body, "/threads/thread-alpha/artifact?") ||
		strings.Contains(body, "window.history.pushState") ||
		strings.Contains(body, "selector #workbench-v2-artifact-body") ||
		strings.Contains(body, "selector #workbench-root") ||
		strings.Contains(
			body,
			`href="/thoughts/owner/plans/alpha/design.md" data-on:click`,
		) {
		t.Fatalf("ServeThread used file-select patch or intercepted Thoughts: %s", body)
	}
}

func TestHandleThreadArtifactBrowserPatchesOnlyBrowserAndHistory(t *testing.T) {
	t.Parallel()

	root := t.TempDir()
	mustMkdirAll(t, filepath.Join(root, "owner", "plans", "alpha", "docs"))
	mustWriteFile(
		t,
		filepath.Join(root, "owner", "plans", "alpha", "design.md"),
		[]byte("# Alpha"),
	)
	svc, err := NewService(root, nil, nil)
	if err != nil {
		t.Fatal(err)
	}
	svc.WithWorkbenchThreadRenderer(&threadWorkbenchTestRenderer{})
	rec := httptest.NewRecorder()
	c := echo.New().NewContext(
		httptest.NewRequest(
			http.MethodGet,
			"/threads/thread-alpha/artifact-browser?artifact=thoughts/owner/plans/alpha/design.md&artifact_dir=thoughts/owner/plans/alpha/docs",
			http.NoBody,
		),
		rec,
	)
	c.SetParamNames("threadID")
	c.SetParamValues("thread-alpha")
	if err := svc.HandleThreadArtifactBrowser(c); err != nil {
		t.Fatal(err)
	}
	body := rec.Body.String()
	for _, want := range []string{
		"selector #thread-artifact-browser",
		`data-thread-artifact-cwd="thoughts/owner/plans/alpha/docs"`,
		"window.history.pushState",
		`/threads/thread-alpha?artifact=thoughts%2Fowner%2Fplans%2Falpha%2Fdesign.md\u0026artifact_dir=thoughts%2Fowner%2Fplans%2Falpha%2Fdocs`,
	} {
		if !strings.Contains(body, want) {
			t.Fatalf("artifact browser SSE missing %q: %s", want, body)
		}
	}
	for _, forbidden := range []string{
		"selector #workbench-v2-artifact-body",
		"selector #workbench-v2-comments-body",
		"selector #workbench-root",
	} {
		if strings.Contains(body, forbidden) {
			t.Fatalf("artifact browser SSE patched %q: %s", forbidden, body)
		}
	}
}

func TestHandleThreadArtifactBrowserRejectsUnsafeDirectory(t *testing.T) {
	t.Parallel()

	root := t.TempDir()
	mustWriteFile(t, filepath.Join(root, "safe.md"), []byte("# Safe"))
	outside := t.TempDir()
	if err := os.Symlink(outside, filepath.Join(root, "escape")); err != nil {
		t.Fatal(err)
	}
	svc, err := NewService(root, nil, nil)
	if err != nil {
		t.Fatal(err)
	}
	svc.WithWorkbenchThreadRenderer(&threadWorkbenchTestRenderer{})
	for _, directory := range []string{"thoughts/../outside", "thoughts/escape"} {
		t.Run(directory, func(t *testing.T) {
			rec := httptest.NewRecorder()
			requestURL := "/threads/thread-alpha/artifact-browser?artifact=thoughts/safe.md&artifact_dir=" + url.QueryEscape(
				directory,
			)
			c := echo.New().NewContext(
				httptest.NewRequest(http.MethodGet, requestURL, http.NoBody),
				rec,
			)
			c.SetParamNames("threadID")
			c.SetParamValues("thread-alpha")
			err := svc.HandleThreadArtifactBrowser(c)
			var httpErr *echo.HTTPError
			if !errors.As(err, &httpErr) || httpErr.Code != http.StatusBadRequest {
				t.Fatalf("HandleThreadArtifactBrowser() error = %#v", err)
			}
			if strings.Contains(strings.ToLower(httpErr.Message.(string)), "outside") {
				t.Fatalf("unsafe directory leaked resolver detail: %v", httpErr.Message)
			}
		})
	}
}

func TestHandleThreadArtifactDirectoryLoadsOnlyRequestedBranch(t *testing.T) {
	t.Parallel()

	root := t.TempDir()
	mustMkdirAll(t, filepath.Join(root, "owner", "plans", "alpha", "docs", "nested"))
	mustWriteFile(
		t,
		filepath.Join(root, "owner", "plans", "alpha", "docs", "note.md"),
		[]byte("# Note"),
	)
	mustWriteFile(
		t,
		filepath.Join(root, "owner", "plans", "alpha", "docs", "nested", "deep.md"),
		[]byte("# Deep"),
	)
	svc, err := NewService(root, nil, nil)
	if err != nil {
		t.Fatal(err)
	}
	svc.WithWorkbenchThreadRenderer(&threadWorkbenchTestRenderer{})
	rec := httptest.NewRecorder()
	c := echo.New().NewContext(
		httptest.NewRequest(
			http.MethodGet,
			"/threads/thread-alpha/artifact-directory?artifact=thoughts/owner/plans/alpha/design.md&artifact_dir=thoughts/owner/plans/alpha&directory=thoughts/owner/plans/alpha/docs",
			http.NoBody,
		),
		rec,
	)
	c.SetParamNames("threadID")
	c.SetParamValues("thread-alpha")
	if err := svc.HandleThreadArtifactDirectory(c); err != nil {
		t.Fatal(err)
	}
	body := rec.Body.String()
	for _, want := range []string{
		threadArtifactDirectoryID("owner/plans/alpha/docs"),
		`data-loaded="true"`,
		"note",
		"nested/",
		"artifact_dir=thoughts%2Fowner%2Fplans%2Falpha",
	} {
		if !strings.Contains(body, want) {
			t.Fatalf("directory SSE missing %q: %s", want, body)
		}
	}
	if strings.Contains(body, "Deep") ||
		strings.Contains(body, "workbench-v2-artifact-body") {
		t.Fatalf("directory toggle recursively loaded or replaced artifact: %s", body)
	}
}

func TestHandleThreadArtifactRejectsUnknownThreadAndDirectorySelection(t *testing.T) {
	t.Parallel()

	root := t.TempDir()
	mustMkdirAll(t, filepath.Join(root, "owner", "plans", "alpha", "docs"))
	svc, err := NewService(root, nil, nil)
	if err != nil {
		t.Fatal(err)
	}
	svc.WithWorkbenchThreadRenderer(
		&threadWorkbenchTestRenderer{threadPlanErr: sql.ErrNoRows},
	)
	for _, target := range []string{
		"/threads/missing/artifact-browser?artifact=thoughts/owner/plans/alpha/design.md&artifact_dir=thoughts/owner/plans/alpha/docs",
		"/threads/missing/artifact-directory?directory=thoughts/owner/plans/alpha/docs",
	} {
		rec := httptest.NewRecorder()
		c := echo.New().NewContext(
			httptest.NewRequest(http.MethodGet, target, http.NoBody),
			rec,
		)
		c.SetParamNames("threadID")
		c.SetParamValues("missing")
		var err error
		if strings.Contains(target, "artifact-browser") {
			err = svc.HandleThreadArtifactBrowser(c)
		}
		if strings.Contains(target, "artifact-directory") {
			err = svc.HandleThreadArtifactDirectory(c)
		}
		var httpErr *echo.HTTPError
		if !errors.As(err, &httpErr) || httpErr.Code != http.StatusNotFound {
			t.Fatalf("%s error = %#v", target, err)
		}
	}
}

func TestResolveThreadArtifactRejectsUnsafeExplicitArtifacts(t *testing.T) {
	root := t.TempDir()
	mustMkdirAll(t, filepath.Join(root, "owner", "plans", "alpha"))
	mustWriteFile(
		t,
		filepath.Join(root, "owner", "plans", "alpha", "design.md"),
		[]byte("# Alpha"),
	)
	outside := t.TempDir()
	mustWriteFile(t, filepath.Join(outside, "secret.md"), []byte("# Secret"))
	if err := os.Symlink(
		filepath.Join(outside, "secret.md"),
		filepath.Join(root, "escape.md"),
	); err != nil {
		t.Fatal(err)
	}
	svc, err := NewService(root, nil, nil)
	if err != nil {
		t.Fatal(err)
	}
	svc.WithWorkbenchThreadRenderer(&threadWorkbenchTestRenderer{})
	for _, artifact := range []string{"../secret.md", "owner/plans/alphax/doc.md", "thoughts/../secret.md", "escape.md"} {
		if _, err := svc.ResolveThreadArtifact(
			t.Context(),
			"thread-alpha",
			artifact,
		); err == nil {
			t.Fatalf("ResolveThreadArtifact(%q) succeeded", artifact)
		}
	}
}

func TestServeThreadsRendersDirectoryArtifactsAndKeepsAbsentArtifactNeutral(
	t *testing.T,
) {
	root := t.TempDir()
	mustMkdirAll(t, filepath.Join(root, "owner", "nested"))
	mustWriteFile(t, filepath.Join(root, "root.md"), []byte("# Root"))
	mustWriteFile(t, filepath.Join(root, "owner", "nested", "note.md"), []byte("# Note"))

	for _, tc := range []struct {
		name, target, wantArtifact, want, notWant string
	}{
		{name: "root", target: "/threads?artifact=thoughts/", want: "root", wantArtifact: ""},
		{name: "nested", target: "/threads?artifact=thoughts/owner/nested", want: "note", wantArtifact: "owner/nested"},
		{name: "absent", target: "/threads", want: "Select a thread to view an artifact.", notWant: "root.md"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			svc, err := NewService(root, nil, nil)
			if err != nil {
				t.Fatal(err)
			}
			r := &threadWorkbenchTestRenderer{}
			svc.WithWorkbenchThreadRenderer(r)
			rec := httptest.NewRecorder()
			c := echo.New().NewContext(httptest.NewRequest("GET", tc.target, nil), rec)
			if err := svc.ServeThreads(c); err != nil {
				t.Fatal(err)
			}
			if r.artifact != tc.wantArtifact {
				t.Fatalf("artifact = %q, want %q", r.artifact, tc.wantArtifact)
			}
			if !strings.Contains(rec.Body.String(), tc.want) {
				t.Fatalf("missing %q in %s", tc.want, rec.Body.String())
			}
			if tc.notWant != "" && strings.Contains(rec.Body.String(), tc.notWant) {
				t.Fatalf("unexpected %q in %s", tc.notWant, rec.Body.String())
			}
			if tc.name != "absent" {
				if strings.Contains(
					rec.Body.String(),
					`id="thoughts-directory-primary"`,
				) ||
					!strings.Contains(rec.Body.String(), `data-on:toggle`) {
					t.Fatalf(
						"directory rendered as fullscreen browser instead of disclosure tree: %s",
						rec.Body.String(),
					)
				}
			}
		})
	}
}
