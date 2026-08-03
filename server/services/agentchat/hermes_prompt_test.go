package agentchat

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

type crashingHermesPromptGateway struct {
	mode string
}

func (g crashingHermesPromptGateway) DeliverPrompt(
	context.Context,
	HermesPrompt,
) HermesPromptDeliveryObservation {
	if g.mode == "after-started" {
		os.Exit(17)
	}
	return HermesPromptDeliveryObservation{Status: HermesPromptAccepted}
}

func (crashingHermesPromptGateway) DeliverPiCompletion(context.Context, string, string, []byte) error {
	return nil
}

type hermesPromptGatewayFake struct {
	mu          sync.Mutex
	calls       []HermesPrompt
	observation HermesPromptDeliveryObservation
	entered     chan struct{}
	release     chan struct{}
	hadDeadline bool
}

type roundTripFunc func(*http.Request) (*http.Response, error)

func (f roundTripFunc) RoundTrip(req *http.Request) (*http.Response, error) {
	return f(req)
}

type hermesPromptSubprocessResult struct {
	Result HermesPromptResult `json:"result"`
	Error  string             `json:"error,omitempty"`
	Calls  int                `json:"calls"`
}

func (f *hermesPromptGatewayFake) DeliverPrompt(
	ctx context.Context,
	prompt HermesPrompt,
) HermesPromptDeliveryObservation {
	f.mu.Lock()
	f.calls = append(f.calls, prompt)
	_, f.hadDeadline = ctx.Deadline()
	f.mu.Unlock()
	if f.entered != nil {
		select {
		case f.entered <- struct{}{}:
		default:
		}
	}
	if f.release != nil {
		select {
		case <-f.release:
		case <-ctx.Done():
			return HermesPromptDeliveryObservation{Status: HermesPromptUncertain, Detail: ctx.Err().Error()}
		}
	}
	return f.observation
}

func (f *hermesPromptGatewayFake) DeliverPiCompletion(
	context.Context, string, string, []byte,
) error {
	return nil
}

func (f *hermesPromptGatewayFake) callCount() int {
	f.mu.Lock()
	defer f.mu.Unlock()
	return len(f.calls)
}

func newHermesPromptFixture(t *testing.T) (*Service, string, HermesPrompt) {
	t.Helper()
	root := t.TempDir()
	identity := HermesPlanIdentity("owner/plans/alpha")
	planDir := filepath.Join(root, filepath.FromSlash(string(identity)))
	if err := os.MkdirAll(planDir, 0o700); err != nil {
		t.Fatal(err)
	}
	if err := AppendHermesTranscript(planDir, hermesMetadataFixture(identity, "thread_1")); err != nil {
		t.Fatal(err)
	}
	contextPath := filepath.Join(root, "owner", "plans", "alpha", "design.md")
	if err := os.WriteFile(contextPath, []byte("design"), 0o600); err != nil {
		t.Fatal(err)
	}
	return &Service{thoughtsRoot: root}, planDir, HermesPrompt{
		CommandID: "command_1", ThreadID: "thread_1", PlanDir: string(identity),
		ContextPaths: []string{"thoughts/owner/plans/alpha/design.md"}, Prompt: "  exact prompt bytes  ",
	}
}

func TestHermesPromptBindingPreservesExactPromptAndCanonicalContextOrder(t *testing.T) {
	service, _, prompt := newHermesPromptFixture(t)
	second := filepath.Join(service.thoughtsRoot, "owner", "plans", "alpha", "outline.md")
	if err := os.WriteFile(second, []byte("outline"), 0o600); err != nil {
		t.Fatal(err)
	}
	prompt.ContextPaths = []string{
		"thoughts/owner/plans/alpha/./design.md",
		"thoughts/owner/plans/alpha/outline.md",
	}
	validated, firstDigest, err := service.validateHermesPromptCommand(prompt)
	if err != nil {
		t.Fatal(err)
	}
	if validated.Prompt != prompt.Prompt || validated.ContextPaths[0] != "thoughts/owner/plans/alpha/design.md" {
		t.Fatalf("validated prompt = %#v", validated)
	}
	prompt.ContextPaths[0], prompt.ContextPaths[1] = prompt.ContextPaths[1], prompt.ContextPaths[0]
	_, reversedDigest, err := service.validateHermesPromptCommand(prompt)
	if err != nil {
		t.Fatal(err)
	}
	if firstDigest == reversedDigest {
		t.Fatal("ordered context bytes did not affect command binding")
	}
}

