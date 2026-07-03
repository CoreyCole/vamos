package appletruntime

import (
	"context"
	"errors"
	"net/http"
	"net/http/httputil"
	"net/url"
	"strings"

	"github.com/CoreyCole/vamos/pkg/collections"
)

const clientClosedRequestStatus = 499

type ProxyOptions struct {
	FlushSSE            bool
	RewriteCookiePath   bool
	AllowNullOriginCORS bool
}

type CookieRewriteConfig struct {
	ScopedPrefix string
	AliasPaths   []string
}

func NewAppletProxy(manager Manager, match AppletProxyMatch, opts ProxyOptions) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		target, ok := manager.ProxyTarget(match.AppID)
		if !ok {
			http.Error(w, "App is starting. Please try again in a moment.", http.StatusBadGateway)
			return
		}
		targetURL, err := url.Parse(target)
		if err != nil {
			http.Error(w, "App is unavailable.", http.StatusBadGateway)
			return
		}

		manager.Touch(match.AppID, 1)
		defer manager.Touch(match.AppID, -1)

		proxy := httputil.NewSingleHostReverseProxy(targetURL)
		if opts.FlushSSE {
			proxy.FlushInterval = -1
		}
		originalDirector := proxy.Director
		proxy.Director = func(req *http.Request) {
			original := req.Clone(req.Context())
			originalDirector(req)
			if !match.Alias {
				stripRequestPath(req, match.StripPrefix)
			}
			SetForwardedHeaders(req, original, match.StripPrefix)
		}
		proxy.ModifyResponse = func(resp *http.Response) error {
			if opts.RewriteCookiePath && !match.Alias {
				RewriteAppletCookiePaths(resp.Header, CookieRewriteConfig{
					ScopedPrefix: match.StripPrefix,
					AliasPaths:   match.AliasCookiePaths,
				})
			}
			if opts.AllowNullOriginCORS {
				setNullOriginCORS(resp.Header)
			}
			return nil
		}
		proxy.ErrorHandler = func(w http.ResponseWriter, r *http.Request, err error) {
			status := http.StatusBadGateway
			if errors.Is(err, context.Canceled) {
				status = clientClosedRequestStatus
			}
			http.Error(w, "applet backend unavailable", status)
		}
		proxy.ServeHTTP(w, r)
	})
}

func RewriteAppletCookiePaths(header http.Header, cfg CookieRewriteConfig) {
	scopedPrefix := normalizedProxyPrefix(cfg.ScopedPrefix)
	aliasPaths := normalizedCookieAliasPaths(cfg.AliasPaths, scopedPrefix)
	if scopedPrefix == "" && len(aliasPaths) == 0 {
		return
	}
	cookies := header.Values("Set-Cookie")
	if len(cookies) == 0 {
		return
	}
	header.Del("Set-Cookie")
	for _, cookie := range cookies {
		if scopedPrefix != "" {
			header.Add("Set-Cookie", rewriteCookiePath(cookie, scopedPrefix))
		} else {
			header.Add("Set-Cookie", cookie)
		}
		if !cookieHasRootPath(cookie) {
			continue
		}
		for _, aliasPath := range aliasPaths {
			header.Add("Set-Cookie", rewriteCookiePath(cookie, aliasPath))
		}
	}
}

func RewriteCookiePath(header http.Header, scopedPrefix string) {
	RewriteAppletCookiePaths(header, CookieRewriteConfig{ScopedPrefix: scopedPrefix})
}

func normalizedCookieAliasPaths(paths []string, scopedPrefix string) []string {
	seen := collections.NewSet[string]()
	out := make([]string, 0, len(paths))
	for _, path := range paths {
		path = normalizedProxyPrefix(path)
		if path == "" || path == scopedPrefix || seen.Has(path) {
			continue
		}
		seen.Add(path)
		out = append(out, path)
	}
	return out
}

func cookieHasRootPath(cookie string) bool {
	parts := strings.Split(cookie, ";")
	for _, part := range parts[1:] {
		if strings.EqualFold(strings.TrimSpace(part), "Path=/") {
			return true
		}
	}
	return false
}

func SetForwardedHeaders(req *http.Request, original *http.Request, prefix string) {
	if req == nil || original == nil {
		return
	}
	req.Header.Set("X-Forwarded-Host", original.Host)
	req.Header.Set("X-Forwarded-Proto", forwardedProto(original))
	if normalized := normalizedProxyPrefix(prefix); normalized != "" {
		req.Header.Set("X-Forwarded-Prefix", normalized)
	} else {
		req.Header.Del("X-Forwarded-Prefix")
	}
	req.Header.Set("X-Vamos-Applet-Proxy", "1")
}

func stripRequestPath(req *http.Request, prefix string) {
	prefix = normalizedProxyPrefix(prefix)
	if prefix == "" {
		return
	}
	req.URL.Path = stripPathPrefix(req.URL.Path, prefix)
	if req.URL.RawPath != "" {
		req.URL.RawPath = stripPathPrefix(req.URL.RawPath, prefix)
	}
}

func stripPathPrefix(path, prefix string) string {
	if path == prefix {
		return "/"
	}
	if strings.HasPrefix(path, prefix+"/") {
		return strings.TrimPrefix(path, prefix)
	}
	return path
}

func rewriteCookiePath(cookie, scopedPrefix string) string {
	parts := strings.Split(cookie, ";")
	for i, part := range parts[1:] {
		if strings.EqualFold(strings.TrimSpace(part), "Path=/") {
			parts[i+1] = " Path=" + scopedPrefix
			return strings.Join(parts, ";")
		}
	}
	return cookie
}

func setNullOriginCORS(header http.Header) {
	header.Set("Access-Control-Allow-Origin", "null")
	header.Set("Access-Control-Allow-Methods", "GET, POST, PUT, PATCH, DELETE, OPTIONS")
	header.Set("Access-Control-Allow-Headers", "Content-Type, Accept, Datastar-Request")
}

func normalizedProxyPrefix(prefix string) string {
	prefix = strings.TrimRight(strings.TrimSpace(prefix), "/")
	if prefix == "" || prefix == "/" {
		return ""
	}
	if !strings.HasPrefix(prefix, "/") {
		prefix = "/" + prefix
	}
	return prefix
}

func forwardedProto(req *http.Request) string {
	if proto := strings.TrimSpace(req.Header.Get("X-Forwarded-Proto")); proto != "" {
		return proto
	}
	if req.TLS != nil {
		return "https"
	}
	return "http"
}
