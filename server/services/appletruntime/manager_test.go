package appletruntime

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"
)

func TestManagerStartHealthProxyAndStop(t *testing.T) {
	filesRoot, sourceDir, bin := writeAppletSource(t, `
package main
import (
  "fmt"
  "net/http"
  "os"
)
func main() {
  port := os.Getenv("PORT")
  http.HandleFunc("/healthz", func(w http.ResponseWriter, r *http.Request) { fmt.Fprint(w, "ok") })
  http.HandleFunc("/echo/", func(w http.ResponseWriter, r *http.Request) { fmt.Fprint(w, r.URL.Path) })
  if err := http.ListenAndServe("127.0.0.1:"+port, nil); err != nil { panic(err) }
}
`)
	manager := NewManager(t.TempDir())
	state, err := manager.Start(context.Background(), RuntimeConfig{
		AppID:        "pickleball",
		FilesRoot:    filesRoot,
		SourceDir:    sourceDir,
		BuildCommand: []string{"go", "build", "-o", bin, "."},
		StartCommand: []string{bin},
		HealthPath:   "/healthz",
	})
	if err != nil {
		t.Fatalf("Start() error = %v", err)
	}
	if !state.Healthy || state.Port == 0 || state.PID == 0 || state.BaseURL == "" {
		t.Fatalf("unexpected process state: %+v", state)
	}

	server := httptest.NewServer(NewAppletProxy(
		manager,
		AppletProxyMatch{AppID: "pickleball", StripPrefix: "/examples/pickleball/app"},
		ProxyOptions{FlushSSE: true},
	))
	defer server.Close()
	resp, err := http.Get(server.URL + "/examples/pickleball/app/echo/rounds")
	if err != nil {
		t.Fatalf("proxy get: %v", err)
	}
	defer resp.Body.Close()
	body, _ := io.ReadAll(resp.Body)
	if string(body) != "/echo/rounds" {
		t.Fatalf("proxy forwarded path = %q", body)
	}

	if err := manager.Stop(context.Background(), "pickleball"); err != nil {
		t.Fatalf("Stop() error = %v", err)
	}
	if _, ok := manager.ProxyTarget("pickleball"); ok {
		t.Fatal("ProxyTarget still active after Stop")
	}
}

func TestEnsureStartedReusesHealthyProcess(t *testing.T) {
	filesRoot, sourceDir, bin := writeAppletSource(t, `
package main
import (
  "fmt"
  "net/http"
  "os"
)
func main() {
  port := os.Getenv("PORT")
  http.HandleFunc("/healthz", func(w http.ResponseWriter, r *http.Request) { fmt.Fprint(w, "ok") })
  if err := http.ListenAndServe("127.0.0.1:"+port, nil); err != nil { panic(err) }
}
`)
	manager := NewManager(t.TempDir())
	cfg := RuntimeConfig{
		AppID:        "pickleball",
		FilesRoot:    filesRoot,
		SourceDir:    sourceDir,
		BuildCommand: []string{"go", "build", "-o", bin, "."},
		StartCommand: []string{bin},
		HealthPath:   "/healthz",
		IdleTimeout:  time.Minute,
	}
	first, err := manager.EnsureStarted(context.Background(), cfg)
	if err != nil {
		t.Fatalf("first EnsureStarted() error = %v", err)
	}
	second, err := manager.EnsureStarted(context.Background(), cfg)
	if err != nil {
		t.Fatalf("second EnsureStarted() error = %v", err)
	}
	defer manager.Stop(context.Background(), "pickleball")
	if second.Port != first.Port || second.PID != first.PID || second.Status != ProcessStatusHealthy {
		t.Fatalf("EnsureStarted() did not reuse healthy process: first=%+v second=%+v", first, second)
	}
}

func TestTouchAndSweepInactive(t *testing.T) {
	filesRoot, sourceDir, bin := writeAppletSource(t, `
package main
import (
  "fmt"
  "net/http"
  "os"
)
func main() {
  port := os.Getenv("PORT")
  http.HandleFunc("/healthz", func(w http.ResponseWriter, r *http.Request) { fmt.Fprint(w, "ok") })
  if err := http.ListenAndServe("127.0.0.1:"+port, nil); err != nil { panic(err) }
}
`)
	manager := NewManager(t.TempDir())
	state, err := manager.Start(context.Background(), RuntimeConfig{
		AppID:        "pickleball",
		FilesRoot:    filesRoot,
		SourceDir:    sourceDir,
		BuildCommand: []string{"go", "build", "-o", bin, "."},
		StartCommand: []string{bin},
		HealthPath:   "/healthz",
		IdleTimeout:  time.Millisecond,
	})
	if err != nil {
		t.Fatalf("Start() error = %v", err)
	}
	manager.Touch("pickleball", 1)
	if active, err := manager.Health(context.Background(), "pickleball"); err != nil || active.ActiveConnections != 1 {
		t.Fatalf("Health() after Touch(+1) = %+v, %v", active, err)
	}
	stopped, err := manager.SweepInactive(context.Background(), state.LastSeenAt.Add(time.Hour))
	if err != nil {
		t.Fatalf("SweepInactive() with active connection error = %v", err)
	}
	if len(stopped) != 0 {
		t.Fatalf("SweepInactive() stopped active applet: %+v", stopped)
	}
	manager.Touch("pickleball", -2)
	stopped, err = manager.SweepInactive(context.Background(), time.Now().Add(time.Hour))
	if err != nil {
		t.Fatalf("SweepInactive() error = %v", err)
	}
	if len(stopped) != 1 || stopped[0].Status != ProcessStatusStopped || stopped[0].AppID != "pickleball" {
		t.Fatalf("SweepInactive() stopped = %+v", stopped)
	}
	if _, ok := manager.ProxyTarget("pickleball"); ok {
		t.Fatal("ProxyTarget still active after idle sweep")
	}
}

