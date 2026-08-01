//go:build windows

package hermescmd

import (
	"errors"
	"os"
)

func LoadSettlementEvidence(
	_ *os.File,
	_ SettlementLoadExpectation,
) (SettlementEvidenceV1, error) {
	return SettlementEvidenceV1{}, errors.New(
		"descriptor-safe settlement loading is unsupported on this platform",
	)
}
