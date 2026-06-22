package appletruntime

import (
	"net/http"
	"net/http/httputil"
	"net/url"
	"strings"
)

// NewAppletProxy returns an HTTP reverse proxy for the active applet process.
// Optional prefixes are stripped from request paths before forwarding.
func NewAppletProxy(manager Manager, appID string, stripPrefixes ...string) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		target, ok := manager.ProxyTarget(appID)
		if !ok {
			http.Error(w, "App is starting. Please try again in a moment.", http.StatusBadGateway)
			return
		}
		targetURL, err := url.Parse(target)
		if err != nil {
			http.Error(w, "App is unavailable.", http.StatusBadGateway)
			return
		}

		proxy := httputil.NewSingleHostReverseProxy(targetURL)
		originalDirector := proxy.Director
		proxy.Director = func(req *http.Request) {
			originalDirector(req)
			stripRequestPath(req, stripPrefixes)
		}
		proxy.ServeHTTP(w, r)
	})
}

func stripRequestPath(req *http.Request, prefixes []string) {
	for _, prefix := range prefixes {
		prefix = strings.TrimRight(prefix, "/")
		if prefix == "" || !strings.HasPrefix(req.URL.Path, prefix) {
			continue
		}
		stripped := strings.TrimPrefix(req.URL.Path, prefix)
		if stripped == "" {
			stripped = "/"
		}
		req.URL.Path = stripped
		if req.URL.RawPath != "" {
			raw := strings.TrimPrefix(req.URL.RawPath, prefix)
			if raw == "" {
				raw = "/"
			}
			req.URL.RawPath = raw
		}
		return
	}
}
