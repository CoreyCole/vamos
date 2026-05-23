package auth

import (
	"net/url"
	"strings"
)

const defaultThoughtsRedirectPath = "/thoughts/"

// NormalizeRedirectURL keeps post-login redirects on real pages instead of transport
// endpoints.
func NormalizeRedirectURL(raw string) string {
	raw = strings.TrimSpace(raw)
	if raw == "" || !strings.HasPrefix(raw, "/") || strings.HasPrefix(raw, "//") {
		return ""
	}

	u, err := url.Parse(raw)
	if err != nil || u.Scheme != "" || u.Host != "" || !strings.HasPrefix(u.Path, "/") ||
		strings.HasPrefix(u.Path, "//") {
		return ""
	}

	if u.Path == "/agent-chat" || strings.HasPrefix(u.Path, "/agent-chat/") {
		u.Path = defaultThoughtsRedirectPath
		u.RawPath = ""
		u.RawQuery = ""
		return u.RequestURI()
	}

	if strings.HasSuffix(u.Path, "/stream") {
		u.Path = strings.TrimSuffix(u.Path, "/stream")
		if u.Path == "" {
			u.Path = "/"
		}
		u.RawPath = ""
	}

	return u.RequestURI()
}
