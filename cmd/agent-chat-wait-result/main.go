package main

import (
	"bufio"
	"context"
	"database/sql"
	"errors"
	"flag"
	"fmt"
	"html"
	"io"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"time"

	_ "modernc.org/sqlite"

	"github.com/CoreyCole/vamos/pkg/agents/workflows/qrspi"
	wruntime "github.com/CoreyCole/vamos/pkg/agents/workflows/runtime"
)

const (
	exitOK       = 0
	exitInvalid  = 1
	exitFailed   = 2
	exitTimeout  = 124
	defaultSince = "0"
)

type options struct {
	baseURL   string
	workspace string
	thread    string
	run       string
	stage     string
	timeout   time.Duration
	cookie    string
	cookieJar string
	database  string
	headers   headerFlags
}

type headerFlags []string

func (h *headerFlags) String() string { return strings.Join(*h, ",") }
func (h *headerFlags) Set(value string) error {
	value = strings.TrimSpace(value)
	if value != "" {
		*h = append(*h, value)
	}
	return nil
}

type waitResult struct {
	XML        string
	ExitCode   int
	Diagnostic string
}

type sseFrame struct {
	Event string
	Data  string
}

func main() {
	opts := parseFlags(os.Args[1:])
	if err := validateOptions(opts); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(exitInvalid)
	}

	ctx, cancel := context.WithTimeout(context.Background(), opts.timeout)
	defer cancel()

	result := waitForResult(ctx, opts)
	if result.XML != "" {
		fmt.Fprintln(os.Stdout, result.XML)
	}
	if result.Diagnostic != "" {
		fmt.Fprintln(os.Stderr, result.Diagnostic)
	}
	os.Exit(result.ExitCode)
}

func parseFlags(args []string) options {
	var opts options
	fs := flag.NewFlagSet("agent-chat-wait-result", flag.ExitOnError)
	fs.StringVar(&opts.baseURL, "url", "http://localhost:4200", "Agent Chat base URL")
	fs.StringVar(&opts.workspace, "workspace", "", "Agent Chat workspace ID")
	fs.StringVar(&opts.thread, "thread", "", "Agent Chat thread ID")
	fs.StringVar(&opts.run, "run", "", "Agent Chat run ID")
	fs.StringVar(&opts.stage, "stage", "", "expected QRSPI stage/node ID")
	fs.DurationVar(&opts.timeout, "timeout", 10*time.Minute, "maximum wait duration")
	fs.StringVar(&opts.cookie, "cookie", "", "raw Cookie header value")
	fs.StringVar(&opts.cookieJar, "cookie-jar", "", "curl/Netscape cookie jar path")
	fs.StringVar(
		&opts.database,
		"database",
		defaultDatabasePath(),
		"SQLite database path for previous-run status checks; empty disables DB check",
	)
	fs.Var(&opts.headers, "header", "extra HTTP header, may be repeated")
	_ = fs.Parse(args)
	return opts
}

func validateOptions(opts options) error {
	missing := []string{}
	if strings.TrimSpace(opts.baseURL) == "" {
		missing = append(missing, "--url")
	}
	if strings.TrimSpace(opts.workspace) == "" {
		missing = append(missing, "--workspace")
	}
	if strings.TrimSpace(opts.thread) == "" {
		missing = append(missing, "--thread")
	}
	if strings.TrimSpace(opts.run) == "" {
		missing = append(missing, "--run")
	}
	if len(missing) > 0 {
		return fmt.Errorf("missing required flags: %s", strings.Join(missing, ", "))
	}
	if opts.timeout <= 0 {
		return fmt.Errorf("--timeout must be positive")
	}
	if _, err := url.ParseRequestURI(strings.TrimRight(opts.baseURL, "/")); err != nil {
		return fmt.Errorf("invalid --url: %w", err)
	}
	return nil
}

func waitForResult(ctx context.Context, opts options) waitResult {
	fetchFinal := func() (string, error) {
		return fetchCurrentPage(ctx, opts)
	}
	if result := waitFromDatabase(ctx, opts, fetchFinal); result != nil {
		return *result
	}

	streamURL := buildStreamURL(opts)
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, streamURL, nil)
	if err != nil {
		return invalidResult(err.Error())
	}
	req.Header.Set("Accept", "text/event-stream")
	applyAuthHeaders(req, opts)

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		if errors.Is(ctx.Err(), context.DeadlineExceeded) {
			return waitResult{
				ExitCode:   exitTimeout,
				Diagnostic: "timed out waiting for Agent Chat run to stop",
			}
		}
		return invalidResult(fmt.Sprintf("connect to Agent Chat SSE: %v", err))
	}
	defer resp.Body.Close()
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return invalidResult(fmt.Sprintf("SSE request failed: %s", resp.Status))
	}

	return consumeSSE(ctx, resp.Body, opts, fetchFinal)
}

