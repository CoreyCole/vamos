package workspacecmd

import (
	"encoding/json"
	"net"
	"net/url"
	"strings"
)

type workspaceURLSummary struct {
	URL           string `json:"URL"`
	LowerURL      string `json:"url"`
	WorkspaceSlug string `json:"workspace_slug"`
	Slug          string `json:"Slug"`
	LowerSlug     string `json:"slug"`
}

func workspaceURLFromResponse(responseBody []byte, fallbackSlug, managerURL string) string {
	summary := workspaceURLSummary{}
	if len(responseBody) > 0 {
		_ = json.Unmarshal(responseBody, &summary)
	}
	if u := strings.TrimSpace(firstNonEmpty(summary.URL, summary.LowerURL)); u != "" {
		return ensureTrailingSlash(u)
	}
	slug := strings.TrimSpace(firstNonEmpty(summary.WorkspaceSlug, summary.Slug, summary.LowerSlug, fallbackSlug))
	return inferWorkspaceURL(slug, managerURL)
}

func inferWorkspaceURL(slug, managerURL string) string {
	slug = strings.TrimSpace(slug)
	if slug == "" {
		return ""
	}
	u, err := url.Parse(strings.TrimSpace(managerURL))
	if err != nil || u.Scheme == "" || u.Host == "" {
		return ""
	}
	host := u.Hostname()
	if host == "" || host == "localhost" || net.ParseIP(host) != nil {
		return ""
	}
	if strings.Contains(host, ".") {
		if rest, ok := strings.CutPrefix(host, "main."); ok {
			host = slug + "." + rest
		} else {
			host = slug + "." + host
		}
	} else {
		return ""
	}
	if port := u.Port(); port != "" {
		host = net.JoinHostPort(host, port)
	}
	return u.Scheme + "://" + host + "/"
}

func ensureTrailingSlash(raw string) string {
	if strings.HasSuffix(raw, "/") {
		return raw
	}
	return raw + "/"
}

func firstNonEmpty(values ...string) string {
	for _, value := range values {
		if strings.TrimSpace(value) != "" {
			return value
		}
	}
	return ""
}
