package sessioningress

import (
	"bytes"
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"net/http/httptrace"
	"strings"
	"sync"
	"testing"
	"time"
)

func TestGatewayAbsenceFallbackSendsExactCanonicalRawSettlement(t *testing.T) {
	t.Parallel()

	request := testEnqueueRequest()
	wantEnqueue, err := EncodeCanonical(request)
	if err != nil {
		t.Fatal(err)
	}
	wantCapability, err := EncodeCanonical(
		CapabilityRequest{Op: "capabilities", Version: ProtocolVersion},
	)
	if err != nil {
		t.Fatal(err)
	}
	var paths []string
	server := httptest.NewServer(
		http.HandlerFunc(func(writer http.ResponseWriter, httpRequest *http.Request) {
			body, readErr := io.ReadAll(httpRequest.Body)
			if readErr != nil {
				t.Errorf("read request: %v", readErr)
			}
			paths = append(paths, httpRequest.URL.Path)
			if got := httpRequest.Header.Get(
				"Authorization",
			); got != "Bearer ingress-secret" {
				t.Errorf("Authorization = %q", got)
			}
			writer.Header().Set("Content-Type", "application/json")
			switch httpRequest.URL.Path {
			case capabilitiesPath:
				if !bytes.Equal(body, wantCapability) {
					t.Errorf("capability body = %q", body)
				}
				writeProtocolResponse(t, writer, CapabilityResponse{
					Capabilities:     []string{ExactSessionCapability},
					Code:             "capabilities",
					MaxFrameBytes:    MaxFrameBytes,
					ProtocolVersions: []int{ProtocolVersion},
					Version:          ProtocolVersion,
				})
			case enqueuePath:
				if !bytes.Equal(body, wantEnqueue) {
					t.Errorf("enqueue body changed:\n got %q\nwant %q", body, wantEnqueue)
				}
				if string(body) == managerWrappedForTest(request.Message) {
					t.Error("Vamos pre-wrapped the raw settlement")
				}
				writeProtocolResponse(
					t,
					writer,
					AcceptedResponse{Code: "accepted_idle", Version: ProtocolVersion},
				)
			default:
				t.Errorf("unexpected path %q", httpRequest.URL.Path)
			}
		}),
	)
	defer server.Close()

	config := testClientConfig(t)
	config.GatewayBaseURL = server.URL
	config.GatewayCredential = "ingress-secret"
	notifier, err := NewNotifier(config)
	if err != nil {
		t.Fatal(err)
	}
	result := notifier.Notify(t.Context(), request)
	if !result.Admission || result.Code != "accepted_idle" ||
		result.Transport != TransportGateway ||
		result.Attempts != 1 {
		t.Fatalf("result = %#v", result)
	}
	if len(paths) != 2 || paths[0] != capabilitiesPath || paths[1] != enqueuePath {
		t.Fatalf("paths = %#v", paths)
	}
}

func TestGatewayRetriesCanonicalBytesWithStableMessageID(t *testing.T) {
	t.Parallel()

	request := testEnqueueRequest()
	want, err := EncodeCanonical(request)
	if err != nil {
		t.Fatal(err)
	}
	var mu sync.Mutex
	var bodies [][]byte
	server := httptest.NewServer(
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
			body, readErr := io.ReadAll(httpRequest.Body)
			if readErr != nil {
				t.Errorf("read request: %v", readErr)
			}
			mu.Lock()
			bodies = append(bodies, body)
			attempt := len(bodies)
			mu.Unlock()
			if attempt < 3 {
				retry := 60_000
				writeProtocolResponse(t, writer, RejectionResponse{
					Code: "queue_full", RetryAfterMS: &retry, Version: ProtocolVersion,
				})

				return
			}
			writeProtocolResponse(
				t,
				writer,
				AcceptedResponse{Code: "accepted_queued", Version: ProtocolVersion},
			)
		}),
	)
	defer server.Close()

	config := testClientConfig(t)
	config.GatewayBaseURL = server.URL
	config.GatewayCredential = testGatewayCredential
	var delays []time.Duration
	config.Sleep = func(_ context.Context, delay time.Duration) error {
		delays = append(delays, delay)

		return nil
	}
	notifier, err := NewNotifier(config)
	if err != nil {
		t.Fatal(err)
	}
	result := notifier.Notify(t.Context(), request)
	if !result.Admission || result.Attempts != 3 || result.Code != "accepted_queued" {
		t.Fatalf("result = %#v", result)
	}
	for index, body := range bodies {
		if !bytes.Equal(body, want) {
			t.Fatalf("attempt %d body changed", index+1)
		}
	}
	for _, delay := range delays {
		if delay > config.BackoffCap {
			t.Fatalf("retry delay %v exceeds cap %v", delay, config.BackoffCap)
		}
	}
}

