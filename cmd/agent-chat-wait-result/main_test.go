package main

import (
	"context"
	"database/sql"
	"io"
	"os"
	"strings"
	"testing"
	"time"

	_ "modernc.org/sqlite"
)

func TestConsumeSSEStoppedWithValidYAML(t *testing.T) {
	result := consumeFixture(
		t,
		completedFixture(validYAML("design")),
		options{run: "run-1", stage: "design"},
	)
	if result.ExitCode != exitOK {
		t.Fatalf("ExitCode = %d, diagnostic = %s", result.ExitCode, result.Diagnostic)
	}
	if !strings.Contains(result.Result, `stage: "design"`) {
		t.Fatalf("stdout YAML missing expected stage: %s", result.Result)
	}
	if strings.Contains(result.Result, "event:") || strings.Contains(result.Result, "data:") {
		t.Fatalf("stdout should be YAML only, got %s", result.Result)
	}
}

func TestConsumeSSEHandlesMultilineDataAndComments(t *testing.T) {
	fixture := ": heartbeat\n" +
		"event: datastar-patch-elements\n" +
		sseData(
			"<div id=\"agent-chat-run-session-panel\"><p>run-1</p>\n<p>complete</p><pre>"+validYAML(
				"design",
			)+"</pre></div>",
		) + "\n"
	result := consumeFixture(t, fixture, options{run: "run-1", stage: "design"})
	if result.ExitCode != exitOK {
		t.Fatalf("ExitCode = %d, diagnostic = %s", result.ExitCode, result.Diagnostic)
	}
}

func TestConsumeSSEStoppedWithoutYAML(t *testing.T) {
	result := consumeFixture(
		t,
		completedFixture("plain assistant text"),
		options{run: "run-1", stage: "design"},
	)
	if result.ExitCode != exitInvalid {
		t.Fatalf("ExitCode = %d, want %d", result.ExitCode, exitInvalid)
	}
	if !strings.Contains(result.Diagnostic, "missing fenced YAML qrspi_result") {
		t.Fatalf("diagnostic missing parser error: %s", result.Diagnostic)
	}
}

func TestConsumeSSEMalformedYAML(t *testing.T) {
	result := consumeFixture(
		t,
		completedFixture("```yaml\nqrspi_result:\n  stage: [\n```"),
		options{run: "run-1", stage: "design"},
	)
	if result.ExitCode != exitInvalid {
		t.Fatalf("ExitCode = %d, want %d", result.ExitCode, exitInvalid)
	}
	if !strings.Contains(result.Diagnostic, "valid QRSPI YAML") {
		t.Fatalf("diagnostic missing valid YAML context: %s", result.Diagnostic)
	}
}

func TestConsumeSSEStageMismatch(t *testing.T) {
	result := consumeFixture(
		t,
		completedFixture(validYAML("review-outline")),
		options{run: "run-1", stage: "design"},
	)
	if result.ExitCode != exitInvalid {
		t.Fatalf("ExitCode = %d, want %d", result.ExitCode, exitInvalid)
	}
	if !strings.Contains(result.Diagnostic, "does not match expected workflow node") {
		t.Fatalf("diagnostic missing stage mismatch: %s", result.Diagnostic)
	}
}

func TestConsumeSSEFailedRunPatch(t *testing.T) {
	fixture := "event: datastar-patch-elements\n" +
		"data: <div id=\"agent-chat-run-session-panel\"><p>run-1</p><p>failed</p><p>boom</p></div>\n\n"
	result := consumeFixture(t, fixture, options{run: "run-1", stage: "design"})
	if result.ExitCode != exitFailed {
		t.Fatalf(
			"ExitCode = %d, want %d, diagnostic=%s",
			result.ExitCode,
			exitFailed,
			result.Diagnostic,
		)
	}
}

