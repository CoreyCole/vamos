//go:build windows

package sessioningress

import "context"

func (n *notifier) notifyLocalAttempt(context.Context, NotifyRequest) attemptResult {
	return attemptResult{NotifyResult: NotifyResult{
		Code:      "surface_unsupported",
		Detail:    "local ingress is unsupported on this platform",
		Transport: TransportLocal,
	}, localAbsent: true}
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
