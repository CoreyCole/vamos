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

func TestConsumeSSEStoppedWithValidXML(t *testing.T) {
	result := consumeFixture(
		t,
		completedFixture(validXML("review-design")),
		options{run: "run-1", stage: "review-design"},
	)
	if result.ExitCode != exitOK {
		t.Fatalf("ExitCode = %d, diagnostic = %s", result.ExitCode, result.Diagnostic)
	}
	if !strings.Contains(result.XML, "<stage>review-design</stage>") {
		t.Fatalf("stdout XML missing expected stage: %s", result.XML)
	}
	if strings.Contains(result.XML, "event:") || strings.Contains(result.XML, "data:") {
		t.Fatalf("stdout should be XML only, got %s", result.XML)
	}
}

func TestConsumeSSEHandlesMultilineDataAndComments(t *testing.T) {
	fixture := ": heartbeat\n" +
		"event: datastar-patch-elements\n" +
		sseData(
			"<div id=\"agent-chat-run-session-panel\"><p>run-1</p>\n<p>complete</p><pre>"+validXML(
				"design",
			)+"</pre></div>",
		) + "\n"
	result := consumeFixture(t, fixture, options{run: "run-1", stage: "design"})
	if result.ExitCode != exitOK {
		t.Fatalf("ExitCode = %d, diagnostic = %s", result.ExitCode, result.Diagnostic)
	}
}

func TestConsumeSSEStoppedWithoutXML(t *testing.T) {
	result := consumeFixture(
		t,
		completedFixture("plain assistant text"),
		options{run: "run-1", stage: "review-design"},
	)
	if result.ExitCode != exitInvalid {
		t.Fatalf("ExitCode = %d, want %d", result.ExitCode, exitInvalid)
	}
	if !strings.Contains(result.Diagnostic, "missing <qrspi-result>") {
		t.Fatalf("diagnostic missing parser error: %s", result.Diagnostic)
	}
}

func TestConsumeSSEMalformedXML(t *testing.T) {
	result := consumeFixture(
		t,
		completedFixture(`<qrspi-result><stage>review-design</stage>`),
		options{run: "run-1", stage: "review-design"},
	)
	if result.ExitCode != exitInvalid {
		t.Fatalf("ExitCode = %d, want %d", result.ExitCode, exitInvalid)
	}
	if !strings.Contains(result.Diagnostic, "valid QRSPI XML") {
		t.Fatalf("diagnostic missing valid XML context: %s", result.Diagnostic)
	}
}

func TestConsumeSSEStageMismatch(t *testing.T) {
	result := consumeFixture(
		t,
		completedFixture(validXML("review-outline")),
		options{run: "run-1", stage: "review-design"},
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
	result := consumeFixture(t, fixture, options{run: "run-1", stage: "review-design"})
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
		func() (string, error) { return validXML("outline"), nil },
	)
	if result == nil {
		t.Fatalf("waitFromDatabase() = nil, want completed result")
	}
	if result.ExitCode != exitOK {
		t.Fatalf("ExitCode = %d, diagnostic = %s", result.ExitCode, result.Diagnostic)
	}
	if !strings.Contains(result.XML, "<stage>outline</stage>") {
		t.Fatalf("unexpected XML: %s", result.XML)
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
		func() (string, error) { return validXML("outline"), nil },
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
		func() (string, error) { return validXML("outline"), nil },
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
	result := consumeSSE(ctx, reader, options{run: "run-1", stage: "review-design"}, nil)
	if result.ExitCode != exitTimeout {
		t.Fatalf("ExitCode = %d, want %d", result.ExitCode, exitTimeout)
	}
}

func TestExtractAndValidateXMLUnescapesHTML(t *testing.T) {
	escaped := strings.NewReplacer("<", "&lt;", ">", "&gt;").Replace(validXML("plan"))
	xmlText, err := extractAndValidateXML(`<code>`+escaped+`</code>`, "plan")
	if err != nil {
		t.Fatalf("extractAndValidateXML() error = %v", err)
	}
	if !strings.Contains(xmlText, "<stage>plan</stage>") {
		t.Fatalf("unexpected xml: %s", xmlText)
	}
}

func TestExtractAndValidateXMLFromChromaHTML(t *testing.T) {
	highlighted := `<span class="o">&lt;</span><span class="n">qrspi</span><span class="o">-</span><span class="n">result</span><span class="o">&gt;</span>
<span class="o">&lt;</span><span class="n">stage</span><span class="o">&gt;</span>design<span class="o">&lt;/</span><span class="n">stage</span><span class="o">&gt;</span>
<span class="o">&lt;</span><span class="n">status</span><span class="o">&gt;</span>complete<span class="o">&lt;/</span><span class="n">status</span><span class="o">&gt;</span>
<span class="o">&lt;</span><span class="n">outcome</span><span class="o">&gt;</span>complete<span class="o">&lt;/</span><span class="n">outcome</span><span class="o">&gt;</span>
<span class="o">&lt;</span><span class="n">policy</span><span class="o">&gt;</span><span class="o">&lt;</span><span class="n">autoMode</span><span class="o">&gt;</span>false<span class="o">&lt;/</span><span class="n">autoMode</span><span class="o">&gt;</span><span class="o">&lt;</span><span class="n">enablePlanReviews</span><span class="o">&gt;</span>true<span class="o">&lt;/</span><span class="n">enablePlanReviews</span><span class="o">&gt;</span><span class="o">&lt;</span><span class="n">invalidResultRetryLimit</span><span class="o">&gt;</span>1<span class="o">&lt;/</span><span class="n">invalidResultRetryLimit</span><span class="o">&gt;</span><span class="o">&lt;/</span><span class="n">policy</span><span class="o">&gt;</span>
<span class="o">&lt;</span><span class="n">summary</span><span class="o">&gt;</span>Done<span class="o">&lt;/</span><span class="n">summary</span><span class="o">&gt;</span>
<span class="o">&lt;</span><span class="n">artifact</span><span class="o">&gt;</span>thoughts/example/design.md<span class="o">&lt;/</span><span class="n">artifact</span><span class="o">&gt;</span>
<span class="o">&lt;</span><span class="n">next</span><span class="o">&gt;</span>/q-outline thoughts/example/design.md<span class="o">&lt;/</span><span class="n">next</span><span class="o">&gt;</span>
<span class="o">&lt;/</span><span class="n">qrspi</span><span class="o">-</span><span class="n">result</span><span class="o">&gt;</span>`
	xmlText, err := extractAndValidateXML(highlighted, "design")
	if err != nil {
		t.Fatalf("extractAndValidateXML() error = %v", err)
	}
	if !strings.Contains(xmlText, "<qrspi-result>") ||
		!strings.Contains(xmlText, "<stage>") ||
		!strings.Contains(xmlText, "design") {
		t.Fatalf("unexpected xml: %s", xmlText)
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

func validXML(stage string) string {
	return `<qrspi-result>
  <stage>` + stage + `</stage>
  <status>complete</status>
  <outcome>complete</outcome>
  <policy>
    <autoMode>false</autoMode>
    <enablePlanReviews>true</enablePlanReviews>
    <invalidResultRetryLimit>1</invalidResultRetryLimit>
  </policy>
  <summary>
    <plan-goal>Goal.</plan-goal>
    <stage-completed>Completed.</stage-completed>
    <key-decisions>None.</key-decisions>
  </summary>
  <artifact>thoughts/example/review.md</artifact>
  <next>/q-review thoughts/example/review.md</next>
</qrspi-result>`
}
