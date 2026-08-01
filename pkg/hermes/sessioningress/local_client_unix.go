//go:build !windows

package sessioningress

import (
	"context"
	"errors"
	"io"
	"net"
	"os"
	"path/filepath"
	"syscall"
	"time"
)

func (n *notifier) notifyLocalAttempt(
	ctx context.Context,
	request NotifyRequest,
) attemptResult {
	euid, err := CurrentEUID()
	if err != nil {
		return localUnsupported()
	}
	path, err := DeriveSocketPath(
		request.Request.HermesSessionID,
		n.config.HermesHome,
		euid,
	)
	if err != nil {
		return attemptResult{NotifyResult: NotifyResult{
			Code:      "malformed",
			Detail:    "invalid local ingress address",
			Transport: TransportLocal,
		}}
	}
	directory := filepath.Dir(path)
	if err := ValidateRuntimeDirectory(directory, euid); err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return localUnsupported()
		}

		return attemptResult{NotifyResult: NotifyResult{
			Code:      "surface_unsupported",
			Detail:    "local ingress directory is unsafe",
			Transport: TransportLocal,
		}}
	}

	capabilityPayload, err := EncodeCanonical(
		CapabilityRequest{Op: "capabilities", Version: ProtocolVersion},
	)
	if err != nil {
		return malformedPeer(TransportLocal)
	}
	capability, outcome := n.localExchange(
		ctx,
		path,
		directory,
		euid,
		capabilityPayload,
		false,
	)
	if outcome != nil {
		return *outcome
	}
	parsedCapability, err := ParseResponse(capability)
	if err != nil {
		return malformedPeer(TransportLocal)
	}
	capabilities, ok := parsedCapability.(CapabilityResponse)
	if rejection, rejected := parsedCapability.(RejectionResponse); rejected {
		result := classifyResponse(rejection, TransportLocal, request.Request)
		result.peerAnswered = true

		return result
	}
	if !ok || !hasExactSessionCapability(capabilities) {
		return attemptResult{NotifyResult: NotifyResult{
			Code:      "surface_unsupported",
			Detail:    "local peer lacks exact-session capability",
			Transport: TransportLocal,
		}, peerAnswered: true}
	}

	responsePayload, outcome := n.localExchange(
		ctx,
		path,
		directory,
		euid,
		request.Bytes,
		true,
	)
	if outcome != nil {
		outcome.localAbsent = false
		outcome.peerAnswered = true

		return *outcome
	}
	response, err := ParseResponse(responsePayload)
	if err != nil {
		result := malformedPeer(TransportLocal)
		result.Uncertain = true
		result.peerAnswered = true

		return result
	}
	result := classifyResponse(response, TransportLocal, request.Request)
	result.peerAnswered = true

	return result
}

func (n *notifier) localExchange(
	ctx context.Context,
	path string,
	directory string,
	euid int,
	payload []byte,
	admissionPossible bool,
) ([]byte, *attemptResult) {
	exchangeCtx, cancel := context.WithTimeout(ctx, n.config.ExchangeTimeout)
	defer cancel()
	dialer := net.Dialer{Timeout: n.config.ConnectTimeout}
	connection, err := dialer.DialContext(exchangeCtx, "unix", path)
	if err != nil {
		if safelyAbsentLocalPeer(err, path, directory, euid) {
			result := localUnsupported()

			return nil, &result
		}
		result := retryableTransport(
			TransportLocal,
			"local connection failed before write",
			false,
		)

		return nil, &result
	}
	defer connection.Close()
	if deadline, ok := exchangeCtx.Deadline(); ok {
		if err := connection.SetDeadline(deadline); err != nil {
			result := retryableTransport(
				TransportLocal,
				"local exchange deadline failed",
				false,
			)

			return nil, &result
		}
	}

	frame, err := EncodeFrame(payload)
	if err != nil {
		result := malformedPeer(TransportLocal)

		return nil, &result
	}
	if err := connection.SetWriteDeadline(
		earliestDeadline(exchangeCtx, n.config.WriteTimeout),
	); err != nil {
		result := retryableTransport(TransportLocal, "local write deadline failed", false)

		return nil, &result
	}
	written, err := writeAll(connection, frame)
	if err != nil {
		result := retryableTransport(
			TransportLocal,
			"local write did not complete",
			admissionPossible && written > 0,
		)

		return nil, &result
	}
	if err := connection.SetReadDeadline(
		earliestDeadline(exchangeCtx, n.config.ReadTimeout),
	); err != nil {
		result := retryableTransport(
			TransportLocal,
			"local read deadline failed",
			admissionPossible,
		)

		return nil, &result
	}
	response, err := ReadFrame(connection)
	if err != nil {
		if isTimeoutOrContext(err, exchangeCtx) {
			result := retryableTransport(
				TransportLocal,
				"local response unavailable after write",
				admissionPossible,
			)

			return nil, &result
		}
		result := malformedPeer(TransportLocal)
		result.Uncertain = admissionPossible

		return nil, &result
	}

	return response, nil
}

func localUnsupported() attemptResult {
	return attemptResult{NotifyResult: NotifyResult{
		Code:      "surface_unsupported",
		Detail:    "local ingress peer is absent",
		Transport: TransportLocal,
	}, localAbsent: true}
}

func safelyAbsentLocalPeer(err error, path, directory string, euid int) bool {
	if errors.Is(err, os.ErrNotExist) {
		return true
	}
	if !errors.Is(err, syscall.ECONNREFUSED) ||
		ValidateRuntimeDirectory(directory, euid) != nil {
		return false
	}
	info, statErr := os.Lstat(path)
	if statErr != nil || info.Mode()&os.ModeSocket == 0 ||
		info.Mode()&os.ModeSymlink != 0 {
		return false
	}
	stat, ok := info.Sys().(*syscall.Stat_t)
	if !ok || euid < 0 || uint64(euid) > uint64(^uint32(0)) {
		return false
	}

	convertedEUID := uint32(euid)

	return stat.Uid == convertedEUID
}

func earliestDeadline(ctx context.Context, timeout time.Duration) time.Time {
	deadline := time.Now().Add(timeout)
	if contextDeadline, ok := ctx.Deadline(); ok && contextDeadline.Before(deadline) {
		return contextDeadline
	}

	return deadline
}

func writeAll(writer io.Writer, payload []byte) (int, error) {
	total := 0
	for len(payload) > 0 {
		written, err := writer.Write(payload)
		total += written
		payload = payload[written:]
		if err != nil {
			return total, err
		}
		if written == 0 {
			return total, io.ErrShortWrite
		}
	}

	return total, nil
}

func isTimeoutOrContext(err error, ctx context.Context) bool {
	var netError net.Error

	return errors.As(err, &netError) && netError.Timeout() || ctx.Err() != nil
}

func hasExactSessionCapability(response CapabilityResponse) bool {
	if len(response.ProtocolVersions) != 1 ||
		response.ProtocolVersions[0] != ProtocolVersion {
		return false
	}
	for _, capability := range response.Capabilities {
		if capability == ExactSessionCapability {
			return true
		}
	}

	return false
}
