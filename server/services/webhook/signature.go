package webhook

import (
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"strings"
)

// VerifySignature validates the GitHub webhook signature
// GitHub sends the signature in the X-Hub-Signature-256 header as "sha256=<hex>"
func VerifySignature(payload []byte, signature, secret string) bool {
	if secret == "" {
		return false
	}

	// GitHub signature format: "sha256=<hex>"
	if !strings.HasPrefix(signature, "sha256=") {
		return false
	}

	expectedSig := signature[7:] // Remove "sha256=" prefix

	mac := hmac.New(sha256.New, []byte(secret))
	mac.Write(payload)
	actualSig := hex.EncodeToString(mac.Sum(nil))

	return hmac.Equal([]byte(expectedSig), []byte(actualSig))
}
