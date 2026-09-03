package markdown

import (
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"testing"

	"github.com/labstack/echo/v4"

	"github.com/CoreyCole/vamos/server/services/applets"
)

func TestServeMarkdownRedirectsAppletManifestToRenderApp(t *testing.T) {
	root := t.TempDir()
	cases := []struct {
		rel  string
		body string
	}{
		{
			rel: "v2-wordle/AGENTS.md",
			body: `---
vamos_artifact: applet
applet:
  id: e2e-v2-wordle
  kind: datastar
  title: Wordle
  source_dir: app
  start_command: ["wordle"]
---
# Wordle
`,
		},
		{
			rel: "v2-streamlit/AGENTS.md",
			body: `---
vamos_artifact: applet
applet:
  id: e2e-v2-streamlit
  kind: streamlit
  title: Streamlit
  source_dir: app
  start_command: ["streamlit", "run", "app.py"]
---
# Streamlit
`,
		},
	}
	for _, tc := range cases {
		mustMkdirAll(t, filepath.Join(root, filepath.Dir(tc.rel)))
		mustWriteFile(t, filepath.Join(root, filepath.FromSlash(tc.rel)), []byte(tc.body))
	}
	svc, err := NewService(root, nil, nil)
	if err != nil {
		t.Fatal(err)
	}

	for _, tc := range cases {
		t.Run(tc.rel, func(t *testing.T) {
			e := echo.New()
			rec := httptest.NewRecorder()
			c := e.NewContext(
				httptest.NewRequest(http.MethodGet, "/thoughts/"+tc.rel, nil),
				rec,
			)
			c.SetParamNames("*")
			c.SetParamValues(tc.rel)
			if err := svc.ServeMarkdown(c); err != nil {
				t.Fatal(err)
			}
			if rec.Code != http.StatusSeeOther {
				t.Fatalf("status = %d, want %d body=%s", rec.Code, http.StatusSeeOther, rec.Body.String())
			}
			want := "/thoughts/_render/app/" + applets.EncodeAppletIdentity("thoughts/"+tc.rel)
			if got := rec.Header().Get("Location"); got != want {
				t.Fatalf("Location = %q, want %q", got, want)
			}
		})
	}
}