func TestSweepInactiveSkipsZeroTimeout(t *testing.T) {
	filesRoot, sourceDir, bin := writeAppletSource(t, `
package main
import (
  "fmt"
  "net/http"
  "os"
)
func main() {
  port := os.Getenv("PORT")
  http.HandleFunc("/healthz", func(w http.ResponseWriter, r *http.Request) { fmt.Fprint(w, "ok") })
  if err := http.ListenAndServe("127.0.0.1:"+port, nil); err != nil { panic(err) }
}
`)
	manager := NewManager(t.TempDir())
	state, err := manager.Start(context.Background(), RuntimeConfig{
		AppID:        "pickleball",
		FilesRoot:    filesRoot,
		SourceDir:    sourceDir,
		BuildCommand: []string{"go", "build", "-o", bin, "."},
		StartCommand: []string{bin},
		HealthPath:   "/healthz",
		IdleTimeout:  0,
	})
	if err != nil {
		t.Fatalf("Start() error = %v", err)
	}
	defer manager.Stop(context.Background(), "pickleball")
	stopped, err := manager.SweepInactive(context.Background(), state.LastSeenAt.Add(24*time.Hour))
	if err != nil {
		t.Fatalf("SweepInactive() error = %v", err)
	}
	if len(stopped) != 0 {
		t.Fatalf("SweepInactive() stopped zero-timeout applet: %+v", stopped)
	}
}

func TestFailedStartLeavesPreviousProcessActive(t *testing.T) {
	filesRoot, sourceDir, bin := writeAppletSource(t, `
package main
import (
  "fmt"
  "net/http"
  "os"
)
func main() {
  port := os.Getenv("PORT")
  http.HandleFunc("/healthz", func(w http.ResponseWriter, r *http.Request) { fmt.Fprint(w, "ok") })
  http.HandleFunc("/version", func(w http.ResponseWriter, r *http.Request) { fmt.Fprint(w, "one") })
  if err := http.ListenAndServe("127.0.0.1:"+port, nil); err != nil { panic(err) }
}
`)
	manager := NewManager(t.TempDir())
	first, err := manager.Start(context.Background(), RuntimeConfig{
		AppID:        "pickleball",
		FilesRoot:    filesRoot,
		SourceDir:    sourceDir,
		BuildCommand: []string{"go", "build", "-o", bin, "."},
		StartCommand: []string{bin},
		HealthPath:   "/healthz",
	})
	if err != nil {
		t.Fatalf("first Start() error = %v", err)
	}
	defer manager.Stop(context.Background(), "pickleball")

	badSource := filepath.Join(filesRoot, "apps", "bad")
	if err := os.MkdirAll(badSource, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(badSource, "go.mod"), []byte("module bad\n\ngo 1.25.1\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(badSource, "main.go"), []byte(`package main
func main() { select{} }
`), 0o644); err != nil {
		t.Fatal(err)
	}
	badBin := filepath.Join(badSource, "bad"+exeSuffix())
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	_, err = manager.Start(ctx, RuntimeConfig{
		AppID:        "pickleball",
		FilesRoot:    filesRoot,
		SourceDir:    badSource,
		BuildCommand: []string{"go", "build", "-o", badBin, "."},
		StartCommand: []string{badBin},
		HealthPath:   "/healthz",
	})
	if err == nil {
		t.Fatal("second Start() unexpectedly succeeded")
	}
	target, ok := manager.ProxyTarget("pickleball")
	if !ok {
		t.Fatal("previous process was not preserved")
	}
	if target != first.BaseURL {
		t.Fatalf("active target = %q, want %q", target, first.BaseURL)
	}
}

func TestStartRejectsSourceOutsideFilesRoot(t *testing.T) {
	root := t.TempDir()
	outside := t.TempDir()
	manager := NewManager(t.TempDir())
	_, err := manager.Start(context.Background(), RuntimeConfig{
		AppID:        "pickleball",
		FilesRoot:    root,
		SourceDir:    outside,
		StartCommand: []string{"go", "version"},
	})
	if err == nil || !strings.Contains(err.Error(), "source dir must be inside files root") {
		t.Fatalf("Start() error = %v", err)
	}
}

func TestStartRejectsSymlinkedSourceOutsideFilesRoot(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("symlink permissions vary on windows")
	}
	root := t.TempDir()
	outside := t.TempDir()
	link := filepath.Join(root, "apps", "current")
	if err := os.MkdirAll(filepath.Dir(link), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(outside, link); err != nil {
		t.Fatal(err)
	}
	manager := NewManager(t.TempDir())
	_, err := manager.Start(context.Background(), RuntimeConfig{
		AppID:        "pickleball",
		FilesRoot:    root,
		SourceDir:    link,
		StartCommand: []string{"go", "version"},
	})
	if err == nil || !strings.Contains(err.Error(), "source dir must be inside files root") {
		t.Fatalf("Start() error = %v", err)
	}
}

func writeAppletSource(t *testing.T, mainGo string) (filesRoot, sourceDir, bin string) {
	t.Helper()
	filesRoot = t.TempDir()
	sourceDir = filepath.Join(filesRoot, "apps", "current")
	if err := os.MkdirAll(sourceDir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(sourceDir, "go.mod"), []byte("module applet\n\ngo 1.25.1\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(sourceDir, "main.go"), []byte(mainGo), 0o644); err != nil {
		t.Fatal(err)
	}
	bin = filepath.Join(sourceDir, fmt.Sprintf("applet%s", exeSuffix()))
	return filesRoot, sourceDir, bin
}

func exeSuffix() string {
	if runtime.GOOS == "windows" {
		return ".exe"
	}
	return ""
}