func TestSubmitOwnedHermesPromptAuthorizationAppendsNothingAndCallsNothing(t *testing.T) {
	service, planDir, prompt := newHermesPromptFixture(t)
	gateway := &hermesPromptGatewayFake{observation: HermesPromptDeliveryObservation{Status: HermesPromptAccepted}}
	service.hermesGateway = gateway
	path, err := HermesTranscriptPath(planDir, prompt.ThreadID)
	if err != nil {
		t.Fatal(err)
	}
	before, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	_, err = service.SubmitOwnedHermesPrompt(t.Context(), "reader@example.com", prompt)
	if !errors.Is(err, ErrHermesPromptUnauthorized) {
		t.Fatalf("error = %v", err)
	}
	after, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if string(after) != string(before) || gateway.callCount() != 0 {
		t.Fatalf("unauthorized mutation/calls = %t/%d", string(after) != string(before), gateway.callCount())
	}
}

func TestSubmitOwnedHermesPromptConcurrentLoserCallbackAndReplay(t *testing.T) {
	service, planDir, prompt := newHermesPromptFixture(t)
	gateway := &hermesPromptGatewayFake{
		observation: HermesPromptDeliveryObservation{Status: HermesPromptAccepted},
		entered:     make(chan struct{}, 1), release: make(chan struct{}),
	}
	service.hermesGateway = gateway
	resultCh := make(chan HermesPromptResult, 1)
	errCh := make(chan error, 1)
	go func() {
		result, err := service.SubmitOwnedHermesPrompt(context.Background(), "OWNER@example.com", prompt)
		resultCh <- result
		errCh <- err
	}()
	select {
	case <-gateway.entered:
	case <-time.After(time.Second):
		t.Fatal("gateway was not entered")
	}
	loser, err := service.SubmitOwnedHermesPrompt(t.Context(), "owner@example.com", prompt)
	if err != nil || !loser.InProgress || gateway.callCount() != 1 {
		t.Fatalf("loser/calls = %#v, %v, %d", loser, err, gateway.callCount())
	}
	conflicting := prompt
	conflicting.Prompt = "different concurrent bytes"
	if _, err := service.SubmitOwnedHermesPrompt(t.Context(), "owner@example.com", conflicting); !errors.Is(err, ErrHermesPromptConflict) {
		t.Fatalf("concurrent conflict error = %v", err)
	}
	if err := service.AppendHermesTranscript(t.Context(), HermesCallbackEvent{
		PlanDir: prompt.PlanDir,
		HermesTranscriptEvent: HermesTranscriptEvent{
			ID: "callback_while_gateway_blocked", Type: "lifecycle", ThreadID: prompt.ThreadID,
			Content: "callback progressed",
		},
	}); err != nil {
		t.Fatalf("callback blocked by gateway-held transcript lock: %v", err)
	}
	events, err := readHermesTranscriptContext(t.Context(), planDir, HermesPlanIdentity(prompt.PlanDir), prompt.ThreadID)
	if err != nil {
		t.Fatal(err)
	}
	for _, event := range events {
		if event.Type == "prompt_delivery" {
			t.Fatal("concurrent loser appended a terminal event")
		}
	}
	close(gateway.release)
	if err := <-errCh; err != nil {
		t.Fatal(err)
	}
	if result := <-resultCh; result.Status != HermesPromptAccepted {
		t.Fatalf("owner result = %#v", result)
	}
	replayed, err := service.SubmitOwnedHermesPrompt(t.Context(), "owner@example.com", prompt)
	if err != nil || replayed.Status != HermesPromptAccepted || gateway.callCount() != 1 {
		t.Fatalf("replay/calls = %#v, %v, %d", replayed, err, gateway.callCount())
	}
	gateway.mu.Lock()
	sent := gateway.calls[0]
	gateway.mu.Unlock()
	if sent.CommandID != prompt.CommandID || sent.Prompt != prompt.Prompt ||
		sent.ConversationReference == "" || sent.ContextPaths[0] != prompt.ContextPaths[0] ||
		!gateway.hadDeadline {
		t.Fatalf("gateway prompt = %#v", sent)
	}
}

