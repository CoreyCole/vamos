package agentbrowser

import (
	"errors"
	"net/url"
	"strings"
)

func ValidateRedirectPath(path string) (string, error) {
	path = strings.TrimSpace(path)
	if path == "" {
		return "/", nil
	}
	if strings.HasPrefix(path, "//") {
		return "", errors.New("redirect must be local path")
	}
	u, err := url.Parse(path)
	if err != nil {
		return "", errors.New("invalid redirect")
	}
	if u.IsAbs() || u.Host != "" {
		return "", errors.New("redirect must be local path")
	}
	if !strings.HasPrefix(u.Path, "/") {
		return "", errors.New("redirect must start with /")
	}
	if strings.HasPrefix(u.Path, "/internal/agent-auth") || strings.HasPrefix(u.Path, "/internal/dev-auth") {
		return "", errors.New("redirect to auth endpoint not allowed")
	}
	return u.RequestURI(), nil
}
