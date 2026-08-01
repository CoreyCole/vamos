package sessioningress

import (
	"context"
	"net/http"
	"strings"
	"testing"
	"time"
)

const testGatewayCredential = "secret"

func testClientConfig(t *testing.T) ClientConfig {
	t.Helper()

	return ClientConfig{
		HermesHome:      t.TempDir(),
		ConnectTimeout:  100 * time.Millisecond,
		WriteTimeout:    100 * time.Millisecond,
		ReadTimeout:     100 * time.Millisecond,
		ExchangeTimeout: 250 * time.Millisecond,
		TotalTimeout:    time.Second,
		MaxAttempts:     3,
		BackoffCap:      10 * time.Millisecond,
		Sleep: func(ctx context.Context, _ time.Duration) error {
			return ctx.Err()
		},
	}
}

func testEnqueueRequest() EnqueueRequest {
	return EnqueueRequest{
		HermesSessionID: "20260731_153837_5d85bf",
		Message:         "outcome: complete\nnext: successor\n```yaml\npi done\n```",
		MessageID:       "pi-settlement-v1-test",
		Op:              "enqueue",
		PiSessionID:     "pi-123",
		Version:         ProtocolVersion,
	}
}

func TestNewNotifierRejectsUnboundedAndPartialConfiguration(t *testing.T) {
	t.Parallel()

	valid := testClientConfig(t)
	cases := []ClientConfig{
		{},
		func() ClientConfig {
			value := valid
			value.MaxAttempts = 0

			return value
		}(),
		func() ClientConfig {
			value := valid
			value.TotalTimeout = 0

			return value
		}(),
		func() ClientConfig {
			value := valid
			value.GatewayBaseURL = "https://gateway.example"

			return value
		}(),
		func() ClientConfig {
			value := valid
			value.GatewayCredential = testGatewayCredential

			return value
		}(),
		func() ClientConfig {
			value := valid
			value.GatewayBaseURL = "https://user:secret@gateway.example"
			value.GatewayCredential = testGatewayCredential

			return value
		}(),
	}
	for index, config := range cases {
		if _, err := NewNotifier(config); err == nil {
			t.Fatalf("case %d accepted invalid configuration", index)
		}
	}
}

func TestNotifyRejectsInvalidRequestBeforeTransport(t *testing.T) {
	t.Parallel()

	config := testClientConfig(t)
	called := false
	config.GatewayBaseURL = "https://gateway.example"
	config.GatewayCredential = testGatewayCredential
	config.HTTPClient = &http.Client{
		Transport: roundTripFunc(func(*http.Request) (*http.Response, error) {
			called = true

			return nil, context.Canceled
		}),
	}
	notifier, err := NewNotifier(config)
	if err != nil {
		t.Fatal(err)
	}
	request := testEnqueueRequest()
	request.Op = ""
	result := notifier.Notify(t.Context(), request)
	if result.Code != "malformed" || called {
		t.Fatalf("result = %#v, transport called = %v", result, called)
	}
}

func TestRedactDetailRemovesKnownIdentityAndCoordinates(t *testing.T) {
	t.Parallel()

	request := testEnqueueRequest()
	if got := redactDetail(
		"session "+request.HermesSessionID,
		request,
	); strings.Contains(
		got,
		request.HermesSessionID,
	) {
		t.Fatalf("detail contains raw Hermes session ID: %q", got)
	}
	if got := redactDetail(
		"https://private.example/path",
		request,
	); got != "peer detail redacted" {
		t.Fatalf("URL detail was not redacted: %q", got)
	}
}

func mustCanonical(t *testing.T, value any) []byte {
	t.Helper()
	payload, err := EncodeCanonical(value)
	if err != nil {
		t.Fatal(err)
	}

	return payload
}

type roundTripFunc func(*http.Request) (*http.Response, error)

func (function roundTripFunc) RoundTrip(request *http.Request) (*http.Response, error) {
	return function(request)
}
