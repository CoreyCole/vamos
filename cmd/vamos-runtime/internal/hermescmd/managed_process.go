package hermescmd

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"time"

	"github.com/CoreyCole/vamos/pkg/hermes/sessioningress"
)

const (
	launchNonceLabel      = "vamos-hermes-launch-nonce-v1"
	managedDrainTimeout   = 2 * time.Second
	managedTerminateGrace = 2 * time.Second
	parentReadTimeout     = 2 * time.Second
	parentExchangeTimeout = 3 * time.Second
	parentTotalTimeout    = 10 * time.Second
	parentMaximumAttempts = 3
)

type ParentClientConfig struct {
	HermesHome        string
	GatewayBaseURL    string
	GatewayCredential string
	ConnectTimeout    time.Duration
	WriteTimeout      time.Duration
	ReadTimeout       time.Duration
	ExchangeTimeout   time.Duration
	TotalTimeout      time.Duration
	MaxAttempts       int
	BackoffCap        time.Duration
}

type NotifierFactory func(validatedHermesSessionID string, config ParentClientConfig) (sessioningress.Notifier, error)

type ProcessSpec struct {
	Name       string
	Args       []string
	Env        []string
	Stdin      io.Reader
	Stdout     io.Writer
	Stderr     io.Writer
	ExtraFiles []*os.File
}

type ManagedCommand interface {
	Start() error
	Wait() error
	Signal(signal os.Signal) error
	Kill() error
}

type StartWaitProcessFactory func(context.Context, ProcessSpec) ManagedCommand

type managedProcessInput struct {
	PlanDir         string
	PiSessionID     string
	HermesSessionID string
	Name            string
	Args            []string
	Environment     []string
	Stdin           io.Reader
	Stdout          io.Writer
	Stderr          io.Writer
	Notifier        sessioningress.Notifier
	ProcessFactory  StartWaitProcessFactory
	DrainTimeout    time.Duration
	TerminateGrace  time.Duration
	NotifyTimeout   time.Duration
	OwnerUID        int
}

type SettlementNotificationResult struct {
	EventIndex int
	MessageID  string
	Result     sessioningress.NotifyResult
	Err        error
}

type ManagedProcessResult struct {
	Notifications []SettlementNotificationResult
	LaunchError   error
	ProtocolError error
	ChildError    error
	Secondary     []error
}

func (result ManagedProcessResult) Err() error {
	if result.LaunchError != nil {
		return result.LaunchError
	}
	if result.ProtocolError != nil {
		return result.ProtocolError
	}
	if result.ChildError != nil {
		return result.ChildError
	}
	if len(result.Notifications) == 0 {
		return errors.New("managed Pi exited without a settlement notification")
	}
	for _, event := range result.Notifications {
		if event.Err != nil {
			return event.Err
		}
		if !event.Result.Admission {
			return fmt.Errorf(
				"settlement notification %d was not admitted: %s",
				event.EventIndex,
				event.Result.Code,
			)
		}
	}

	return nil
}

func deriveLaunchNonce(piSessionID string) string {
	digest := sha256.New()
	_, _ = digest.Write([]byte(launchNonceLabel))
	_, _ = digest.Write([]byte{0})
	_, _ = digest.Write([]byte(piSessionID))

	return hex.EncodeToString(digest.Sum(nil))
}