func defaultDatabasePath() string {
	if path := strings.TrimSpace(os.Getenv("DATABASE_PATH")); path != "" {
		return path
	}
	home, err := os.UserHomeDir()
	if err != nil || home == "" {
		return ""
	}
	path := filepath.Join(home, ".local", "state", "cn-agents", "agents.db")
	if _, err := os.Stat(path); err == nil {
		return path
	}
	return ""
}

type dbRunStatus struct {
	Status       string
	ErrorMessage string
}

func waitFromDatabase(
	ctx context.Context,
	opts options,
	fetchFinal func() (string, error),
) *waitResult {
	path := strings.TrimSpace(opts.database)
	if path == "" || strings.TrimSpace(opts.run) == "" {
		return nil
	}
	run, err := lookupRunStatus(ctx, path, opts)
	if err != nil {
		return nil
	}
	switch strings.ToLower(strings.TrimSpace(run.Status)) {
	case "complete":
		result := finalizeStoppedRun("", opts, fetchFinal)
		return &result
	case "failed":
		diagnostic := strings.TrimSpace(run.ErrorMessage)
		if diagnostic == "" {
			diagnostic = "run status is failed"
		}
		result := waitResult{
			ExitCode:   exitFailed,
			Diagnostic: "Agent Chat run failed or errored: " + diagnostic,
		}
		return &result
	default:
		return nil
	}
}

func lookupRunStatus(
	ctx context.Context,
	databasePath string,
	opts options,
) (dbRunStatus, error) {
	db, err := sql.Open("sqlite", databasePath)
	if err != nil {
		return dbRunStatus{}, err
	}
	defer db.Close()

	var status dbRunStatus
	var errorMessage sql.NullString
	err = db.QueryRowContext(
		ctx,
		`select status, error_message
			from agent_runs
			where id = ?
			  and workspace_id = ?
			  and thread_id = ?`,
		opts.run,
		opts.workspace,
		opts.thread,
	).Scan(&status.Status, &errorMessage)
	if err != nil {
		return dbRunStatus{}, err
	}
	if errorMessage.Valid {
		status.ErrorMessage = errorMessage.String
	}
	return status, nil
}

func buildStreamURL(opts options) string {
	base := strings.TrimRight(opts.baseURL, "/")
	return fmt.Sprintf(
		"%s/agent-chat/%s/stream?thread=%s&run=%s&since=%s",
		base,
		url.PathEscape(opts.workspace),
		url.QueryEscape(opts.thread),
		url.QueryEscape(opts.run),
		defaultSince,
	)
}

func applyAuthHeaders(req *http.Request, opts options) {
	for _, header := range opts.headers {
		name, value, ok := strings.Cut(header, ":")
		if ok && strings.TrimSpace(name) != "" {
			req.Header.Add(strings.TrimSpace(name), strings.TrimSpace(value))
		}
	}
	cookies := strings.TrimSpace(opts.cookie)
	if opts.cookieJar != "" {
		jarCookies, err := readNetscapeCookieJar(opts.cookieJar)
		if err == nil && jarCookies != "" {
			if cookies != "" {
				cookies += "; "
			}
			cookies += jarCookies
		}
	}
	if cookies != "" {
		req.Header.Set("Cookie", cookies)
	}
}

func readNetscapeCookieJar(path string) (string, error) {
	file, err := os.Open(path)
	if err != nil {
		return "", err
	}
	defer file.Close()

	pairs := []string{}
	scanner := bufio.NewScanner(file)
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if strings.HasPrefix(line, "#HttpOnly_") {
			line = strings.TrimPrefix(line, "#HttpOnly_")
		}
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		fields := strings.Split(line, "\t")
		if len(fields) < 7 {
			continue
		}
		name := strings.TrimSpace(fields[5])
		value := strings.TrimSpace(fields[6])
		if name != "" {
			pairs = append(pairs, name+"="+value)
		}
	}
	if err := scanner.Err(); err != nil {
		return "", err
	}
	return strings.Join(pairs, "; "), nil
}

