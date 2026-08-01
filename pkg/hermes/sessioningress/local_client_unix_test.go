//go:build !windows

package sessioningress

import (
	"bytes"
	"net"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

func TestLocalClientCapabilityThenExactEnqueue(t *testing.T) {
	t.Parallel()

	request := testEnqueueRequest()
	want, err := EncodeCanonical(request)
	if err != nil {
		t.Fatal(err)
	}
	var received [][]byte
	server := startLocalFixture(
		t,
		request.HermesSessionID,
		func(_ int, payload []byte) ([]byte, time.Duration) {
			received = append(received, append([]byte(nil), payload...))
			parsed, parseErr := ParseRequest(payload)
			if parseErr != nil {
				t.Errorf("parse request: %v", parseErr)

				return nil, 0
			}
			switch parsed.(type) {
			case CapabilityRequest:
				return mustCanonical(t, CapabilityResponse{
					Capabilities:     []string{ExactSessionCapability},
					Code:             "capabilities",
					MaxFrameBytes:    MaxFrameBytes,
					ProtocolVersions: []int{ProtocolVersion},
					Version:          ProtocolVersion,
				}), 0
			case EnqueueRequest:
				return mustCanonical(
					t,
					AcceptedResponse{Code: "accepted_idle", Version: ProtocolVersion},
				), 0
			default:
				t.Errorf("unexpected request type %T", parsed)

				return nil, 0
			}
		},
	)
	defer server.Close()
	config := testClientConfig(t)
	config.HermesHome = server.hermesHome
	notifier, err := NewNotifier(config)
	if err != nil {
		t.Fatal(err)
	}
	result := notifier.Notify(t.Context(), request)
	if !result.Admission || result.Transport != TransportLocal || result.Attempts != 1 {
		t.Fatalf("result = %#v", result)
	}
	if len(received) != 2 || !bytes.Equal(received[1], want) {
		t.Fatalf("received payloads = %q", received)
	}
}

func TestLiveLocalAnswerNeverFallsThroughToGateway(t *testing.T) {
	t.Parallel()

	request := testEnqueueRequest()
	server := startLocalFixture(
		t,
		request.HermesSessionID,
		func(_ int, _ []byte) ([]byte, time.Duration) {
			return mustCanonical(t, CapabilityResponse{
				Capabilities:     []string{"other-capability"},
				Code:             "capabilities",
				MaxFrameBytes:    MaxFrameBytes,
				ProtocolVersions: []int{ProtocolVersion},
				Version:          ProtocolVersion,
			}), 0
		},
	)
	defer server.Close()
	var gatewayCalls atomic.Int32
	gateway := httptest.NewServer(
		http.HandlerFunc(func(http.ResponseWriter, *http.Request) {
			gatewayCalls.Add(1)
		}),
	)
	defer gateway.Close()
	config := testClientConfig(t)
	config.HermesHome = server.hermesHome
	config.GatewayBaseURL = gateway.URL
	config.GatewayCredential = testGatewayCredential
	notifier, err := NewNotifier(config)
	if err != nil {
		t.Fatal(err)
	}
	result := notifier.Notify(t.Context(), request)
	if result.Code != "surface_unsupported" || result.Retryable ||
		gatewayCalls.Load() != 0 {
		t.Fatalf("result = %#v, gateway calls = %d", result, gatewayCalls.Load())
	}
}

func TestLocalRetryReusesImmutableEnqueueBytes(t *testing.T) {
	t.Parallel()

	request := testEnqueueRequest()
	want, err := EncodeCanonical(request)
	if err != nil {
		t.Fatal(err)
	}
	var enqueueBodies [][]byte
	server := startLocalFixture(
		t,
		request.HermesSessionID,
		func(_ int, payload []byte) ([]byte, time.Duration) {
			parsed, parseErr := ParseRequest(payload)
			if parseErr != nil {
				t.Errorf("parse request: %v", parseErr)

				return nil, 0
			}
			if _, ok := parsed.(CapabilityRequest); ok {
				return mustCanonical(t, CapabilityResponse{
					Capabilities:     []string{ExactSessionCapability},
					Code:             "capabilities",
					MaxFrameBytes:    MaxFrameBytes,
					ProtocolVersions: []int{ProtocolVersion},
					Version:          ProtocolVersion,
				}), 0
			}
			enqueueBodies = append(enqueueBodies, append([]byte(nil), payload...))
			if len(enqueueBodies) == 1 {
				retry := 1

				return mustCanonical(t, RejectionResponse{
					Code:         "temporarily_unavailable",
					RetryAfterMS: &retry,
					Version:      ProtocolVersion,
				}), 0
			}

			return mustCanonical(
				t,
				AcceptedResponse{Code: "accepted_queued", Version: ProtocolVersion},
			), 0
		},
	)
	defer server.Close()
	config := testClientConfig(t)
	config.HermesHome = server.hermesHome
	notifier, err := NewNotifier(config)
	if err != nil {
		t.Fatal(err)
	}
	result := notifier.Notify(t.Context(), request)
	if !result.Admission || result.Attempts != 2 {
		t.Fatalf("result = %#v", result)
	}
	if len(enqueueBodies) != 2 || !bytes.Equal(enqueueBodies[0], want) ||
		!bytes.Equal(enqueueBodies[1], want) {
		t.Fatalf("enqueue bytes varied: %q", enqueueBodies)
	}
}

func TestLocalTimeoutAfterWriteIsUncertainAndBounded(t *testing.T) {
	t.Parallel()

	request := testEnqueueRequest()
	server := startLocalFixture(
		t,
		request.HermesSessionID,
		func(_ int, payload []byte) ([]byte, time.Duration) {
			parsed, parseErr := ParseRequest(payload)
			if parseErr != nil {
				t.Errorf("parse request: %v", parseErr)

				return nil, 0
			}
			if _, ok := parsed.(CapabilityRequest); ok {
				return mustCanonical(t, CapabilityResponse{
					Capabilities:     []string{ExactSessionCapability},
					Code:             "capabilities",
					MaxFrameBytes:    MaxFrameBytes,
					ProtocolVersions: []int{ProtocolVersion},
					Version:          ProtocolVersion,
				}), 0
			}

			return nil, 150 * time.Millisecond
		},
	)
	defer server.Close()
	config := testClientConfig(t)
	config.HermesHome = server.hermesHome
	config.ReadTimeout = 20 * time.Millisecond
	config.MaxAttempts = 2
	notifier, err := NewNotifier(config)
	if err != nil {
		t.Fatal(err)
	}
	result := notifier.Notify(t.Context(), request)
	if result.Code != "temporarily_unavailable" || !result.Retryable ||
		!result.Uncertain ||
		result.Attempts != 2 {
		t.Fatalf("result = %#v", result)
	}
}

func TestSafelyRefusedStaleSocketAllowsGatewayFallback(t *testing.T) {
	t.Parallel()

	request := testEnqueueRequest()
	home := shortHermesHome(t)
	euid, err := CurrentEUID()
	if err != nil {
		t.Fatal(err)
	}
	path, err := DeriveSocketPath(request.HermesSessionID, home, euid)
	if err != nil {
		t.Fatal(err)
	}
	if err := PrepareRuntimeDirectory(filepath.Dir(path), euid); err != nil {
		t.Fatal(err)
	}
	listener, err := (&net.ListenConfig{}).Listen(t.Context(), "unix", path)
	if err != nil {
		t.Fatal(err)
	}
	unixListener, ok := listener.(*net.UnixListener)
	if !ok {
		t.Fatalf("listener type = %T", listener)
	}
	unixListener.SetUnlinkOnClose(false)
	if err := listener.Close(); err != nil {
		t.Fatal(err)
	}
	gateway := httptest.NewServer(
		http.HandlerFunc(func(writer http.ResponseWriter, httpRequest *http.Request) {
			writer.Header().Set("Content-Type", "application/json")
			if httpRequest.URL.Path == capabilitiesPath {
				writeProtocolResponse(t, writer, CapabilityResponse{
					Capabilities:     []string{ExactSessionCapability},
					Code:             "capabilities",
					MaxFrameBytes:    MaxFrameBytes,
					ProtocolVersions: []int{ProtocolVersion},
					Version:          ProtocolVersion,
				})

				return
			}
			writeProtocolResponse(
				t,
				writer,
				AcceptedResponse{Code: "accepted_idle", Version: ProtocolVersion},
			)
		}),
	)
	defer gateway.Close()
	config := testClientConfig(t)
	config.HermesHome = home
	config.GatewayBaseURL = gateway.URL
	config.GatewayCredential = testGatewayCredential
	notifier, err := NewNotifier(config)
	if err != nil {
		t.Fatal(err)
	}
	result := notifier.Notify(t.Context(), request)
	if !result.Admission || result.Transport != TransportGateway {
		t.Fatalf("result = %#v", result)
	}
}

type localFixture struct {
	hermesHome  string
	listener    net.Listener
	connections sync.WaitGroup
}

func startLocalFixture(
	t *testing.T,
	sessionID string,
	handler func(int, []byte) ([]byte, time.Duration),
) *localFixture {
	t.Helper()
	home := shortHermesHome(t)
	euid, err := CurrentEUID()
	if err != nil {
		t.Fatal(err)
	}
	path, err := DeriveSocketPath(sessionID, home, euid)
	if err != nil {
		t.Fatal(err)
	}
	if err := PrepareRuntimeDirectory(filepath.Dir(path), euid); err != nil {
		t.Fatal(err)
	}
	listener, err := (&net.ListenConfig{}).Listen(t.Context(), "unix", path)
	if err != nil {
		t.Fatal(err)
	}
	fixture := &localFixture{hermesHome: home, listener: listener}
	var handlerMu sync.Mutex
	go func() {
		index := 0
		for {
			connection, acceptErr := listener.Accept()
			if acceptErr != nil {
				return
			}
			current := index
			index++
			fixture.connections.Add(1)
			go func() {
				defer fixture.connections.Done()
				defer connection.Close()
				payload, readErr := ReadFrame(connection)
				if readErr != nil {
					return
				}
				handlerMu.Lock()
				response, delay := handler(current, payload)
				handlerMu.Unlock()
				if delay > 0 {
					time.Sleep(delay)
				}
				if response != nil {
					_ = WriteFrame(connection, response)
				}
			}()
		}
	}()

	return fixture
}

func shortHermesHome(t *testing.T) string {
	t.Helper()
	//nolint:usetesting // The short root keeps the fixture below Darwin's socket path limit.
	home, err := os.MkdirTemp("/tmp", "h")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.RemoveAll(home) })

	return home
}

func (fixture *localFixture) Close() {
	_ = fixture.listener.Close()
	fixture.connections.Wait()
}
