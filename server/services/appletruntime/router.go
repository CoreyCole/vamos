package appletruntime

import (
	"fmt"
	"net/http"
	"strings"

	"github.com/CoreyCole/vamos/pkg/collections"
)

type RouteAlias struct {
	Pattern string
	Methods []string
}

type AliasRegistration struct {
	AppID   string
	Aliases []RouteAlias
}

type AppletProxyMatch struct {
	AppID            string
	StripPrefix      string
	Alias            bool
	AliasCookiePaths []string
}

type AliasRegistry struct {
	reserved []string
	aliases  map[string]registeredAlias
}

type registeredAlias struct {
	appID   string
	pattern string
	methods map[string]struct{}
}

func NewAliasRegistry(reserved []string) *AliasRegistry {
	return &AliasRegistry{
		reserved: normalizeReservedPrefixes(reserved),
		aliases:  map[string]registeredAlias{},
	}
}

func (r *AliasRegistry) Register(reg AliasRegistration) error {
	appID := strings.TrimSpace(reg.AppID)
	if appID == "" {
		return fmt.Errorf("app id is required")
	}
	if err := ValidateAliasConflicts(reg.Aliases, r.reserved); err != nil {
		return err
	}
	for _, alias := range reg.Aliases {
		pattern := normalizeAliasPattern(alias.Pattern)
		if existing, ok := r.aliases[pattern]; ok {
			return fmt.Errorf("route alias %q already registered for applet %q", pattern, existing.appID)
		}
		r.aliases[pattern] = registeredAlias{appID: appID, pattern: pattern, methods: methodSet(alias.Methods)}
	}
	return nil
}

func (r *AliasRegistry) Match(req *http.Request) (AppletProxyMatch, bool) {
	if req == nil || req.URL == nil {
		return AppletProxyMatch{}, false
	}
	path := req.URL.Path
	method := strings.ToUpper(strings.TrimSpace(req.Method))
	for pattern, alias := range r.aliases {
		if !aliasMatchesPath(pattern, path) || !aliasMatchesMethod(alias.methods, method) {
			continue
		}
		return AppletProxyMatch{AppID: alias.appID, Alias: true}, true
	}
	return AppletProxyMatch{}, false
}

func RootAliasCookiePaths(aliases []RouteAlias) []string {
	seen := collections.NewSet[string]()
	paths := make([]string, 0, len(aliases))
	for _, alias := range aliases {
		path := aliasCookiePath(alias.Pattern)
		if path == "" || seen.Has(path) {
			continue
		}
		seen.Add(path)
		paths = append(paths, path)
	}
	return paths
}

func aliasCookiePath(pattern string) string {
	pattern = normalizeAliasPattern(pattern)
	if pattern == "" || pattern == "/" || pattern == "/*" {
		return ""
	}
	if strings.HasSuffix(pattern, "/*") {
		return strings.TrimSuffix(pattern, "/*")
	}
	return pattern
}

func ValidateAliasConflicts(aliases []RouteAlias, reserved []string) error {
	seen := map[string]struct{}{}
	reserved = normalizeReservedPrefixes(reserved)
	for _, alias := range aliases {
		pattern := normalizeAliasPattern(alias.Pattern)
		if pattern == "" {
			return fmt.Errorf("route alias pattern is required")
		}
		if !strings.HasPrefix(pattern, "/") {
			return fmt.Errorf("route alias %q must be absolute", pattern)
		}
		if pattern == "/" || pattern == "/*" {
			return fmt.Errorf("route alias %q is too broad", pattern)
		}
		if strings.Contains(pattern, "..") {
			return fmt.Errorf("route alias %q contains unsafe traversal", pattern)
		}
		if strings.Count(pattern, "*") > 1 || (strings.Contains(pattern, "*") && !strings.HasSuffix(pattern, "/*")) {
			return fmt.Errorf("route alias %q has unsupported wildcard placement", pattern)
		}
		if _, ok := seen[pattern]; ok {
			return fmt.Errorf("route alias %q is duplicated", pattern)
		}
		seen[pattern] = struct{}{}
		for _, prefix := range reserved {
			if aliasConflictsWithReserved(pattern, prefix) {
				return fmt.Errorf("route alias %q conflicts with reserved Vamos prefix %q", pattern, prefix)
			}
		}
		for _, method := range alias.Methods {
			if strings.TrimSpace(method) == "" {
				return fmt.Errorf("route alias %q has empty method", pattern)
			}
		}
	}
	return nil
}

func normalizeAliasPattern(pattern string) string {
	pattern = strings.TrimSpace(pattern)
	if pattern != "/" && strings.HasSuffix(pattern, "/") && !strings.HasSuffix(pattern, "/*") {
		pattern = strings.TrimRight(pattern, "/")
	}
	return pattern
}

func normalizeReservedPrefixes(reserved []string) []string {
	out := make([]string, 0, len(reserved))
	for _, prefix := range reserved {
		prefix = strings.TrimRight(strings.TrimSpace(prefix), "/")
		if prefix == "" {
			continue
		}
		if !strings.HasPrefix(prefix, "/") {
			prefix = "/" + prefix
		}
		out = append(out, prefix)
	}
	return out
}

func aliasConflictsWithReserved(pattern, reserved string) bool {
	base := strings.TrimSuffix(pattern, "/*")
	return base == reserved || strings.HasPrefix(base, reserved+"/") || strings.HasPrefix(pattern, reserved+"/*")
}

func aliasMatchesPath(pattern, path string) bool {
	if strings.HasSuffix(pattern, "/*") {
		prefix := strings.TrimSuffix(pattern, "/*")
		return path == prefix || strings.HasPrefix(path, prefix+"/")
	}
	return path == pattern
}

func aliasMatchesMethod(methods map[string]struct{}, method string) bool {
	if len(methods) == 0 {
		return true
	}
	_, ok := methods[method]
	return ok
}

func methodSet(methods []string) map[string]struct{} {
	if len(methods) == 0 {
		return nil
	}
	set := make(map[string]struct{}, len(methods))
	for _, method := range methods {
		method = strings.ToUpper(strings.TrimSpace(method))
		if method != "" {
			set[method] = struct{}{}
		}
	}
	return set
}
