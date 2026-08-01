package sessioningress

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"net/url"
	"path/filepath"
	"strings"
	"time"
)

const (
	TransportNone    = "none"
	TransportLocal   = "local"
	TransportGateway = "gateway"

	initialRetryBackoff = 25 * time.Millisecond
	maxDiagnosticBytes  = 256
)

type NotifyRequest struct {
	Request EnqueueRequest
	Bytes   []byte
}

type NotifyResult struct {
	Admission bool
	Code      string
	Detail    string
	Retryable bool
	Uncertain bool
	Transport string
	Attempts  int
}

type (
	Clock      func() time.Time
	SleepFunc  func(context.Context, time.Duration) error
	JitterFunc func(time.Duration) time.Duration
)

type ClientConfig struct {
	HermesHome        string
	ConnectTimeout    time.Duration
	WriteTimeout      time.Duration
	ReadTimeout       time.Duration
	ExchangeTimeout   time.Duration
	TotalTimeout      time.Duration
	MaxAttempts       int
	BackoffCap        time.Duration
	GatewayBaseURL    string
	GatewayCredential string
	HTTPClient        *http.Client
	Clock             Clock
	Sleep             SleepFunc
	Jitter            JitterFunc
}

type Notifier interface {
	Notify(ctx context.Context, request EnqueueRequest) NotifyResult
}

type notifier struct {
	config     ClientConfig
	gatewayURL *url.URL
}

type attemptResult struct {
	NotifyResult
	retryAfter   time.Duration
	localAbsent  bool
	peerAnswered bool
}

func NewNotifier(config ClientConfig) (Notifier, error) {
	if err := validateClientConfig(&config); err != nil {
		return nil, err
	}
	var gatewayURL *url.URL
	if config.GatewayBaseURL != "" {
		parsed, err := url.Parse(config.GatewayBaseURL)
		if err != nil || (parsed.Scheme != "http" && parsed.Scheme != "https") ||
			parsed.Host == "" || parsed.User != nil || parsed.RawQuery != "" || parsed.Fragment != "" {
			return nil, errors.New(
				"gateway base must be an absolute HTTP(S) URL without credentials, query, or fragment",
			)
		}
		parsed.Path = strings.TrimRight(parsed.Path, "/")
		gatewayURL = parsed
	}

	return &notifier{config: config, gatewayURL: gatewayURL}, nil
}

func validateClientConfig(config *ClientConfig) error {
	if strings.TrimSpace(config.HermesHome) == "" {
		return errors.New("hermes home is required")
	}
	if !filepath.IsAbs(config.HermesHome) {
		return errors.New("hermes home must be absolute")
	}
	for name, value := range map[string]time.Duration{
		"connect timeout":  config.ConnectTimeout,
		"write timeout":    config.WriteTimeout,
		"read timeout":     config.ReadTimeout,
		"exchange timeout": config.ExchangeTimeout,
		"total timeout":    config.TotalTimeout,
		"backoff cap":      config.BackoffCap,
	} {
		if value <= 0 {
			return fmt.Errorf("%s must be positive", name)
		}
	}
	if config.MaxAttempts < 1 || config.MaxAttempts > 32 {
		return errors.New("maximum attempts must be between 1 and 32")
	}
	if (config.GatewayBaseURL == "") != (config.GatewayCredential == "") {
		return errors.New("gateway base and credential must be configured together")
	}
	if strings.ContainsAny(config.GatewayCredential, "\r\n") {
		return errors.New("gateway credential contains invalid characters")
	}
	if config.Clock == nil {
		config.Clock = time.Now
	}
	if config.Sleep == nil {
		config.Sleep = sleepContext
	}
	if config.Jitter == nil {
		config.Jitter = func(delay time.Duration) time.Duration { return delay }
	}
	if config.HTTPClient == nil {
		config.HTTPClient = newBoundedHTTPClient(*config)
	}

	return nil
}

func newNotifyRequest(request EnqueueRequest) (NotifyRequest, error) {
	payload, err := EncodeCanonical(request)
	if err != nil {
		return NotifyRequest{}, err
	}
	parsed, err := ParseRequest(payload)
	if err != nil {
		return NotifyRequest{}, err
	}
	if _, ok := parsed.(EnqueueRequest); !ok {
		return NotifyRequest{}, protocolError("request is not an enqueue operation")
	}

	return NotifyRequest{Request: request, Bytes: append([]byte(nil), payload...)}, nil
}