func TestSubmitOwnedHermesPromptFullBoundaryIsExclusiveAcrossProcesses(t *testing.T) {
	if mode := os.Getenv("VAMOS_HERMES_PROMPT_BOUNDARY_HELPER"); mode != "" {
		service := &Service{thoughtsRoot: os.Getenv("VAMOS_HERMES_PROMPT_BOUNDARY_ROOT")}
		gateway := &hermesPromptGatewayFake{
			observation: HermesPromptDeliveryObservation{Status: HermesPromptAccepted},
		}
		if mode == "owner" {
			gateway.entered = make(chan struct{}, 1)
			gateway.release = make(chan struct{})
		}
		service.hermesGateway = gateway
		prompt := HermesPrompt{
			CommandID: "boundary_command", ThreadID: "thread_1",
			PlanDir: "owner/plans/alpha", Prompt: "same command bytes",
		}
		if mode == "conflict" {
			prompt.Prompt = "different command bytes"
		}
		resultCh := make(chan HermesPromptResult, 1)
		errCh := make(chan error, 1)
		go func() {
			result, err := service.SubmitOwnedHermesPrompt(context.Background(), "owner@example.com", prompt)
			resultCh <- result
			errCh <- err
		}()
		if mode == "owner" {
			<-gateway.entered
			if _, err := os.Stdout.WriteString("entered\n"); err != nil {
				os.Exit(2)
			}
			if _, err := bufio.NewReader(os.Stdin).ReadString('\n'); err != nil {
				os.Exit(3)
			}
			close(gateway.release)
		}
		result, err := <-resultCh, <-errCh
		outcome := hermesPromptSubprocessResult{Result: result, Calls: gateway.callCount()}
		if err != nil {
			outcome.Error = err.Error()
		}
		if err := json.NewEncoder(os.Stdout).Encode(outcome); err != nil {
			os.Exit(4)
		}
		return
	}

	service, planDir, _ := newHermesPromptFixture(t)
	owner := exec.Command(os.Args[0], "-test.run=^TestSubmitOwnedHermesPromptFullBoundaryIsExclusiveAcrossProcesses$")
	owner.Env = append(os.Environ(),
		"VAMOS_HERMES_PROMPT_BOUNDARY_HELPER=owner",
		"VAMOS_HERMES_PROMPT_BOUNDARY_ROOT="+service.thoughtsRoot,
	)
	ownerStdout, err := owner.StdoutPipe()
	if err != nil {
		t.Fatal(err)
	}
	ownerStdin, err := owner.StdinPipe()
	if err != nil {
		t.Fatal(err)
	}
	if err := owner.Start(); err != nil {
		t.Fatal(err)
	}
	defer func() { _ = owner.Process.Kill() }()
	ownerOutput := bufio.NewReader(ownerStdout)
	if line, err := ownerOutput.ReadString('\n'); err != nil || line != "entered\n" {
		t.Fatalf("owner handshake = %q, %v", line, err)
	}

	assertPendingTranscript := func() {
		t.Helper()
		events, err := readHermesTranscriptContext(
			t.Context(), planDir, "owner/plans/alpha", "thread_1",
		)
		if err != nil {
			t.Fatal(err)
		}
		var requested, started, terminal int
		for _, event := range events {
			if event.CommandID != "boundary_command" {
				continue
			}
			switch event.Type {
			case "prompt_requested":
				requested++
			case "prompt_delivery_started":
				started++
			case "prompt_delivery":
				terminal++
			}
		}
		if requested != 1 || started != 1 || terminal != 0 {
			t.Fatalf("pending command events = requested %d, started %d, terminal %d", requested, started, terminal)
		}
	}
	assertPendingTranscript()

	runLoser := func(mode string) hermesPromptSubprocessResult {
		t.Helper()
		cmd := exec.Command(os.Args[0], "-test.run=^TestSubmitOwnedHermesPromptFullBoundaryIsExclusiveAcrossProcesses$")
		cmd.Env = append(os.Environ(),
			"VAMOS_HERMES_PROMPT_BOUNDARY_HELPER="+mode,
			"VAMOS_HERMES_PROMPT_BOUNDARY_ROOT="+service.thoughtsRoot,
		)
		output, err := cmd.Output()
		if err != nil {
			t.Fatalf("%s subprocess: %v", mode, err)
		}
		var outcome hermesPromptSubprocessResult
		if err := json.Unmarshal(bytes.SplitN(output, []byte("\n"), 2)[0], &outcome); err != nil {
			t.Fatalf("decode %s subprocess %q: %v", mode, output, err)
		}
		return outcome
	}
	identical := runLoser("identical")
	if identical.Error != "" || !identical.Result.InProgress || identical.Calls != 0 {
		t.Fatalf("identical loser = %#v", identical)
	}
	assertPendingTranscript()
	conflict := runLoser("conflict")
	if conflict.Error != ErrHermesPromptConflict.Error() || conflict.Calls != 0 {
		t.Fatalf("conflicting loser = %#v", conflict)
	}
	assertPendingTranscript()

	if _, err := ownerStdin.Write([]byte("release\n")); err != nil {
		t.Fatal(err)
	}
	var ownerResult hermesPromptSubprocessResult
	if err := json.NewDecoder(ownerOutput).Decode(&ownerResult); err != nil {
		t.Fatal(err)
	}
	if err := owner.Wait(); err != nil {
		t.Fatal(err)
	}
	if ownerResult.Error != "" || ownerResult.Result.Status != HermesPromptAccepted || ownerResult.Calls != 1 {
		t.Fatalf("owner result = %#v", ownerResult)
	}
	events, err := readHermesTranscriptContext(t.Context(), planDir, "owner/plans/alpha", "thread_1")
	if err != nil {
		t.Fatal(err)
	}
	terminals := 0
	for _, event := range events {
		if event.CommandID == "boundary_command" && event.Type == "prompt_delivery" {
			terminals++
		}
	}
	if terminals != 1 {
		t.Fatalf("terminal count after owner release = %d", terminals)
	}
}