func TestGatewayCapabilityMismatchAndMalformedResponseAreTerminal(t *testing.T) {
	t.Parallel()

	cases := []struct {
		name    string
		handler http.HandlerFunc
	}{
		{
			name: "capability mismatch",
			handler: func(writer http.ResponseWriter, _ *http.Request) {
				writer.Header().Set("Content-Type", "application/json")
				writeProtocolResponse(t, writer, CapabilityResponse{
					Capabilities:     []string{"other-capability"},
					Code:             "capabilities",
					MaxFrameBytes:    MaxFrameBytes,
					ProtocolVersions: []int{ProtocolVersion},
					Version:          ProtocolVersion,
				})
			},
		},
		{
			name: "wrong content type",
			handler: func(writer http.ResponseWriter, _ *http.Request) {
				writer.Header().Set("Content-Type", "text/plain")
				writer.WriteHeader(http.StatusOK)
				_, _ = writer.Write([]byte("not-json"))
			},
		},
	}
	for _, test := range cases {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()

			server := httptest.NewServer(test.handler)
			defer server.Close()
			config := testClientConfig(t)
			config.GatewayBaseURL = server.URL
			config.GatewayCredential = testGatewayCredential
			notifier, err := NewNotifier(config)
			if err != nil {
				t.Fatal(err)
			}
			result := notifier.Notify(t.Context(), testEnqueueRequest())
			if result.Retryable || result.Admission || result.Attempts != 1 {
				t.Fatalf("result = %#v", result)
			}
		})
	}
}

func TestGatewayCancellationRedactsDiagnosticsAndStopsRequest(t *testing.T) {
	t.Parallel()

	cancelObserved := make(chan struct{})
	config := testClientConfig(t)
	config.GatewayBaseURL = "https://private-gateway.example"
	config.GatewayCredential = "highly-secret"
	config.TotalTimeout = 50 * time.Millisecond
	config.ExchangeTimeout = time.Second
	config.HTTPClient = &http.Client{
		Transport: roundTripFunc(func(request *http.Request) (*http.Response, error) {
			if request.URL.Path == capabilitiesPath {
				payload := mustCanonical(t, CapabilityResponse{
					Capabilities:     []string{ExactSessionCapability},
					Code:             "capabilities",
					MaxFrameBytes:    MaxFrameBytes,
					ProtocolVersions: []int{ProtocolVersion},
					Version:          ProtocolVersion,
				})

				return &http.Response{
					StatusCode: http.StatusOK,
					Header: http.Header{
						"Content-Type": []string{"application/json"},
					},
					Body:          io.NopCloser(bytes.NewReader(payload)),
					ContentLength: int64(len(payload)),
				}, nil
			}
			if trace := httptrace.ContextClientTrace(
				request.Context(),
			); trace != nil &&
				trace.WroteHeaders != nil {
				trace.WroteHeaders()
			}
			<-request.Context().Done()
			close(cancelObserved)

			return nil, request.Context().Err()
		}),
	}
	notifier, err := NewNotifier(config)
	if err != nil {
		t.Fatal(err)
	}
	result := notifier.Notify(t.Context(), testEnqueueRequest())
	if result.Code != "canceled" || !result.Uncertain {
		t.Fatalf("result = %#v", result)
	}
	select {
	case <-cancelObserved:
	case <-time.After(time.Second):
		t.Fatal("gateway transport did not observe cancellation")
	}
	formatted := result.Code + result.Detail
	if strings.Contains(formatted, "highly-secret") ||
		strings.Contains(formatted, "private-gateway") {
		t.Fatalf("result leaked transport authority: %#v", result)
	}
}

func writeProtocolResponse(t *testing.T, writer http.ResponseWriter, response Response) {
	t.Helper()
	payload, err := EncodeCanonical(response)
	if err != nil {
		t.Errorf("encode response: %v", err)

		return
	}
	status, err := HTTPStatusForCode(responseCode(response))
	if err != nil {
		t.Errorf("response status: %v", err)

		return
	}
	writer.WriteHeader(status)
	if _, err := writer.Write(payload); err != nil {
		t.Errorf("write response: %v", err)
	}
}

func managerWrappedForTest(message string) string {
	return "A managed Pi child settled.\n\n" + message
}