func (n *notifier) Notify(parent context.Context, request EnqueueRequest) NotifyResult {
	immutable, err := newNotifyRequest(request)
	if err != nil {
		return NotifyResult{
			Code:      "malformed",
			Detail:    "invalid enqueue request",
			Transport: TransportNone,
		}
	}
	ctx, cancel := context.WithTimeout(parent, n.config.TotalTimeout)
	defer cancel()
	budgetDeadline := n.config.Clock().Add(n.config.TotalTimeout)

	transport := TransportLocal
	localPeerAnswered := false
	var last attemptResult
	var everUncertain bool
	for attempt := 1; attempt <= n.config.MaxAttempts; attempt++ {
		if err := ctx.Err(); err != nil {
			return cancellationResult(last, attempt-1)
		}
		if transport == TransportLocal {
			last = n.notifyLocalAttempt(ctx, immutable)
			localPeerAnswered = localPeerAnswered || last.peerAnswered
			if last.localAbsent && localPeerAnswered {
				last = retryableTransport(
					TransportLocal,
					"local peer disappeared before write",
					false,
				)
			} else if last.localAbsent && n.gatewayURL != nil {
				transport = TransportGateway
				last = n.notifyGatewayAttempt(ctx, immutable)
			}
		} else {
			last = n.notifyGatewayAttempt(ctx, immutable)
		}
		everUncertain = everUncertain || last.Uncertain
		last.Attempts = attempt
		if !last.Retryable || last.Admission {
			if !last.Admission {
				last.Uncertain = everUncertain
			}

			return last.NotifyResult
		}
		if attempt == n.config.MaxAttempts {
			last.Uncertain = everUncertain

			return last.NotifyResult
		}
		delay := n.retryDelay(attempt, last.retryAfter)
		remaining := budgetDeadline.Sub(n.config.Clock())
		if remaining <= 0 {
			last.Uncertain = everUncertain

			return cancellationResult(last, attempt)
		}
		if delay > remaining {
			delay = remaining
		}
		if delay <= 0 {
			continue
		}
		if err := n.config.Sleep(ctx, delay); err != nil {
			last.Uncertain = everUncertain

			return cancellationResult(last, attempt)
		}
	}

	return last.NotifyResult
}

func (n *notifier) retryDelay(attempt int, hint time.Duration) time.Duration {
	delay := initialRetryBackoff
	for i := 1; i < attempt && delay < n.config.BackoffCap; i++ {
		delay *= 2
	}
	if hint > delay {
		delay = hint
	}
	if delay > n.config.BackoffCap {
		delay = n.config.BackoffCap
	}
	delay = n.config.Jitter(delay)
	if delay < 0 {
		return 0
	}
	if delay > n.config.BackoffCap {
		return n.config.BackoffCap
	}

	return delay
}

func classifyResponse(
	response Response,
	transport string,
	request EnqueueRequest,
	extraSensitive ...string,
) attemptResult {
	switch value := response.(type) {
	case AcceptedResponse:
		return attemptResult{NotifyResult: NotifyResult{
			Admission: true, Code: value.Code, Transport: transport,
		}}
	case RejectionResponse:
		classification, err := ClassifyResult(value.Code)
		if err != nil {
			return malformedPeer(transport)
		}
		result := attemptResult{NotifyResult: NotifyResult{
			Code:      value.Code,
			Detail:    redactDetail(value.Detail, request, extraSensitive...),
			Retryable: classification == "retryable",
			Transport: transport,
		}}
		if value.RetryAfterMS != nil {
			result.retryAfter = time.Duration(*value.RetryAfterMS) * time.Millisecond
		}

		return result
	default:
		return malformedPeer(transport)
	}
}

func malformedPeer(transport string) attemptResult {
	return attemptResult{NotifyResult: NotifyResult{
		Code: "malformed", Detail: "malformed peer response", Transport: transport,
	}}
}

func retryableTransport(transport, detail string, uncertain bool) attemptResult {
	return attemptResult{NotifyResult: NotifyResult{
		Code: "temporarily_unavailable", Detail: detail, Retryable: true,
		Uncertain: uncertain, Transport: transport,
	}}
}

func cancellationResult(previous attemptResult, attempts int) NotifyResult {
	result := NotifyResult{
		Code: "canceled", Detail: "notification canceled", Retryable: false,
		Uncertain: previous.Uncertain, Transport: previous.Transport, Attempts: attempts,
	}
	if result.Transport == "" {
		result.Transport = TransportNone
	}

	return result
}

func redactDetail(
	detail string,
	request EnqueueRequest,
	extraSensitive ...string,
) string {
	if detail == "" {
		return ""
	}
	sensitiveValues := append(
		[]string{request.HermesSessionID, request.PiSessionID},
		extraSensitive...)
	for _, sensitive := range sensitiveValues {
		if sensitive != "" {
			detail = strings.ReplaceAll(detail, sensitive, "[redacted]")
		}
	}
	if strings.Contains(detail, "://") || strings.Contains(detail, "Bearer ") {
		return "peer detail redacted"
	}
	if len(detail) > maxDiagnosticBytes {
		detail = detail[:maxDiagnosticBytes]
	}

	return detail
}

func sleepContext(ctx context.Context, delay time.Duration) error {
	timer := time.NewTimer(delay)
	defer timer.Stop()
	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-timer.C:
		return nil
	}
}