func TestSubmitOwnedHermesPromptProcessCrashBecomesUncertainWithoutResend(t *testing.T) {
	if mode := os.Getenv("VAMOS_HERMES_PROMPT_CRASH_MODE"); mode != "" {
		service := &Service{thoughtsRoot: os.Getenv("VAMOS_HERMES_PROMPT_CRASH_ROOT")}
		service.hermesGateway = crashingHermesPromptGateway{mode: mode}
		if mode == "after-response" {
			hermesPromptAfterGatewayHook = func() { os.Exit(18) }
		}
		_, _ = service.SubmitOwnedHermesPrompt(context.Background(), "owner@example.com", HermesPrompt{
			CommandID: "crash_command", ThreadID: "thread_1", PlanDir: "owner/plans/alpha", Prompt: "crash prompt",
		})
		os.Exit(19)
	}
	for _, mode := range []string{"after-started", "after-response"} {
		t.Run(mode, func(t *testing.T) {
			service, _, prompt := newHermesPromptFixture(t)
			prompt.CommandID = "crash_command"
			prompt.Prompt = "crash prompt"
			prompt.ContextPaths = nil
			cmd := exec.Command(os.Args[0], "-test.run=^TestSubmitOwnedHermesPromptProcessCrashBecomesUncertainWithoutResend$")
			cmd.Env = append(os.Environ(),
				"VAMOS_HERMES_PROMPT_CRASH_MODE="+mode,
				"VAMOS_HERMES_PROMPT_CRASH_ROOT="+service.thoughtsRoot,
			)
			err := cmd.Run()
			exitErr, ok := err.(*exec.ExitError)
			if !ok || (exitErr.ExitCode() != 17 && exitErr.ExitCode() != 18) {
				t.Fatalf("crash subprocess = %v", err)
			}
			gateway := &hermesPromptGatewayFake{observation: HermesPromptDeliveryObservation{Status: HermesPromptAccepted}}
			service.hermesGateway = gateway
			result, err := service.SubmitOwnedHermesPrompt(t.Context(), "owner@example.com", prompt)
			if err != nil || result.Status != HermesPromptUncertain || gateway.callCount() != 0 {
				t.Fatalf("crash recovery = %#v, %v, calls %d", result, err, gateway.callCount())
			}
		})
	}
}

func TestSubmitOwnedHermesPromptCancellationBecomesDurableUncertainWithoutResend(t *testing.T) {
	service, _, prompt := newHermesPromptFixture(t)
	gateway := &hermesPromptGatewayFake{
		observation: HermesPromptDeliveryObservation{Status: HermesPromptAccepted},
		release:     make(chan struct{}),
	}
	service.hermesGateway = gateway
	ctx, cancel := context.WithCancel(t.Context())
	cancel()
	result, err := service.SubmitOwnedHermesPrompt(ctx, "owner@example.com", prompt)
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("cancellation before reservation should append nothing: %#v/%v", result, err)
	}

	ctx, cancel = context.WithTimeout(t.Context(), 20*time.Millisecond)
	defer cancel()
	result, err = service.SubmitOwnedHermesPrompt(ctx, "owner@example.com", prompt)
	if err != nil || result.Status != HermesPromptUncertain || gateway.callCount() != 1 {
		t.Fatalf("canceled delivery = %#v, %v, calls %d", result, err, gateway.callCount())
	}
	result, err = service.SubmitOwnedHermesPrompt(t.Context(), "owner@example.com", prompt)
	if err != nil || result.Status != HermesPromptUncertain || gateway.callCount() != 1 {
		t.Fatalf("canceled replay = %#v, %v, calls %d", result, err, gateway.callCount())
	}
	commandLocks.mu.Lock()
	remainingCommandLocks := len(commandLocks.entries)
	commandLocks.mu.Unlock()
	if remainingCommandLocks != 0 {
		t.Fatalf("command lock registry retained %d entries", remainingCommandLocks)
	}
}