func consumeSSE(
	ctx context.Context,
	reader io.Reader,
	opts options,
	fetchFinal func() (string, error),
) waitResult {
	frames := make(chan sseFrame)
	errs := make(chan error, 1)
	go func() {
		defer close(frames)
		errs <- parseSSE(reader, frames)
	}()

	var accumulated strings.Builder
	var stopped bool
	var failed bool
	var failureDiagnostic string

	for {
		select {
		case <-ctx.Done():
			return waitResult{
				ExitCode:   exitTimeout,
				Diagnostic: "timed out waiting for Agent Chat run to stop",
			}
		case frame, ok := <-frames:
			if !ok {
				if err := <-errs; err != nil {
					return invalidResult(fmt.Sprintf("read SSE: %v", err))
				}
				stopped = true
			} else {
				payload := frame.Event + "\n" + frame.Data
				accumulated.WriteString("\n")
				accumulated.WriteString(payload)
				if isFailurePatch(payload, opts.run) {
					failed = true
					failureDiagnostic = compactDiagnostic(payload)
					stopped = true
				} else if isStoppedPatch(payload, opts.run) {
					stopped = true
				}
			}

			if !stopped {
				continue
			}
			if failed {
				return waitResult{
					ExitCode:   exitFailed,
					Diagnostic: "Agent Chat run failed or errored: " + failureDiagnostic,
				}
			}
			return finalizeStoppedRun(accumulated.String(), opts, fetchFinal)
		}
	}
}

func parseSSE(reader io.Reader, out chan<- sseFrame) error {
	scanner := bufio.NewScanner(reader)
	scanner.Buffer(make([]byte, 0, 64*1024), 8*1024*1024)
	var event string
	var data []string
	dispatch := func() {
		if event == "" && len(data) == 0 {
			return
		}
		out <- sseFrame{Event: event, Data: strings.Join(data, "\n")}
		event = ""
		data = nil
	}
	for scanner.Scan() {
		line := strings.TrimSuffix(scanner.Text(), "\r")
		if line == "" {
			dispatch()
			continue
		}
		if strings.HasPrefix(line, ":") {
			continue
		}
		field, value, ok := strings.Cut(line, ":")
		if ok && strings.HasPrefix(value, " ") {
			value = value[1:]
		}
		switch field {
		case "event":
			event = value
		case "data":
			data = append(data, value)
		}
	}
	if err := scanner.Err(); err != nil {
		return err
	}
	dispatch()
	return nil
}

func isStoppedPatch(payload, runID string) bool {
	lower := strings.ToLower(html.UnescapeString(payload))
	if !looksLikeRunStatePatch(lower) {
		return false
	}
	runID = strings.ToLower(strings.TrimSpace(runID))
	if strings.Contains(lower, "no active run selected") ||
		(runID != "" && strings.Contains(payloadAroundRunID(lower, runID), "run completed")) {
		return true
	}
	if strings.Contains(lower, "workspace log") &&
		!strings.Contains(lower, "agent-chat-run-session-panel") &&
		!strings.Contains(lower, "agent-chat-run-header") &&
		!strings.Contains(lower, "workflow_done") {
		return false
	}
	lower = payloadAroundRunID(lower, runID)
	if runID != "" && lower == "" {
		return false
	}
	return containsStatusToken(lower, "complete") ||
		containsStatusToken(lower, "completed") ||
		containsStatusToken(lower, "done") ||
		strings.Contains(lower, "workflow_done")
}

func isFailurePatch(payload, runID string) bool {
	lower := strings.ToLower(html.UnescapeString(payload))
	if !looksLikeRunStatePatch(lower) {
		return false
	}
	runID = strings.ToLower(strings.TrimSpace(runID))
	if runID != "" && strings.Contains(lower, runID) &&
		strings.Contains(payloadAroundRunID(lower, runID), "run failed") {
		return true
	}
	if strings.Contains(lower, "workspace log") &&
		!strings.Contains(lower, "agent-chat-run-session-panel") &&
		!strings.Contains(lower, "agent-chat-run-header") &&
		!strings.Contains(lower, "workflow_error") {
		return false
	}
	lower = payloadAroundRunID(lower, runID)
	if runID != "" && lower == "" {
		return false
	}
	return containsStatusToken(lower, "failed") ||
		containsStatusToken(lower, "error") ||
		strings.Contains(lower, "workflow_error")
}

func payloadAroundRunID(lower, runID string) string {
	if runID == "" {
		return lower
	}
	idx := strings.Index(lower, runID)
	if idx < 0 {
		return ""
	}
	start := idx - 2000
	if start < 0 {
		start = 0
	}
	end := idx + len(runID) + 2000
	if end > len(lower) {
		end = len(lower)
	}
	return lower[start:end]
}

