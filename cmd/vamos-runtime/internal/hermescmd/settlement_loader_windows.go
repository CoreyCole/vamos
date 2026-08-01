//go:build windows

package hermescmd

import (
	"errors"
	"os"
)

func currentOwnerUID() int { return -1 }

func requestGracefulTermination(process ManagedCommand) error {
	return process.Kill()
}

func LoadSettlementEvidence(
	_ *os.File,
	_ SettlementLoadExpectation,
) (SettlementEvidenceV1, error) {
	return SettlementEvidenceV1{}, errors.New(
		"descriptor-safe settlement loading is unsupported on this platform",
	)
}

func LoadExactSettlementEvidence(
	_ *os.File,
	_, _ string,
	_ int,
) (SettlementEvidenceV1, error) {
	return SettlementEvidenceV1{}, errors.New(
		"descriptor-safe settlement loading is unsupported on this platform",
	)
}