func TestSubmitOwnedHermesPromptConflictAndIncompleteRecoveryNeverResend(t *testing.T) {
	service, planDir, prompt := newHermesPromptFixture(t)
	gateway := &hermesPromptGatewayFake{observation: HermesPromptDeliveryObservation{Status: HermesPromptAccepted}}
	service.hermesGateway = gateway
	validated, digest, err := service.validateHermesPromptCommand(prompt)
	if err != nil {
		t.Fatal(err)
	}
	path, err := HermesTranscriptPath(planDir, prompt.ThreadID)
	if err != nil {
		t.Fatal(err)
	}
	for _, event := range []HermesTranscriptEvent{
		{ID: hermesPromptEventID("request", prompt.CommandID), Type: "prompt_requested", ThreadID: prompt.ThreadID, PlanDir: HermesPlanIdentity(prompt.PlanDir), CommandID: prompt.CommandID, CommandDigest: digest, Content: prompt.Prompt, ContextPaths: validated.ContextPaths},
		{ID: hermesPromptEventID("started", prompt.CommandID), Type: "prompt_delivery_started", ThreadID: prompt.ThreadID, PlanDir: HermesPlanIdentity(prompt.PlanDir), CommandID: prompt.CommandID, CommandDigest: digest},
	} {
		if err := AppendHermesTranscript(planDir, event); err != nil {
			t.Fatal(err)
		}
	}
	result, err := service.SubmitOwnedHermesPrompt(t.Context(), "owner@example.com", prompt)
	if err != nil || result.Status != HermesPromptUncertain || gateway.callCount() != 0 {
		t.Fatalf("recovery/calls = %#v, %v, %d", result, err, gateway.callCount())
	}
	result, err = service.SubmitOwnedHermesPrompt(t.Context(), "owner@example.com", prompt)
	if err != nil || result.Status != HermesPromptUncertain || gateway.callCount() != 0 {
		t.Fatalf("terminal replay/calls = %#v, %v, %d", result, err, gateway.callCount())
	}
	conflict := prompt
	conflict.Prompt = "different"
	if _, err := service.SubmitOwnedHermesPrompt(t.Context(), "owner@example.com", conflict); !errors.Is(err, ErrHermesPromptConflict) {
		t.Fatalf("conflict error = %v", err)
	}
	events, err := readHermesTranscriptContext(t.Context(), planDir, HermesPlanIdentity(prompt.PlanDir), prompt.ThreadID)
	if err != nil {
		t.Fatal(err)
	}
	terminals := 0
	for _, event := range events {
		if event.Type == "prompt_delivery" && event.CommandID == prompt.CommandID {
			terminals++
		}
	}
	if terminals != 1 {
		t.Fatalf("terminal count = %d; transcript = %s", terminals, path)
	}
}

func TestHTTPHermesGatewayPromptEmitsExactFiniteNonRetryingRequest(t *testing.T) {
	prompt := HermesPrompt{
		CommandID: "command_1", ThreadID: "thread_1", OwnerEmail: "owner@example.com",
		PlanDir: "owner/plans/alpha", ConversationReference: "vhc1_reference",
		ContextPaths: []string{"thoughts/owner/plans/alpha/design.md"}, Prompt: "exact prompt",
	}
	var calls atomic.Int32
	transport := roundTripFunc(func(req *http.Request) (*http.Response, error) {
		calls.Add(1)
		if req.Method != http.MethodPost || req.URL.String() != "https://gateway.example/vamos/prompts" {
			t.Fatalf("request target = %s %s", req.Method, req.URL.String())
		}
		if req.Header.Get("Authorization") != "Bearer ingress-secret" || req.Header.Get("Content-Type") != "application/json" {
			t.Fatalf("request headers = %#v", req.Header)
		}
		deadline, ok := req.Context().Deadline()
		if !ok || time.Until(deadline) <= 0 || time.Until(deadline) > time.Second {
			t.Fatalf("request deadline = %v, %v", deadline, ok)
		}
		body, err := io.ReadAll(req.Body)
		if err != nil {
			t.Fatal(err)
		}
		const want = `{"command_id":"command_1","thread_id":"thread_1","owner_email":"owner@example.com","plan_dir":"owner/plans/alpha","conversation_reference":"vhc1_reference","context_paths":["thoughts/owner/plans/alpha/design.md"],"prompt":"exact prompt"}`
		if string(body) != want {
			t.Fatalf("request body = %s", body)
		}
		return &http.Response{StatusCode: http.StatusAccepted, Status: "202 Accepted", Body: io.NopCloser(strings.NewReader("")), Header: make(http.Header)}, nil
	})
	client := httpHermesGatewayClient{
		url: "https://gateway.example/", token: "ingress-secret",
		client: &http.Client{Transport: transport, Timeout: time.Second},
	}
	ctx, cancel := context.WithTimeout(t.Context(), time.Second)
	defer cancel()
	observation := client.DeliverPrompt(ctx, prompt)
	if observation.Status != HermesPromptAccepted || calls.Load() != 1 {
		t.Fatalf("observation/calls = %#v/%d", observation, calls.Load())
	}
}

