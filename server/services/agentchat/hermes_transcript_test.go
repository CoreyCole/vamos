package agentchat

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"sync"
	"testing"
	"time"

	"golang.org/x/sys/unix"
)

func hermesMetadataFixture(plan HermesPlanIdentity, thread string) HermesTranscriptEvent {
	return HermesTranscriptEvent{
		ID: "metadata_" + thread, At: time.Unix(1, 0).UTC(), Type: "thread_metadata",
		ThreadID: thread, PlanDir: plan, CreatorEmail: "Creator@Example.com",
		PromptAuthority: &HermesPromptAuthority{
			PrincipalType: "authenticated_email", PrincipalValue: "owner@example.com",
		},
		Title: "Shared thread",
	}
}

func TestHermesTranscriptHeaderFirstCreateReadAndImmutableMetadata(t *testing.T) {
	root := t.TempDir()
	planDir := filepath.Join(root, "owner", "plans", "alpha")
	if err := os.MkdirAll(planDir, 0o700); err != nil {
		t.Fatal(err)
	}
	metadata := hermesMetadataFixture("owner/plans/alpha", "thread_1")
	if err := AppendHermesTranscript(planDir, HermesTranscriptEvent{
		ID: "early", At: time.Unix(2, 0).UTC(), Type: "user", ThreadID: "thread_1",
		PlanDir: "owner/plans/alpha", Content: "early",
	}); err == nil {
		t.Fatal("event before metadata was accepted")
	}
	if err := AppendHermesTranscript(planDir, metadata); err != nil {
		t.Fatal(err)
	}
	if err := AppendHermesTranscript(planDir, metadata); err != nil {
		t.Fatalf("identical metadata replay: %v", err)
	}
	changed := metadata
	changed.Title = "changed"
	if err := AppendHermesTranscript(planDir, changed); err == nil {
		t.Fatal("changed metadata was accepted")
	}
	events, err := readHermesTranscriptContext(
		context.Background(), planDir, "owner/plans/alpha", "thread_1",
	)
	if err != nil {
		t.Fatal(err)
	}
	if len(events) != 1 || events[0].PromptAuthority.PrincipalValue != "owner@example.com" {
		t.Fatalf("events = %#v", events)
	}
}