func TestWaitFromDatabaseCompletedRunUsesFinalPage(t *testing.T) {
	databasePath := createRunStatusDB(
		t,
		"run-1",
		"workspace-1",
		"thread-1",
		"complete",
		"",
	)
	result := waitFromDatabase(
		context.Background(),
		options{
			database:  databasePath,
			run:       "run-1",
			workspace: "workspace-1",
			thread:    "thread-1",
			stage:     "outline",
		},
		func() (string, error) { return validYAML("outline"), nil },
	)
	if result == nil {
		t.Fatalf("waitFromDatabase() = nil, want completed result")
	}
	if result.ExitCode != exitOK {
		t.Fatalf("ExitCode = %d, diagnostic = %s", result.ExitCode, result.Diagnostic)
	}
	if !strings.Contains(result.Result, `stage: "outline"`) {
		t.Fatalf("unexpected YAML: %s", result.Result)
	}
}

func TestWaitFromDatabaseFailedRunReturnsFailure(t *testing.T) {
	databasePath := createRunStatusDB(
		t,
		"run-1",
		"workspace-1",
		"thread-1",
		"failed",
		"boom",
	)
	result := waitFromDatabase(
		context.Background(),
		options{
			database:  databasePath,
			run:       "run-1",
			workspace: "workspace-1",
			thread:    "thread-1",
		},
		func() (string, error) { return validYAML("outline"), nil },
	)
	if result == nil {
		t.Fatalf("waitFromDatabase() = nil, want failed result")
	}
	if result.ExitCode != exitFailed {
		t.Fatalf("ExitCode = %d, want %d", result.ExitCode, exitFailed)
	}
	if !strings.Contains(result.Diagnostic, "boom") {
		t.Fatalf("diagnostic missing DB error: %s", result.Diagnostic)
	}
}

func TestWaitFromDatabaseRunningRunFallsBackToSSE(t *testing.T) {
	databasePath := createRunStatusDB(
		t,
		"run-1",
		"workspace-1",
		"thread-1",
		"running",
		"",
	)
	result := waitFromDatabase(
		context.Background(),
		options{
			database:  databasePath,
			run:       "run-1",
			workspace: "workspace-1",
			thread:    "thread-1",
		},
		func() (string, error) { return validYAML("outline"), nil },
	)
	if result != nil {
		t.Fatalf("waitFromDatabase() = %#v, want nil for running run", result)
	}
}

func TestReadNetscapeCookieJarIncludesHttpOnlyCookies(t *testing.T) {
	path := t.TempDir() + "/cookies.txt"
	content := "# Netscape HTTP Cookie File\n#HttpOnly_localhost\tFALSE\t/\tFALSE\t0\tsession\tabc123\nlocalhost\tFALSE\t/\tFALSE\t0\ttheme\tdark\n"
	if err := os.WriteFile(path, []byte(content), 0o600); err != nil {
		t.Fatalf("WriteFile() error = %v", err)
	}
	cookies, err := readNetscapeCookieJar(path)
	if err != nil {
		t.Fatalf("readNetscapeCookieJar() error = %v", err)
	}
	if !strings.Contains(cookies, "session=abc123") ||
		!strings.Contains(cookies, "theme=dark") {
		t.Fatalf("cookies missing expected pairs: %s", cookies)
	}
}

func TestConsumeSSETimeoutBeforeStop(t *testing.T) {
	reader, writer := io.Pipe()
	defer writer.Close()
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Millisecond)
	defer cancel()
	result := consumeSSE(ctx, reader, options{run: "run-1", stage: "design"}, nil)
	if result.ExitCode != exitTimeout {
		t.Fatalf("ExitCode = %d, want %d", result.ExitCode, exitTimeout)
	}
}

func TestExtractAndValidateYAMLUnescapesHTML(t *testing.T) {
	escaped := strings.NewReplacer("<", "&lt;", ">", "&gt;").Replace(validYAML("plan"))
	yamlText, err := extractAndValidateYAML(`<code>`+escaped+`</code>`, "plan")
	if err != nil {
		t.Fatalf("extractAndValidateYAML() error = %v", err)
	}
	if !strings.Contains(yamlText, `stage: "plan"`) {
		t.Fatalf("unexpected YAML: %s", yamlText)
	}
}