func TestHTTPHermesGatewayPromptClassifiesEveryDeliveryBoundary(t *testing.T) {
	response := func(status int) roundTripFunc {
		return func(*http.Request) (*http.Response, error) {
			return &http.Response{
				StatusCode: status, Status: http.StatusText(status),
				Body: io.NopCloser(strings.NewReader("")), Header: make(http.Header),
			}, nil
		}
	}
	for _, test := range []struct {
		name      string
		transport roundTripFunc
		cancel    bool
		want      HermesPromptDeliveryStatus
	}{
		{name: "accepted", transport: response(http.StatusAccepted), want: HermesPromptAccepted},
		{name: "definitive 4xx", transport: response(http.StatusUnprocessableEntity), want: HermesPromptRejected},
		{name: "indeterminate 5xx", transport: response(http.StatusInternalServerError), want: HermesPromptUncertain},
		{name: "connection reset", transport: func(*http.Request) (*http.Response, error) { return nil, io.ErrUnexpectedEOF }, want: HermesPromptUncertain},
		{name: "timeout", transport: func(req *http.Request) (*http.Response, error) {
			<-req.Context().Done()
			return nil, req.Context().Err()
		}, want: HermesPromptUncertain},
		{name: "cancellation", transport: func(req *http.Request) (*http.Response, error) {
			<-req.Context().Done()
			return nil, req.Context().Err()
		}, cancel: true, want: HermesPromptUncertain},
	} {
		t.Run(test.name, func(t *testing.T) {
			var calls atomic.Int32
			client := httpHermesGatewayClient{url: "https://gateway.example", client: &http.Client{
				Transport: roundTripFunc(func(req *http.Request) (*http.Response, error) {
					calls.Add(1)
					return test.transport(req)
				}),
			}}
			ctx, cancel := context.WithTimeout(t.Context(), 20*time.Millisecond)
			if test.cancel {
				cancel()
			} else {
				defer cancel()
			}
			observation := client.DeliverPrompt(ctx, HermesPrompt{CommandID: "command_1"})
			if observation.Status != test.want || calls.Load() != 1 {
				t.Fatalf("observation/calls = %#v/%d", observation, calls.Load())
			}
		})
	}

	var calls atomic.Int32
	client := httpHermesGatewayClient{
		url: "://invalid",
		client: &http.Client{Transport: roundTripFunc(func(*http.Request) (*http.Response, error) {
			calls.Add(1)
			return nil, errors.New("must not be reached")
		})},
	}
	observation := client.DeliverPrompt(t.Context(), HermesPrompt{CommandID: "command_1"})
	if observation.Status != HermesPromptFailed || calls.Load() != 0 {
		t.Fatalf("pre-send observation/calls = %#v/%d", observation, calls.Load())
	}
}

func TestHermesPromptTerminalRaceCannotCreateContradictoryObservations(t *testing.T) {
	service, planDir, prompt := newHermesPromptFixture(t)
	validated, digest, err := service.validateHermesPromptCommand(prompt)
	if err != nil {
		t.Fatal(err)
	}
	if _, send, err := reserveHermesPromptCommand(
		t.Context(), planDir, HermesPlanIdentity(prompt.PlanDir), validated, digest,
	); err != nil || !send {
		t.Fatalf("reserve = %t, %v", send, err)
	}
	observations := []HermesPromptDeliveryObservation{
		{Status: HermesPromptAccepted},
		{Status: HermesPromptRejected},
	}
	errs := make(chan error, len(observations))
	for _, observation := range observations {
		observation := observation
		go func() {
			_, err := finishHermesPromptCommand(
				context.Background(), planDir, HermesPlanIdentity(prompt.PlanDir), validated, digest, observation,
			)
			errs <- err
		}()
	}
	conflicts := 0
	for range observations {
		if err := <-errs; errors.Is(err, ErrHermesPromptConflict) {
			conflicts++
		} else if err != nil {
			t.Fatal(err)
		}
	}
	if conflicts != 1 {
		t.Fatalf("terminal conflict count = %d", conflicts)
	}
	events, err := readHermesTranscriptContext(t.Context(), planDir, HermesPlanIdentity(prompt.PlanDir), prompt.ThreadID)
	if err != nil {
		t.Fatal(err)
	}
	terminals := 0
	for _, event := range events {
		if event.Type == "prompt_delivery" {
			terminals++
		}
	}
	if terminals != 1 {
		t.Fatalf("terminal count = %d", terminals)
	}
}