func runManagedProcess(
	ctx context.Context,
	input managedProcessInput,
) ManagedProcessResult {
	var result ManagedProcessResult
	if input.ProcessFactory == nil || input.Notifier == nil {
		result.LaunchError = errors.New("managed process dependencies are required")

		return result
	}
	if err := sessioningress.ValidatePiSessionID(input.PiSessionID); err != nil {
		result.LaunchError = errors.New("invalid managed Pi session ID")

		return result
	}
	if _, err := sessioningress.ValidateSessionID(input.HermesSessionID); err != nil {
		result.LaunchError = errors.New("invalid managed Hermes session ID")

		return result
	}
	if input.DrainTimeout <= 0 || input.TerminateGrace <= 0 || input.NotifyTimeout <= 0 {
		result.LaunchError = errors.New("managed process deadlines must be positive")

		return result
	}

	sessionPath := filepath.Join(
		input.PlanDir,
		".vamos",
		"sessions",
		"pi",
		input.PiSessionID,
	)
	if err := os.MkdirAll(sessionPath, 0o700); err != nil {
		result.LaunchError = fmt.Errorf("create Pi session directory: %w", err)

		return result
	}
	sessionDirectory, err := os.Open(sessionPath)
	if err != nil {
		result.LaunchError = fmt.Errorf("open Pi session directory: %w", err)

		return result
	}
	defer sessionDirectory.Close()

	reader, writer, err := os.Pipe()
	if err != nil {
		result.LaunchError = fmt.Errorf("create settlement handoff pipe: %w", err)

		return result
	}
	defer reader.Close()
	handoffFD := firstNonstandardDescriptor
	environment, err := BuildManagedChildEnvironment(ManagedChildEnvironmentInput{
		Base: input.Environment,
		Overrides: []string{
			"VAMOS_PLAN_DIR=" + input.PlanDir,
			"VAMOS_THOUGHTS_ROOT=" + thoughtsRoot(input.PlanDir),
			"PI_SESSION_ID=" + input.PiSessionID,
		},
		Managed: true, HermesSessionID: input.HermesSessionID, HandoffFD: &handoffFD,
	})
	if err != nil {
		_ = writer.Close()
		result.LaunchError = err

		return result
	}
	process := input.ProcessFactory(ctx, ProcessSpec{
		Name: input.Name, Args: input.Args, Env: environment, Stdin: input.Stdin,
		Stdout: input.Stdout, Stderr: input.Stderr, ExtraFiles: []*os.File{writer},
	})
	if process == nil {
		_ = writer.Close()
		result.LaunchError = errors.New("process factory returned nil")

		return result
	}
	if err := process.Start(); err != nil {
		_ = writer.Close()
		result.LaunchError = fmt.Errorf("start managed Pi: %w", err)

		return result
	}
	if err := writer.Close(); err != nil {
		result.Secondary = append(
			result.Secondary,
			fmt.Errorf("close parent handoff writer: %w", err),
		)
	}

	processCtx, cancelNetwork := context.WithCancel(ctx)
	defer cancelNetwork()
	readDone := make(chan managedReadResult, 1)
	go func() {
		readDone <- readAndNotifySettlements(processCtx, reader, sessionDirectory, input)
	}()
	waitDone := make(chan error, 1)
	go func() { waitDone <- process.Wait() }()

	var readResult managedReadResult
	var readChannel <-chan managedReadResult = readDone
	var waitChannel <-chan error = waitDone
	cancelChannel := ctx.Done()
	for readChannel != nil || waitChannel != nil {
		select {
		case waitErr := <-waitChannel:
			waitChannel = nil
			result.ChildError = waitErr
			if readChannel != nil {
				readResult = drainManagedReader(
					cancelNetwork,
					reader,
					readChannel,
					input.DrainTimeout,
				)
				readChannel = nil
			}
		case readResult = <-readChannel:
			readChannel = nil
			if readResult.err != nil && waitChannel != nil {
				var secondary []error
				secondary, result.ChildError = terminateAndReap(
					process,
					waitChannel,
					input.TerminateGrace,
				)
				result.Secondary = append(result.Secondary, secondary...)
				waitChannel = nil
			}
		case <-cancelChannel:
			cancelChannel = nil
			cancelNetwork()
			if waitChannel != nil {
				var secondary []error
				secondary, result.ChildError = terminateAndReap(
					process,
					waitChannel,
					input.TerminateGrace,
				)
				result.Secondary = append(result.Secondary, secondary...)
				waitChannel = nil
			}
			if readChannel != nil {
				readResult = drainManagedReader(
					cancelNetwork,
					reader,
					readChannel,
					input.DrainTimeout,
				)
				readChannel = nil
			}
		}
	}
	result.Notifications = readResult.events
	result.ProtocolError = readResult.err

	return result
}

func drainManagedReader(
	cancelNetwork context.CancelFunc,
	reader io.Closer,
	read <-chan managedReadResult,
	timeout time.Duration,
) managedReadResult {
	timer := time.NewTimer(timeout)
	select {
	case result := <-read:
		if !timer.Stop() {
			<-timer.C
		}

		return result
	case <-timer.C:
		cancelNetwork()
		_ = reader.Close()
		result := <-read
		if result.err == nil {
			result.err = errors.New("handoff drain deadline exceeded")
		}

		return result
	}
}

func terminateAndReap(
	process ManagedCommand,
	wait <-chan error,
	grace time.Duration,
) ([]error, error) {
	secondary := make([]error, 0)
	if err := requestGracefulTermination(process); err != nil {
		secondary = append(secondary, fmt.Errorf("terminate managed Pi: %w", err))
	}
	timer := time.NewTimer(grace)
	select {
	case waitErr := <-wait:
		if !timer.Stop() {
			<-timer.C
		}

		return secondary, waitErr
	case <-timer.C:
		if err := process.Kill(); err != nil {
			secondary = append(secondary, fmt.Errorf("kill managed Pi: %w", err))
		}

		return secondary, <-wait
	}
}

