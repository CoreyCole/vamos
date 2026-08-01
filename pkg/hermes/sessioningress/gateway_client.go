package sessioningress

import (
	"bytes"
	"context"
	"crypto/tls"
	"io"
	"mime"
	"net"
	"net/http"
	"net/http/httptrace"
	"strings"
)

const (
	capabilitiesPath = "/vamos/session-ingress/v1/capabilities"
	enqueuePath      = "/vamos/session-ingress/v1/enqueue"
)

func newBoundedHTTPClient(config ClientConfig) *http.Client {
	baseTransport, ok := http.DefaultTransport.(*http.Transport)
	if !ok {
		baseTransport = &http.Transport{}
	}
	transport := baseTransport.Clone()
	transport.DialContext = (&net.Dialer{Timeout: config.ConnectTimeout}).DialContext
	transport.ResponseHeaderTimeout = config.ReadTimeout
	transport.TLSHandshakeTimeout = config.ConnectTimeout
	transport.ExpectContinueTimeout = config.WriteTimeout
	transport.TLSClientConfig = &tls.Config{MinVersion: tls.VersionTLS12}

	return &http.Client{Transport: transport}
}

func (n *notifier) notifyGatewayAttempt(
	ctx context.Context,
	request NotifyRequest,
) attemptResult {
	capabilityPayload, err := EncodeCanonical(
		CapabilityRequest{Op: "capabilities", Version: ProtocolVersion},
	)
	if err != nil {
		return malformedPeer(TransportGateway)
	}
	capabilityResponse, outcome := n.gatewayExchange(
		ctx,
		capabilitiesPath,
		capabilityPayload,
		false,
		request.Request,
	)
	if outcome != nil {
		return *outcome
	}
	capabilities, ok := capabilityResponse.(CapabilityResponse)
	if !ok || !hasExactSessionCapability(capabilities) {
		return attemptResult{NotifyResult: NotifyResult{
			Code:      "surface_unsupported",
			Detail:    "gateway lacks exact-session capability",
			Transport: TransportGateway,
		}, peerAnswered: true}
	}

	response, outcome := n.gatewayExchange(
		ctx,
		enqueuePath,
		request.Bytes,
		true,
		request.Request,
	)
	if outcome != nil {
		return *outcome
	}
	result := classifyResponse(
		response,
		TransportGateway,
		request.Request,
		n.config.GatewayCredential,
		n.config.GatewayBaseURL,
	)
	result.peerAnswered = true

	return result
}

func (n *notifier) gatewayExchange(
	ctx context.Context,
	path string,
	payload []byte,
	admissionPossible bool,
	enqueue EnqueueRequest,
) (Response, *attemptResult) {
	exchangeCtx, cancel := context.WithTimeout(ctx, n.config.ExchangeTimeout)
	defer cancel()
	endpoint := *n.gatewayURL
	endpoint.Path = strings.TrimRight(endpoint.Path, "/") + path
	endpoint.RawPath = ""

	wrote := false
	trace := &httptrace.ClientTrace{
		WroteHeaders: func() { wrote = true },
		WroteRequest: func(httptrace.WroteRequestInfo) { wrote = true },
	}
	exchangeCtx = httptrace.WithClientTrace(exchangeCtx, trace)
	httpRequest, err := http.NewRequestWithContext(
		exchangeCtx,
		http.MethodPost,
		endpoint.String(),
		bytes.NewReader(payload),
	)
	if err != nil {
		result := malformedPeer(TransportGateway)

		return nil, &result
	}
	httpRequest.Header.Set("Authorization", "Bearer "+n.config.GatewayCredential)
	httpRequest.Header.Set("Content-Type", "application/json")
	httpRequest.Header.Set("Accept", "application/json")

	httpResponse, err := n.config.HTTPClient.Do(httpRequest)
	if err != nil {
		uncertain := admissionPossible && wrote
		result := retryableTransport(
			TransportGateway,
			gatewayFailureDetail(wrote),
			uncertain,
		)

		return nil, &result
	}
	defer httpResponse.Body.Close()
	if httpResponse.ContentLength > MaxFrameBytes {
		result := malformedPeer(TransportGateway)
		result.Uncertain = admissionPossible

		return nil, &result
	}
	mediaType, _, err := mime.ParseMediaType(httpResponse.Header.Get("Content-Type"))
	if err != nil || mediaType != "application/json" {
		result := malformedPeer(TransportGateway)
		result.Uncertain = admissionPossible

		return nil, &result
	}
	body, err := io.ReadAll(io.LimitReader(httpResponse.Body, MaxFrameBytes+1))
	if err != nil {
		result := retryableTransport(
			TransportGateway,
			"gateway response unavailable after write",
			admissionPossible,
		)

		return nil, &result
	}
	if len(body) > MaxFrameBytes {
		result := malformedPeer(TransportGateway)
		result.Uncertain = admissionPossible

		return nil, &result
	}
	response, err := ParseResponse(body)
	if err != nil {
		result := malformedPeer(TransportGateway)
		result.Uncertain = admissionPossible

		return nil, &result
	}
	code := responseCode(response)
	if ValidateHTTPStatus(code, httpResponse.StatusCode) != nil {
		result := malformedPeer(TransportGateway)
		result.Uncertain = admissionPossible

		return nil, &result
	}
	if rejection, ok := response.(RejectionResponse); ok {
		result := classifyResponse(
			rejection,
			TransportGateway,
			enqueue,
			n.config.GatewayCredential,
			n.config.GatewayBaseURL,
		)
		result.peerAnswered = true

		return response, &result
	}

	return response, nil
}

func responseCode(response Response) string {
	switch value := response.(type) {
	case CapabilityResponse:
		return value.Code
	case AcceptedResponse:
		return value.Code
	case RejectionResponse:
		return value.Code
	default:
		return ""
	}
}

func gatewayFailureDetail(wrote bool) string {
	if wrote {
		return "gateway exchange failed after possible write"
	}

	return "gateway connection failed before write"
}
