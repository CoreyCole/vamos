package agentbrowser

import "time"

type Purpose string

const (
	PurposeE2EPlaywright Purpose = "e2e_playwright"
	PurposeHermesChat    Purpose = "hermes_chat"
	PurposeVerify        Purpose = "verify"
)

type Claims struct {
	Purpose      Purpose `json:"purpose"`
	Email        string  `json:"email"`
	TargetSlug   string  `json:"target_slug"`
	RedirectPath string  `json:"redirect_path"`
	ExpiresAt    int64   `json:"expires_at"`
	JTI          string  `json:"jti"`
	KeyID        string  `json:"key_id"`
}

func (c Claims) ExpiresAtTime() time.Time {
	return time.Unix(c.ExpiresAt, 0)
}