func TestHermesTranscriptRejectsSymlinkFileEscape(t *testing.T) {
	planDir := filepath.Join(t.TempDir(), "plan")
	dir := filepath.Join(planDir, ".vamos", "sessions", "hermes")
	if err := os.MkdirAll(dir, 0o700); err != nil {
		t.Fatal(err)
	}
	outside := filepath.Join(t.TempDir(), "outside.jsonl")
	if err := os.WriteFile(outside, nil, 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(outside, filepath.Join(dir, "thread_1.jsonl")); err != nil {
		t.Fatal(err)
	}
	if err := AppendHermesTranscript(
		planDir, hermesMetadataFixture("owner/plans/alpha", "thread_1"),
	); err == nil {
		t.Fatal("transcript symlink escape was accepted")
	}
}

func TestHermesTranscriptRejectsIdentityMismatchAndConflictingDuplicate(t *testing.T) {
	planDir := filepath.Join(t.TempDir(), "plan")
	if err := os.MkdirAll(planDir, 0o700); err != nil {
		t.Fatal(err)
	}
	metadata := hermesMetadataFixture("owner/plans/alpha", "thread_1")
	if err := AppendHermesTranscript(planDir, metadata); err != nil {
		t.Fatal(err)
	}
	event := HermesTranscriptEvent{
		ID: "event_1", At: time.Unix(2, 0).UTC(), Type: "user", ThreadID: "thread_1",
		PlanDir: "owner/plans/alpha", Content: "one",
	}
	if err := AppendHermesTranscript(planDir, event); err != nil {
		t.Fatal(err)
	}
	if err := AppendHermesTranscript(planDir, event); err != nil {
		t.Fatal(err)
	}
	event.Content = "two"
	if err := AppendHermesTranscript(planDir, event); err == nil {
		t.Fatal("conflicting duplicate was accepted")
	}
	if _, err := readHermesTranscriptContext(
		context.Background(), planDir, "owner/plans/beta", "thread_1",
	); err == nil {
		t.Fatal("requested plan mismatch was accepted")
	}
}

func TestHermesTranscriptZeroTimeReplayIsIdempotentAndPayloadConflictFails(t *testing.T) {
	planDir := filepath.Join(t.TempDir(), "plan")
	if err := os.MkdirAll(planDir, 0o700); err != nil {
		t.Fatal(err)
	}
	if err := AppendHermesTranscript(
		planDir, hermesMetadataFixture("owner/plans/alpha", "thread_1"),
	); err != nil {
		t.Fatal(err)
	}
	event := HermesTranscriptEvent{
		ID: "callback_1", Type: "final", ThreadID: "thread_1",
		PlanDir: "owner/plans/alpha", Content: "done",
	}
	if err := AppendHermesTranscript(planDir, event); err != nil {
		t.Fatal(err)
	}
	if err := AppendHermesTranscript(planDir, event); err != nil {
		t.Fatalf("zero-time replay: %v", err)
	}
	event.Content = "different"
	if err := AppendHermesTranscript(planDir, event); err == nil {
		t.Fatal("differing callback payload was accepted")
	}
	events, err := readHermesTranscript(planDir, "thread_1")
	if err != nil {
		t.Fatal(err)
	}
	if len(events) != 2 || events[1].Content != "done" || events[1].At.IsZero() {
		t.Fatalf("events = %#v", events)
	}
}

func TestHermesTranscriptFailedAppendRestoresOriginalLength(t *testing.T) {
	planDir := filepath.Join(t.TempDir(), "plan")
	if err := os.MkdirAll(planDir, 0o700); err != nil {
		t.Fatal(err)
	}
	if err := AppendHermesTranscript(
		planDir, hermesMetadataFixture("owner/plans/alpha", "thread_1"),
	); err != nil {
		t.Fatal(err)
	}
	path, err := hermesTranscriptReadPath(planDir, "thread_1")
	if err != nil {
		t.Fatal(err)
	}
	before, err := os.Stat(path)
	if err != nil {
		t.Fatal(err)
	}
	injected := errors.New("injected append failure")
	originalWrite := writeHermesTranscriptPayload
	t.Cleanup(func() { writeHermesTranscriptPayload = originalWrite })
	for _, testCase := range []struct {
		name     string
		writeErr error
	}{
		{name: "short"},
		{name: "error", writeErr: injected},
	} {
		t.Run(testCase.name, func(t *testing.T) {
			writeHermesTranscriptPayload = func(file *os.File, payload []byte) (int, error) {
				written, err := file.Write(payload[:len(payload)/2])
				if err != nil {
					return written, err
				}
				return written, testCase.writeErr
			}
			event := HermesTranscriptEvent{
				ID: "event_" + testCase.name, Type: "final", ThreadID: "thread_1",
				PlanDir: "owner/plans/alpha", Content: "never durable",
			}
			err = AppendHermesTranscript(planDir, event)
			writeHermesTranscriptPayload = originalWrite
			if err == nil {
				t.Fatal("failed append returned nil")
			}
			if testCase.writeErr != nil && !errors.Is(err, testCase.writeErr) {
				t.Fatalf("append error = %v", err)
			}
			after, err := os.Stat(path)
			if err != nil {
				t.Fatal(err)
			}
			if after.Size() != before.Size() {
				t.Fatalf("size after failed append = %d, want %d", after.Size(), before.Size())
			}
			events, err := readHermesTranscript(planDir, "thread_1")
			if err != nil {
				t.Fatal(err)
			}
			if len(events) != 1 {
				t.Fatalf("events after failed append = %#v", events)
			}
		})
	}
	if err := AppendHermesTranscript(planDir, HermesTranscriptEvent{
		ID: "event_recovered", Type: "final", ThreadID: "thread_1",
		PlanDir: "owner/plans/alpha", Content: "durable",
	}); err != nil {
		t.Fatalf("append after recovery: %v", err)
	}
}

func TestHermesTranscriptSharedReaderDoesNotObserveInProgressAppend(t *testing.T) {
	planDir := filepath.Join(t.TempDir(), "plan")
	if err := os.MkdirAll(planDir, 0o700); err != nil {
		t.Fatal(err)
	}
	if err := AppendHermesTranscript(
		planDir, hermesMetadataFixture("owner/plans/alpha", "thread_1"),
	); err != nil {
		t.Fatal(err)
	}
	partialWritten := make(chan struct{})
	finishWrite := make(chan struct{})
	originalWrite := writeHermesTranscriptPayload
	writeHermesTranscriptPayload = func(file *os.File, payload []byte) (int, error) {
		first, err := file.Write(payload[:len(payload)/2])
		if err != nil {
			return first, err
		}
		close(partialWritten)
		<-finishWrite
		second, err := file.Write(payload[len(payload)/2:])
		return first + second, err
	}
	t.Cleanup(func() { writeHermesTranscriptPayload = originalWrite })
	appendResult := make(chan error, 1)
	go func() {
		appendResult <- AppendHermesTranscript(planDir, HermesTranscriptEvent{
			ID: "event_1", Type: "final", ThreadID: "thread_1",
			PlanDir: "owner/plans/alpha", Content: "complete",
		})
	}()
	<-partialWritten
	readerContended := make(chan struct{}, 1)
	hermesTranscriptLocalContentionHook = func(shared bool) {
		if shared {
			select {
			case readerContended <- struct{}{}:
			default:
			}
		}
	}
	t.Cleanup(func() { hermesTranscriptLocalContentionHook = nil })
	readResult := make(chan error, 1)
	go func() {
		_, err := readHermesTranscript(planDir, "thread_1")
		readResult <- err
	}()
	<-readerContended
	select {
	case err := <-readResult:
		t.Fatalf("reader returned during partial append: %v", err)
	default:
	}
	close(finishWrite)
	if err := <-appendResult; err != nil {
		t.Fatal(err)
	}
	if err := <-readResult; err != nil {
		t.Fatal(err)
	}
}

func TestHermesTranscriptSubprocessAppendersSerialize(t *testing.T) {
	if os.Getenv("VAMOS_HERMES_APPEND_CHILD") != "" {
		planDir := os.Getenv("VAMOS_HERMES_PLAN")
		id := os.Getenv("VAMOS_HERMES_EVENT")
		event := HermesTranscriptEvent{
			ID: id, At: time.Unix(3, 0).UTC(), Type: "user", ThreadID: "thread_1",
			PlanDir: "owner/plans/alpha", Content: id,
		}
		if err := AppendHermesTranscript(planDir, event); err != nil {
			t.Fatal(err)
		}
		return
	}
	planDir := filepath.Join(t.TempDir(), "plan")
	if err := os.MkdirAll(planDir, 0o700); err != nil {
		t.Fatal(err)
	}
	if err := AppendHermesTranscript(
		planDir, hermesMetadataFixture("owner/plans/alpha", "thread_1"),
	); err != nil {
		t.Fatal(err)
	}
	commands := make([]*exec.Cmd, 2)
	outputs := make([]bytes.Buffer, 2)
	for i, id := range []string{"command_event", "callback_event"} {
		commands[i] = exec.Command(os.Args[0], "-test.run=^TestHermesTranscriptSubprocessAppendersSerialize$")
		commands[i].Env = append(os.Environ(),
			"VAMOS_HERMES_APPEND_CHILD=1", "VAMOS_HERMES_PLAN="+planDir,
			"VAMOS_HERMES_EVENT="+id,
		)
		commands[i].Stdout = &outputs[i]
		commands[i].Stderr = &outputs[i]
		if err := commands[i].Start(); err != nil {
			t.Fatal(err)
		}
	}
	for i, command := range commands {
		if err := command.Wait(); err != nil {
			t.Fatalf("child: %v\n%s", err, outputs[i].String())
		}
	}
	events, err := readHermesTranscriptContext(
		context.Background(), planDir, "owner/plans/alpha", "thread_1",
	)
	if err != nil {
		t.Fatal(err)
	}
	if len(events) != 3 {
		t.Fatalf("events = %d, want 3", len(events))
	}
}

func TestHermesTranscriptReadersCoexistAndLocalLayerPrecedesKernel(t *testing.T) {
	planDir := filepath.Join(t.TempDir(), "plan")
	if err := os.MkdirAll(planDir, 0o700); err != nil {
		t.Fatal(err)
	}
	var localHeld bool
	hermesTranscriptBeforeKernelLockHook = func(lock *sync.RWMutex) {
		if lock.TryLock() {
			lock.Unlock()
			return
		}
		localHeld = true
	}
	first, err := acquireHermesTranscriptLock(context.Background(), planDir, "thread_1", true)
	hermesTranscriptBeforeKernelLockHook = nil
	if err != nil {
		t.Fatal(err)
	}
	if !localHeld {
		t.Fatal("kernel layer was attempted without local ownership")
	}
	second, err := acquireHermesTranscriptLock(context.Background(), planDir, "thread_1", true)
	if err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	writerResult := make(chan error, 1)
	go func() {
		_, err := acquireHermesTranscriptLock(ctx, planDir, "thread_1", false)
		writerResult <- err
	}()
	for registryReferenceCount(first.key) != 3 {
		runtime.Gosched()
	}
	cancel()
	if err := <-writerResult; !errors.Is(err, context.Canceled) {
		t.Fatalf("writer error = %v", err)
	}
	if err := second.Close(); err != nil {
		t.Fatal(err)
	}
	if err := first.Close(); err != nil {
		t.Fatal(err)
	}
}

func TestHermesTranscriptKernelLockCancellationReleasesEarlierOwnership(t *testing.T) {
	if os.Getenv("VAMOS_HERMES_LOCK_CHILD") != "" {
		lock, err := acquireHermesTranscriptLock(
			context.Background(), os.Getenv("VAMOS_HERMES_PLAN"), "thread_1", false,
		)
		if err != nil {
			t.Fatal(err)
		}
		_, _ = os.Stdout.WriteString("ready\n")
		_, _ = bufio.NewReader(os.Stdin).ReadByte()
		if err := lock.Close(); err != nil {
			t.Fatal(err)
		}
		return
	}
	planDir := filepath.Join(t.TempDir(), "plan")
	if err := os.MkdirAll(planDir, 0o700); err != nil {
		t.Fatal(err)
	}
	command := exec.Command(os.Args[0], "-test.run=^TestHermesTranscriptKernelLockCancellationReleasesEarlierOwnership$")
	command.Env = append(os.Environ(), "VAMOS_HERMES_LOCK_CHILD=1", "VAMOS_HERMES_PLAN="+planDir)
	stdin, err := command.StdinPipe()
	if err != nil {
		t.Fatal(err)
	}
	stdout, err := command.StdoutPipe()
	if err != nil {
		t.Fatal(err)
	}
	if err := command.Start(); err != nil {
		t.Fatal(err)
	}
	if line, err := bufio.NewReader(stdout).ReadString('\n'); err != nil || line != "ready\n" {
		t.Fatalf("child readiness = %q, %v", line, err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	result := make(chan error, 1)
	contended := make(chan struct{}, 1)
	waiterFD := -1
	hermesTranscriptLockFileOpenedHook = func(fd int) { waiterFD = fd }
	defer func() { hermesTranscriptLockFileOpenedHook = nil }()
	hermesTranscriptKernelContentionHook = func() {
		select {
		case contended <- struct{}{}:
		default:
		}
	}
	defer func() { hermesTranscriptKernelContentionHook = nil }()
	go func() {
		_, err := acquireHermesTranscriptLock(ctx, planDir, "thread_1", false)
		result <- err
	}()
	<-contended
	cancel()
	if err := <-result; !errors.Is(err, context.Canceled) {
		t.Fatalf("kernel waiter error = %v", err)
	}
	if got := registryReferenceCount(transcriptLockKey(t, planDir, "thread_1")); got != 0 {
		t.Fatalf("references after kernel cancellation = %d", got)
	}
	if waiterFD < 0 {
		t.Fatal("kernel waiter did not open a lock descriptor")
	}
	if _, err := unix.FcntlInt(uintptr(waiterFD), unix.F_GETFD, 0); !errors.Is(err, unix.EBADF) {
		t.Fatalf("waiter descriptor remains open: %v", err)
	}
	_ = stdin.Close()
	if err := command.Wait(); err != nil {
		t.Fatal(err)
	}
	lock, err := acquireHermesTranscriptLock(context.Background(), planDir, "thread_1", false)
	if err != nil {
		t.Fatal(err)
	}
	if err := lock.Close(); err != nil {
		t.Fatal(err)
	}
}

func TestHermesTranscriptLocalLockCancellationCleansRegistry(t *testing.T) {
	planDir := filepath.Join(t.TempDir(), "plan")
	if err := os.MkdirAll(planDir, 0o700); err != nil {
		t.Fatal(err)
	}
	holder, err := acquireHermesTranscriptLock(context.Background(), planDir, "thread_1", false)
	if err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	result := make(chan error, 1)
	waiterOpenedFD := false
	hermesTranscriptLockFileOpenedHook = func(int) { waiterOpenedFD = true }
	defer func() { hermesTranscriptLockFileOpenedHook = nil }()
	go func() {
		_, err := acquireHermesTranscriptLock(ctx, planDir, "thread_1", true)
		result <- err
	}()
	for registryReferenceCount(holder.key) != 2 {
		runtime.Gosched()
	}
	cancel()
	if err := <-result; !errors.Is(err, context.Canceled) {
		t.Fatalf("waiter error = %v", err)
	}
	if got := registryReferenceCount(holder.key); got != 1 {
		t.Fatalf("references while held = %d", got)
	}
	if waiterOpenedFD {
		t.Fatal("local waiter opened a kernel lock descriptor")
	}
	if err := holder.Close(); err != nil {
		t.Fatal(err)
	}
	if got := registryReferenceCount(holder.key); got != 0 {
		t.Fatalf("references after release = %d", got)
	}
}

func transcriptLockKey(t *testing.T, planDir, threadID string) string {
	t.Helper()
	resolved, err := filepath.EvalSymlinks(planDir)
	if err != nil {
		t.Fatal(err)
	}
	return resolved + "\x00" + threadID
}

func registryReferenceCount(key string) int {
	transcriptLocks.mu.Lock()
	defer transcriptLocks.mu.Unlock()
	if entry := transcriptLocks.entries[key]; entry != nil {
		return entry.refs
	}
	return 0
}

func TestHermesTranscriptScannerSeparatesSemanticAndFramingLimits(t *testing.T) {
	planDir := filepath.Join(t.TempDir(), "plan")
	if err := os.MkdirAll(planDir, 0o700); err != nil {
		t.Fatal(err)
	}
	path, err := hermesTranscriptWritePath(planDir, "thread_1")
	if err != nil {
		t.Fatal(err)
	}
	exact := hermesRecordAtSize(t, hermesMetadataFixture("owner/plans/alpha", "thread_1"), maxHermesTranscriptRecordBytes)
	for _, withNewline := range []bool{true, false} {
		payload := append([]byte(nil), exact...)
		if withNewline {
			payload = append(payload, '\n')
		}
		if err := os.WriteFile(path, payload, 0o600); err != nil {
			t.Fatal(err)
		}
		if _, err := readHermesTranscript(planDir, "thread_1"); err != nil {
			t.Fatalf("exact limit with newline=%v: %v", withNewline, err)
		}
	}
	for _, size := range []int{maxHermesTranscriptRecordBytes + 1, hermesTranscriptScannerCapacity + 1} {
		for _, withNewline := range []bool{true, false} {
			payload := bytes.Repeat([]byte{'x'}, size)
			if withNewline {
				payload = append(payload, '\n')
			}
			if err := os.WriteFile(path, payload, 0o600); err != nil {
				t.Fatal(err)
			}
			if _, err := readHermesTranscript(planDir, "thread_1"); err == nil {
				t.Fatalf("size=%d newline=%v succeeded", size, withNewline)
			}
		}
	}
}

func hermesRecordAtSize(t *testing.T, event HermesTranscriptEvent, size int) []byte {
	t.Helper()
	event.Title = "x"
	data, err := json.Marshal(event)
	if err != nil {
		t.Fatal(err)
	}
	if len(data) > size {
		t.Fatalf("base event is larger than %d", size)
	}
	event.Title += strings.Repeat("x", size-len(data))
	data, err = json.Marshal(event)
	if err != nil {
		t.Fatal(err)
	}
	if len(data) != size {
		t.Fatalf("record size = %d, want %d", len(data), size)
	}
	return data
}

func TestHermesTranscriptRecordSemanticAndFramingLimits(t *testing.T) {
	planDir := filepath.Join(t.TempDir(), "plan")
	if err := os.MkdirAll(planDir, 0o700); err != nil {
		t.Fatal(err)
	}
	metadata := hermesMetadataFixture("owner/plans/alpha", "thread_1")
	if err := AppendHermesTranscript(planDir, metadata); err != nil {
		t.Fatal(err)
	}
	base := HermesTranscriptEvent{
		ID: "limit_event", At: time.Unix(2, 0).UTC(), Type: "user",
		ThreadID: "thread_1", PlanDir: "owner/plans/alpha",
	}
	baseBytes, err := json.Marshal(base)
	if err != nil {
		t.Fatal(err)
	}
	base.Content = strings.Repeat("x", maxHermesTranscriptRecordBytes-len(baseBytes)-13)
	for {
		data, err := json.Marshal(base)
		if err != nil {
			t.Fatal(err)
		}
		delta := maxHermesTranscriptRecordBytes - len(data)
		if delta == 0 {
			break
		}
		base.Content += strings.Repeat("x", delta)
	}
	if err := AppendHermesTranscript(planDir, base); err != nil {
		t.Fatalf("exact limit append: %v", err)
	}
	if _, err := readHermesTranscript(planDir, "thread_1"); err != nil {
		t.Fatalf("exact limit read: %v", err)
	}
	base.ID = "over_limit"
	base.Content += "xx"
	if err := AppendHermesTranscript(planDir, base); err == nil {
		t.Fatal("over-limit append succeeded")
	}
}
