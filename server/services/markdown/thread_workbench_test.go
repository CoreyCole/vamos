package markdown

import (
	"context"
	"net/http/httptest"
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

func TestServeThreadsHydratesArtifactAndCarriesIt(t *testing.T) {
	root := t.TempDir()
	mustMkdirAll(t, filepath.Join(root, "owner/plans/alpha"))
	mustWriteFile(
		t,
		filepath.Join(root, "owner/plans/alpha/design.md"),
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
	mustMkdirAll(t, filepath.Join(root, "owner/plans/alpha"))
	mustMkdirAll(t, filepath.Join(root, "other/plans/beta/docs"))
	mustWriteFile(
		t,
		filepath.Join(root, "owner/plans/alpha/design.md"),
		[]byte("# Alpha"),
	)
	mustWriteFile(
		t,
		filepath.Join(root, "other/plans/beta/review.md"),
		[]byte("# Beta review"),
	)
	mustWriteFile(
		t,
		filepath.Join(root, "other/plans/beta/docs/note.md"),
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
				httptest.NewRequest("GET", "/threads/thread-alpha?artifact="+tc.doc, nil), rec,
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
			if strings.Contains(rec.Body.String(), "Alpha") {
				t.Fatalf(
					"default plan artifact replaced cross-plan artifact: %s",
					rec.Body.String(),
				)
			}
		})
	}
}

func TestResolveThreadArtifactRejectsUnsafeExplicitArtifacts(t *testing.T) {
	root := t.TempDir()
	mustMkdirAll(t, filepath.Join(root, "owner/plans/alpha"))
	mustWriteFile(
		t,
		filepath.Join(root, "owner/plans/alpha/design.md"),
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
	mustMkdirAll(t, filepath.Join(root, "owner/nested"))
	mustWriteFile(t, filepath.Join(root, "root.md"), []byte("# Root"))
	mustWriteFile(t, filepath.Join(root, "owner/nested/note.md"), []byte("# Note"))

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
		})
	}
}
