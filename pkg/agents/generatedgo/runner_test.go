package generatedgo

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"
)

func TestBuildAndRunSuccess(t *testing.T) {
	workspace := writeModule(t, generatedProgram(`
fmt.Fprintln(os.Stdout, "hello from generated app")
writeFile(out, "app.html", "<h1>pickleball</h1>")
writeFile(out, "results.csv", "court,team_a,team_b\n1,A+B,C+D\n")
writeManifest(out, "success-build", "seed")
`))
	output := t.TempDir()

	result, err := BuildAndRun(context.Background(), RunnerInput{
		WorkspaceDir:      workspace,
		OutputDir:         output,
		CompileTimeout:    30 * time.Second,
		RunTimeout:        5 * time.Second,
		ArtifactAllowlist: []string{"app.html", "results.csv"},
	})
	if err != nil {
		t.Fatalf("BuildAndRun() error = %v", err)
	}
	if result.Status != BuildStatusSucceeded {
		t.Fatalf("status = %q", result.Status)
	}
	if result.Manifest.BuildID != "success-build" {
		t.Fatalf("build id = %q", result.Manifest.BuildID)
	}
	for _, name := range []string{"app.html", "results.csv", "manifest.json"} {
		if result.ArtifactHashes[name] == "" {
			t.Fatalf("missing hash for %s: %#v", name, result.ArtifactHashes)
		}
	}
	if result.SourceHash == "" || !strings.HasPrefix(result.SourceHash, "sha256:") {
		t.Fatalf("source hash = %q", result.SourceHash)
	}
	if !strings.Contains(result.StdoutTail, "hello from generated app") {
		t.Fatalf("stdout tail = %q", result.StdoutTail)
	}
}

func TestBuildAndRunCompileTimeout(t *testing.T) {
	workspace := writeModule(t, generatedProgram(`writeManifest(out, "never", "never")`))
	output := t.TempDir()
	fakeBin := t.TempDir()
	writeExecutable(t, filepath.Join(fakeBin, "go"), "#!/bin/sh\necho compile-prefix\nsleep 5\necho compile-suffix\n")
	t.Setenv("PATH", fakeBin+string(os.PathListSeparator)+os.Getenv("PATH"))

	result, err := BuildAndRun(context.Background(), RunnerInput{
		WorkspaceDir:   workspace,
		OutputDir:      output,
		CompileTimeout: 25 * time.Millisecond,
		RunTimeout:     time.Second,
		LogLimitBytes:  64,
	})
	if err == nil {
		t.Fatal("BuildAndRun() error = nil, want compile timeout")
	}
	if result.Status != BuildStatusFailed {
		t.Fatalf("status = %q", result.Status)
	}
	if !strings.Contains(result.StdoutTail, "compile-prefix") {
		t.Fatalf("stdout tail = %q", result.StdoutTail)
	}
}

func TestBuildAndRunRunTimeout(t *testing.T) {
	workspace := writeModule(t, generatedProgram(`
fmt.Fprintln(os.Stdout, "run started")
time.Sleep(5 * time.Second)
`))
	output := t.TempDir()

	result, err := BuildAndRun(context.Background(), RunnerInput{
		WorkspaceDir:   workspace,
		OutputDir:      output,
		CompileTimeout: 30 * time.Second,
		RunTimeout:     25 * time.Millisecond,
	})
	if err == nil {
		t.Fatal("BuildAndRun() error = nil, want run timeout")
	}
	if result.Status != BuildStatusFailed {
		t.Fatalf("status = %q", result.Status)
	}
	if !strings.Contains(result.StdoutTail, "run started") {
		t.Fatalf("stdout tail = %q", result.StdoutTail)
	}
}

func TestBuildAndRunMissingArtifact(t *testing.T) {
	workspace := writeModule(t, generatedProgram(`
writeFile(out, "app.html", "<h1>missing csv</h1>")
writeManifest(out, "missing", "missing csv")
`))
	_, err := BuildAndRun(context.Background(), RunnerInput{WorkspaceDir: workspace, OutputDir: t.TempDir()})
	if err == nil || !strings.Contains(err.Error(), "results.csv") {
		t.Fatalf("err = %v, want missing results.csv", err)
	}
}

func TestValidateManifestRejectsPathEscape(t *testing.T) {
	output := t.TempDir()
	writeFile(t, filepath.Join(output, "app.html"), "ok")
	writeFile(t, filepath.Join(output, "results.csv"), "ok")
	manifest := `{"schema_version":1,"build_id":"escape","mode":"one_shot","artifacts":{"html":"../app.html","csv":"results.csv"}}`
	path := filepath.Join(output, "manifest.json")
	writeFile(t, path, manifest)

	_, err := ValidateManifest(path, output)
	if err == nil || !strings.Contains(err.Error(), "html artifact") {
		t.Fatalf("err = %v, want html artifact validation failure", err)
	}
}

func TestBuildAndRunRejectsOutputSymlink(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("symlink creation needs privileges on Windows")
	}
	workspace := writeModule(t, generatedProgram(`
writeFile(out, "app.html", "<h1>ok</h1>")
writeFile(out, "results.csv", "court\n1\n")
writeManifest(out, "symlink", "symlink")
if err := os.Symlink("/tmp", filepath.Join(out, "link")); err != nil { panic(err) }
`))
	_, err := BuildAndRun(context.Background(), RunnerInput{WorkspaceDir: workspace, OutputDir: t.TempDir()})
	if err == nil || !strings.Contains(err.Error(), "symlink") {
		t.Fatalf("err = %v, want symlink rejection", err)
	}
}