type managedReadResult struct {
	events []SettlementNotificationResult
	err    error
}

func readAndNotifySettlements(
	ctx context.Context,
	reader io.Reader,
	directory *os.File,
	input managedProcessInput,
) managedReadResult {
	result := managedReadResult{events: make([]SettlementNotificationResult, 0)}
	seen := make(map[string]struct{})
	nonce := deriveLaunchNonce(input.PiSessionID)
	for index := range MaxHandoffFrames {
		frame, err := ReadHandoffFrame(reader)
		if errors.Is(err, io.EOF) || errors.Is(err, io.ErrClosedPipe) {
			return result
		}
		if err != nil {
			result.err = err

			return result
		}
		if err := ValidateExpectedHandoffFrame(
			frame,
			nonce,
			input.PiSessionID,
		); err != nil {
			result.err = err

			return result
		}
		replayKey := frame.PiSessionID + "\x00" + frame.MessageID
		if _, duplicate := seen[replayKey]; duplicate {
			result.err = errors.New("duplicate settlement handoff")

			return result
		}
		seen[replayKey] = struct{}{}
		evidence, err := LoadSettlementEvidence(directory, SettlementLoadExpectation{
			HermesSessionID: input.HermesSessionID,
			Frame:           frame,
			OwnerUID:        input.OwnerUID,
		})
		event := SettlementNotificationResult{
			EventIndex: index,
			MessageID:  frame.MessageID,
			Err:        err,
		}
		if err == nil {
			notifyCtx, cancel := context.WithTimeout(ctx, input.NotifyTimeout)
			event.Result = input.Notifier.Notify(
				notifyCtx,
				enqueueRequestFromEvidence(evidence),
			)
			cancel()
		}
		result.events = append(result.events, event)
		if err != nil {
			result.err = err

			return result
		}
	}
	if _, err := ReadHandoffFrame(reader); err == nil {
		result.err = errors.New("handoff frame count exceeds limit")
	} else if !errors.Is(err, io.EOF) && !errors.Is(err, io.ErrClosedPipe) {
		result.err = err
	}

	return result
}

func enqueueRequestFromEvidence(
	evidence SettlementEvidenceV1,
) sessioningress.EnqueueRequest {
	return sessioningress.EnqueueRequest{
		HermesSessionID: evidence.HermesSessionID,
		Message:         evidence.RawResponse,
		MessageID:       evidence.MessageID,
		Op:              "enqueue",
		PiSessionID:     evidence.PiSessionID,
		Version:         sessioningress.ProtocolVersion,
	}
}

func defaultNotifierFactory(
	validatedHermesSessionID string,
	config ParentClientConfig,
) (sessioningress.Notifier, error) {
	if _, err := sessioningress.ValidateSessionID(validatedHermesSessionID); err != nil {
		return nil, fmt.Errorf("validate Hermes session ID: %w", err)
	}

	return sessioningress.NewNotifier(sessionIngressClientConfig(config))
}

func sessionIngressClientConfig(config ParentClientConfig) sessioningress.ClientConfig {
	return sessioningress.ClientConfig{
		HermesHome: config.HermesHome, ConnectTimeout: config.ConnectTimeout,
		WriteTimeout: config.WriteTimeout, ReadTimeout: config.ReadTimeout,
		ExchangeTimeout: config.ExchangeTimeout, TotalTimeout: config.TotalTimeout,
		MaxAttempts: config.MaxAttempts, BackoffCap: config.BackoffCap,
		GatewayBaseURL:    config.GatewayBaseURL,
		GatewayCredential: config.GatewayCredential,
	}
}

type execManagedCommand struct{ command *exec.Cmd }

func (command *execManagedCommand) Start() error { return command.command.Start() }
func (command *execManagedCommand) Wait() error  { return command.command.Wait() }
func (command *execManagedCommand) Signal(signal os.Signal) error {
	if command.command.Process == nil {
		return errors.New("process has not started")
	}

	return command.command.Process.Signal(signal)
}

func (command *execManagedCommand) Kill() error {
	if command.command.Process == nil {
		return errors.New("process has not started")
	}

	return command.command.Process.Kill()
}

func defaultStartWaitProcessFactory(
	ctx context.Context,
	spec ProcessSpec,
) ManagedCommand {
	//nolint:gosec // The CLI intentionally launches the configured Pi executable and caller-built arguments.
	command := exec.CommandContext(context.WithoutCancel(ctx), spec.Name, spec.Args...)
	command.Env = spec.Env
	command.Stdin = spec.Stdin
	command.Stdout = spec.Stdout
	command.Stderr = spec.Stderr
	command.ExtraFiles = spec.ExtraFiles

	return &execManagedCommand{command: command}
}