func TestSubmitOwnedHermesPromptTruthfulTerminalClassifications(t *testing.T) {
	for _, test := range []struct {
		name       string
		gateway    HermesGatewayClient
		want       HermesPromptDeliveryStatus
		wantReason HermesPromptDeliveryReason
	}{
		{name: "missing gateway", gateway: nil, want: HermesPromptFailed, wantReason: HermesPromptGatewayUnavailable},
		{name: "rejected", gateway: &hermesPromptGatewayFake{observation: HermesPromptDeliveryObservation{Status: HermesPromptRejected}}, want: HermesPromptRejected},
		{name: "uncertain", gateway: &hermesPromptGatewayFake{observation: HermesPromptDeliveryObservation{Status: HermesPromptUncertain}}, want: HermesPromptUncertain},
		{name: "accepted", gateway: &hermesPromptGatewayFake{observation: HermesPromptDeliveryObservation{Status: HermesPromptAccepted}}, want: HermesPromptAccepted},
	} {
		t.Run(test.name, func(t *testing.T) {
			service, _, prompt := newHermesPromptFixture(t)
			service.hermesGateway = test.gateway
			result, err := service.SubmitOwnedHermesPrompt(t.Context(), "owner@example.com", prompt)
			if err != nil || result.Status != test.want || result.Reason != test.wantReason {
				t.Fatalf("result/error = %#v/%v", result, err)
			}
			replayed, err := service.SubmitOwnedHermesPrompt(t.Context(), "owner@example.com", prompt)
			if err != nil || replayed.Status != test.want || replayed.Reason != test.wantReason {
				t.Fatalf("replayed result/error = %#v/%v", replayed, err)
			}
		})
	}
}

func TestSubmitOwnedHermesPromptRejectsHostContextAndSeparatesEqualIDsAcrossPlans(t *testing.T) {
	service, _, prompt := newHermesPromptFixture(t)
	prompt.ContextPaths = []string{"/etc/passwd"}
	if _, err := service.SubmitOwnedHermesPrompt(t.Context(), "owner@example.com", prompt); err == nil {
		t.Fatal("absolute context path was accepted")
	}
	alphaPlan := filepath.Join(service.thoughtsRoot, "owner", "plans", "alpha")
	events, err := readHermesTranscriptContext(t.Context(), alphaPlan, "owner/plans/alpha", prompt.ThreadID)
	if err != nil {
		t.Fatal(err)
	}
	if len(events) != 1 {
		t.Fatalf("invalid context appended events: %#v", events)
	}
	root := service.thoughtsRoot
	beta := filepath.Join(root, "owner", "plans", "beta")
	if err := os.MkdirAll(beta, 0o700); err != nil {
		t.Fatal(err)
	}
	if err := AppendHermesTranscript(beta, hermesMetadataFixture("owner/plans/beta", "thread_1")); err != nil {
		t.Fatal(err)
	}
	alphaRef, _ := HermesConversationReference("owner/plans/alpha", "thread_1")
	betaRef, _ := HermesConversationReference("owner/plans/beta", "thread_1")
	if alphaRef == betaRef {
		t.Fatal("equal thread IDs shared a conversation reference")
	}
	alphaLock, err := tryAcquireHermesCommandLock(t.Context(), filepath.Join(root, "owner", "plans", "alpha"), "thread_1", "same_command")
	if err != nil {
		t.Fatal(err)
	}
	defer alphaLock.Close()
	betaLock, err := tryAcquireHermesCommandLock(t.Context(), beta, "thread_1", "same_command")
	if err != nil {
		t.Fatalf("different plans shared a command lock: %v", err)
	}
	_ = betaLock.Close()
}