func TestExtractAndValidateYAMLFromChromaHTML(t *testing.T) {
	highlighted := `<span class="s">&#96;&#96;&#96;yaml</span>
<span class="nt">qrspi_result</span><span class="p">:</span>
  <span class="nt">stage</span><span class="p">:</span> <span class="s">"design"</span>
  <span class="nt">status</span><span class="p">:</span> <span class="s">"complete"</span>
  <span class="nt">outcome</span><span class="p">:</span> <span class="s">"complete"</span>
  <span class="nt">policy</span><span class="p">:</span>
    <span class="nt">auto_mode</span><span class="p">:</span> <span class="kc">false</span>
    <span class="nt">enable_plan_reviews</span><span class="p">:</span> <span class="kc">true</span>
    <span class="nt">invalid_result_retry_limit</span><span class="p">:</span> <span class="m">1</span>
  <span class="nt">summary</span><span class="p">:</span>
    <span class="nt">plan_goal</span><span class="p">:</span> <span class="s">"Goal."</span>
    <span class="nt">stage_completed</span><span class="p">:</span> <span class="s">"Completed."</span>
    <span class="nt">key_decisions</span><span class="p">:</span> <span class="s">"None."</span>
  <span class="nt">artifact</span><span class="p">:</span> <span class="s">"thoughts/example/design.md"</span>
  <span class="nt">next</span><span class="p">:</span>
    <span class="nt">steps</span><span class="p">:</span>
      - <span class="nt">action</span><span class="p">:</span> <span class="s">"start_stage"</span>
        <span class="nt">param</span><span class="p">:</span> <span class="s">"q-outline"</span>
<span class="s">&#96;&#96;&#96;</span>`
	yamlText, err := extractAndValidateYAML(highlighted, "design")
	if err != nil {
		t.Fatalf("extractAndValidateYAML() error = %v", err)
	}
	if !strings.Contains(yamlText, "qrspi_result:") ||
		!strings.Contains(yamlText, `stage: "design"`) {
		t.Fatalf("unexpected YAML: %s", yamlText)
	}
}

func consumeFixture(t *testing.T, fixture string, opts options) waitResult {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()
	return consumeSSE(ctx, strings.NewReader(fixture), opts, nil)
}

func completedFixture(body string) string {
	return "event: datastar-patch-elements\n" +
		sseData("<div id=\"agent-chat-live-transcript\">"+body+"</div>") + "\n" +
		"event: datastar-patch-elements\n" +
		sseData(
			"<div id=\"agent-chat-run-session-panel\"><p>run-1</p><p>complete</p></div>",
		) + "\n"
}

func createRunStatusDB(
	t *testing.T,
	runID string,
	workspaceID string,
	threadID string,
	status string,
	errorMessage string,
) string {
	t.Helper()
	path := t.TempDir() + "/agents.db"
	db, err := sql.Open("sqlite", path)
	if err != nil {
		t.Fatalf("sql.Open() error = %v", err)
	}
	defer db.Close()
	_, err = db.Exec(`create table agent_runs (
		id text primary key,
		workspace_id text,
		thread_id text,
		status text not null,
		error_message text
	)`)
	if err != nil {
		t.Fatalf("create table error = %v", err)
	}
	_, err = db.Exec(
		`insert into agent_runs (id, workspace_id, thread_id, status, error_message) values (?, ?, ?, ?, ?)`,
		runID,
		workspaceID,
		threadID,
		status,
		errorMessage,
	)
	if err != nil {
		t.Fatalf("insert run error = %v", err)
	}
	return path
}

func sseData(value string) string {
	lines := strings.Split(value, "\n")
	for i, line := range lines {
		lines[i] = "data: " + line
	}
	return strings.Join(lines, "\n") + "\n"
}

func validYAML(stage string) string {
	return "```yaml\n" + `qrspi_result:
  stage: "` + stage + `"
  status: "complete"
  outcome: "complete"
  policy:
    auto_mode: false
    enable_plan_reviews: true
    invalid_result_retry_limit: 1
  summary:
    plan_goal: "Goal."
    stage_completed: "Completed."
    key_decisions: "None."
  artifact: "thoughts/example/review.md"
  next:
    steps:
      - action: "start_stage"
        param: "q-review"
` + "```"
}