func TestBuildAndRunLogCapKeepsTail(t *testing.T) {
	workspace := writeModule(t, generatedProgram(`
fmt.Fprint(os.Stdout, strings.Repeat("A", 128)+"TAIL")
fmt.Fprint(os.Stderr, strings.Repeat("B", 128)+"ERRTAIL")
writeFile(out, "app.html", "<h1>logs</h1>")
writeFile(out, "results.csv", "court\n1\n")
writeManifest(out, "logs", "logs")
`))
	result, err := BuildAndRun(context.Background(), RunnerInput{
		WorkspaceDir:   workspace,
		OutputDir:      t.TempDir(),
		LogLimitBytes:  16,
		CompileTimeout: 30 * time.Second,
		RunTimeout:     time.Second,
	})
	if err != nil {
		t.Fatalf("BuildAndRun() error = %v", err)
	}
	if len(result.StdoutTail) > 16 || !strings.HasSuffix(result.StdoutTail, "TAIL") {
		t.Fatalf("stdout tail = %q", result.StdoutTail)
	}
	if len(result.StderrTail) > 16 || !strings.HasSuffix(result.StderrTail, "ERRTAIL") {
		t.Fatalf("stderr tail = %q", result.StderrTail)
	}
}

func TestHashSourceRejectsSymlink(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("symlink creation needs privileges on Windows")
	}
	root := t.TempDir()
	writeFile(t, filepath.Join(root, "main.go"), "package main\n")
	if err := os.Symlink("/tmp", filepath.Join(root, "link.go")); err != nil {
		t.Fatalf("symlink: %v", err)
	}
	_, err := HashSource(root)
	if err == nil || !strings.Contains(err.Error(), "symlink") {
		t.Fatalf("err = %v, want source symlink rejection", err)
	}
}

func TestCopySnapshotCopiesSourceAndArtifacts(t *testing.T) {
	workspace := writeModule(t, generatedProgram(``))
	output := t.TempDir()
	writeFile(t, filepath.Join(output, "app.html"), "<h1>snapshot</h1>")
	writeFile(t, filepath.Join(output, "results.csv"), "court\n1\n")
	writeFile(t, filepath.Join(output, "manifest.json"), `{"schema_version":1,"build_id":"copy","mode":"one_shot","artifacts":{"html":"app.html","csv":"results.csv"}}`)

	result, err := CopySnapshot(SnapshotInput{
		SourceDir:   workspace,
		OutputDir:   output,
		SnapshotDir: t.TempDir(),
		Allowlist:   []string{"app.html", "results.csv", "manifest.json"},
	})
	if err != nil {
		t.Fatalf("CopySnapshot() error = %v", err)
	}
	if result.SourceHash == "" || result.ArtifactHashes["app.html"] == "" {
		t.Fatalf("bad snapshot result: %#v", result)
	}
	if _, err := os.Stat(filepath.Join(result.SnapshotDir, "source", "main.go")); err != nil {
		t.Fatalf("copied source main.go: %v", err)
	}
}

func writeModule(t *testing.T, main string) string {
	t.Helper()
	workspace := t.TempDir()
	writeFile(t, filepath.Join(workspace, "go.mod"), "module generated.test/app\n\ngo 1.24\n")
	writeFile(t, filepath.Join(workspace, "main.go"), main)
	return workspace
}

func generatedProgram(body string) string {
	return fmt.Sprintf(`package main

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"
)

type manifest struct {
	SchemaVersion int `+"`json:\"schema_version\"`"+`
	BuildID string `+"`json:\"build_id\"`"+`
	Mode string `+"`json:\"mode\"`"+`
	PromptSummary string `+"`json:\"prompt_summary\"`"+`
	Artifacts artifacts `+"`json:\"artifacts\"`"+`
}

type artifacts struct {
	HTML string `+"`json:\"html\"`"+`
	CSV string `+"`json:\"csv\"`"+`
}

var (
	_ = fmt.Sprintf
	_ = strings.Repeat
	_ = time.Second
)

func main() {
	out := os.Getenv("VAMOS_GENERATED_OUTPUT_DIR")
	if out == "" { panic("missing output dir") }
	if err := os.MkdirAll(out, 0o755); err != nil { panic(err) }
	%s
}

func writeFile(root, name, data string) {
	if err := os.WriteFile(filepath.Join(root, name), []byte(data), 0o644); err != nil { panic(err) }
}

func writeManifest(root, buildID, summary string) {
	data, err := json.Marshal(manifest{SchemaVersion: 1, BuildID: buildID, Mode: "one_shot", PromptSummary: summary, Artifacts: artifacts{HTML: "app.html", CSV: "results.csv"}})
	if err != nil { panic(err) }
	writeFile(root, "manifest.json", string(data))
}
`, body)
}

func writeFile(t *testing.T, path string, data string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	if err := os.WriteFile(path, []byte(data), 0o644); err != nil {
		t.Fatalf("write %s: %v", path, err)
	}
}

func writeExecutable(t *testing.T, path string, data string) {
	t.Helper()
	writeFile(t, path, data)
	if err := os.Chmod(path, 0o755); err != nil {
		t.Fatalf("chmod %s: %v", path, err)
	}
}