func TestHermesCommandLockIsNonblockingAcrossProcesses(t *testing.T) {
	if os.Getenv("VAMOS_HERMES_COMMAND_LOCK_HELPER") == "1" {
		planDir := os.Getenv("VAMOS_HERMES_COMMAND_LOCK_PLAN")
		lock, err := tryAcquireHermesCommandLock(context.Background(), planDir, "thread_1", "command_1")
		if err != nil {
			os.Exit(2)
		}
		if _, err := os.Stdout.WriteString("ready\n"); err != nil {
			os.Exit(3)
		}
		if _, err := bufio.NewReader(os.Stdin).ReadString('\n'); err != nil {
			os.Exit(4)
		}
		_ = lock.Close()
		return
	}
	planDir := filepath.Join(t.TempDir(), "plan")
	if err := os.MkdirAll(planDir, 0o700); err != nil {
		t.Fatal(err)
	}
	cmd := exec.Command(os.Args[0], "-test.run=^TestHermesCommandLockIsNonblockingAcrossProcesses$")
	cmd.Env = append(os.Environ(),
		"VAMOS_HERMES_COMMAND_LOCK_HELPER=1",
		"VAMOS_HERMES_COMMAND_LOCK_PLAN="+planDir,
	)
	stdout, err := cmd.StdoutPipe()
	if err != nil {
		t.Fatal(err)
	}
	stdin, err := cmd.StdinPipe()
	if err != nil {
		t.Fatal(err)
	}
	if err := cmd.Start(); err != nil {
		t.Fatal(err)
	}
	defer func() { _ = cmd.Process.Kill() }()
	if line, err := bufio.NewReader(stdout).ReadString('\n'); err != nil || line != "ready\n" {
		t.Fatalf("subprocess handshake = %q, %v", line, err)
	}
	started := time.Now()
	_, err = tryAcquireHermesCommandLock(t.Context(), planDir, "thread_1", "command_1")
	if !errors.Is(err, ErrHermesPromptInProgress) || time.Since(started) > 100*time.Millisecond {
		t.Fatalf("nonblocking lock error/duration = %v/%s", err, time.Since(started))
	}
	commandLocks.mu.Lock()
	retained := len(commandLocks.entries)
	commandLocks.mu.Unlock()
	if retained != 0 {
		t.Fatalf("contended command lock retained %d local entries", retained)
	}
	if _, err := stdin.Write([]byte("release\n")); err != nil {
		t.Fatal(err)
	}
	if err := cmd.Wait(); err != nil {
		t.Fatal(err)
	}
}

func TestHermesPromptLockOrderAndCallbacksNeverTakeCommandOwnership(t *testing.T) {
	service, _, prompt := newHermesPromptFixture(t)
	gateway := &hermesPromptGatewayFake{observation: HermesPromptDeliveryObservation{Status: HermesPromptAccepted}}
	service.hermesGateway = gateway
	var mu sync.Mutex
	order := make([]string, 0, 3)
	localHeldBeforeKernel := false
	hermesCommandBeforeKernelLockHook = func(lock *sync.Mutex) {
		localHeldBeforeKernel = !lock.TryLock()
		if !localHeldBeforeKernel {
			lock.Unlock()
		}
	}
	hermesCommandLockAcquiredHook = func(string, string, string) {
		mu.Lock()
		order = append(order, "command")
		mu.Unlock()
	}
	hermesTranscriptLockAcquiredHook = func(string, string, bool) {
		mu.Lock()
		order = append(order, "transcript")
		mu.Unlock()
	}
	defer func() {
		hermesCommandBeforeKernelLockHook = nil
		hermesCommandLockAcquiredHook = nil
		hermesTranscriptLockAcquiredHook = nil
	}()
	if _, err := service.SubmitOwnedHermesPrompt(t.Context(), "owner@example.com", prompt); err != nil {
		t.Fatal(err)
	}
	if !localHeldBeforeKernel {
		t.Fatal("command kernel lock was attempted before process-local ownership")
	}
	mu.Lock()
	if len(order) < 2 || order[0] != "transcript" {
		mu.Unlock()
		t.Fatalf("authorization did not read transcript first: %v", order)
	}
	commandIndex := -1
	for i, item := range order {
		if item == "command" {
			commandIndex = i
			break
		}
	}
	if commandIndex < 0 || commandIndex+1 >= len(order) || order[commandIndex+1] != "transcript" {
		mu.Unlock()
		t.Fatalf("nested command-to-transcript order = %v", order)
	}
	order = nil
	mu.Unlock()
	if err := service.AppendHermesTranscript(t.Context(), HermesCallbackEvent{
		PlanDir:               prompt.PlanDir,
		HermesTranscriptEvent: HermesTranscriptEvent{ID: "callback_only", Type: "final", ThreadID: prompt.ThreadID, Content: "done"},
	}); err != nil {
		t.Fatal(err)
	}
	mu.Lock()
	defer mu.Unlock()
	if strings.Contains(strings.Join(order, ","), "command") {
		t.Fatalf("callback acquired command ownership: %v", order)
	}
}