func looksLikeRunStatePatch(lower string) bool {
	return strings.Contains(lower, "agent-chat-run-session-panel") ||
		strings.Contains(lower, "agent-chat-run-header") ||
		strings.Contains(lower, "workspace log") ||
		strings.Contains(lower, "workflow_done") ||
		strings.Contains(lower, "workflow_error")
}

func containsStatusToken(value, token string) bool {
	patterns := []string{
		">" + token + "<",
		"status " + token,
		"status: " + token,
		"\"status\":\"" + token + "\"",
		" " + token + " ",
	}
	for _, pattern := range patterns {
		if strings.Contains(value, pattern) {
			return true
		}
	}
	return false
}

func finalizeStoppedRun(
	accumulated string,
	opts options,
	fetchFinal func() (string, error),
) waitResult {
	xmlText, err := extractAndValidateXML(accumulated, opts.stage)
	if err == nil {
		return waitResult{ExitCode: exitOK, XML: xmlText}
	}

	if fetchFinal != nil {
		finalHTML, fetchErr := fetchFinal()
		if fetchErr == nil && strings.TrimSpace(finalHTML) != "" {
			xmlText, err = extractAndValidateXML(accumulated+"\n"+finalHTML, opts.stage)
			if err == nil {
				return waitResult{ExitCode: exitOK, XML: xmlText}
			}
		} else if fetchErr != nil {
			err = fmt.Errorf("%w; final page fetch failed: %v", err, fetchErr)
		}
	}
	return waitResult{
		ExitCode: exitInvalid,
		Diagnostic: fmt.Sprintf(
			"Agent Chat run stopped without valid QRSPI XML: %v\nLast observed output: %s",
			err,
			compactDiagnostic(accumulated),
		),
	}
}

var qrspiResultPattern = regexp.MustCompile(`(?s)<qrspi-result\b[^>]*>.*?</qrspi-result>`)

func extractAndValidateXML(text, expectedStage string) (string, error) {
	candidates := []string{text, html.UnescapeString(text), renderedHTMLText(text)}
	var lastErr error
	for _, candidate := range candidates {
		matches := qrspiResultPattern.FindAllString(candidate, -1)
		for i := len(matches) - 1; i >= 0; i-- {
			xmlText := strings.TrimSpace(matches[i])
			parser := qrspi.QRSPIXMLParser{}
			_, err := parser.Parse(
				xmlText,
				wruntime.ParseContext{
					ExpectedNodeID: wruntime.NodeID(strings.TrimSpace(expectedStage)),
				},
			)
			if err == nil {
				return xmlText, nil
			}
			lastErr = err
		}
	}
	if lastErr != nil {
		return "", lastErr
	}
	return "", fmt.Errorf("missing <qrspi-result> XML")
}

func renderedHTMLText(text string) string {
	text = regexp.MustCompile(`<[^>]+>`).ReplaceAllString(text, " ")
	text = html.UnescapeString(text)
	text = strings.Join(strings.Fields(text), " ")
	replacements := []struct{ old, new string }{
		{"</ ", "</"},
		{"< ", "<"},
		{" >", ">"},
		{" - ", "-"},
		{"- ", "-"},
		{" -", "-"},
		{"> elements <", "><"},
		{"> elements ", ">"},
		{" elements </", "</"},
		{"> <", "><"},
	}
	for _, replacement := range replacements {
		text = strings.ReplaceAll(text, replacement.old, replacement.new)
	}
	return text
}

func fetchCurrentPage(ctx context.Context, opts options) (string, error) {
	pageURL := fmt.Sprintf(
		"%s/agent-chat/%s/thread/%s?run=%s",
		strings.TrimRight(opts.baseURL, "/"),
		url.PathEscape(opts.workspace),
		url.PathEscape(opts.thread),
		url.QueryEscape(opts.run),
	)
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, pageURL, nil)
	if err != nil {
		return "", err
	}
	applyAuthHeaders(req, opts)
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return "", fmt.Errorf("page request failed: %s", resp.Status)
	}
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return "", err
	}
	return string(body), nil
}

func invalidResult(message string) waitResult {
	return waitResult{ExitCode: exitInvalid, Diagnostic: message}
}

func compactDiagnostic(value string) string {
	value = html.UnescapeString(value)
	value = regexp.MustCompile(`<[^>]+>`).ReplaceAllString(value, " ")
	value = strings.Join(strings.Fields(value), " ")
	const limit = 500
	if len(value) > limit {
		return value[len(value)-limit:]
	}
	return value
}
