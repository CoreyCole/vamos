package workspaces

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"time"
)

const (
	mainWorkspaceSlug       = "main"
	defaultMetadataDirName  = ".vamos"
	defaultCheckoutPrefix   = "vamos"
	defaultMainCheckoutName = "vamos"
	defaultModuleMarker     = "go.mod"
	maxSlugLength           = 63
	slugHashLength          = 8
)

var (
	errInvalidSlug    = errors.New("invalid workspace slug")
	slugRE            = regexp.MustCompile(`^[a-z0-9](?:[a-z0-9-]*[a-z0-9])?$`)
	timestampedSlugRE = regexp.MustCompile(
		`^([0-9]{4}-[0-9]{2}-[0-9]{2}-[0-9]{2}-[0-9]{2}-[0-9]{2})-(.+)$`,
	)
	nonSlugCharRE  = regexp.MustCompile(`[^a-z0-9-]+`)
	repeatedDashRE = regexp.MustCompile(`-+`)
)

func NormalizeDiscoveryConfig(cfg DiscoveryConfig) DiscoveryConfig {
	if strings.TrimSpace(cfg.MetadataDirName) == "" {
		cfg.MetadataDirName = defaultMetadataDirName
	}
	if len(cfg.CheckoutPrefixes) == 0 {
		cfg.CheckoutPrefixes = []string{defaultCheckoutPrefix, "cn-agents"}
	}
	if strings.TrimSpace(cfg.MainCheckoutName) == "" {
		cfg.MainCheckoutName = defaultMainCheckoutName
	}
	if strings.TrimSpace(cfg.ModuleMarker) == "" {
		cfg.ModuleMarker = defaultModuleMarker
	}
	return cfg
}

func Discover(cfg DiscoveryConfig) ([]Workspace, error) {
	cfg = NormalizeDiscoveryConfig(cfg)
	parent, err := discoveryParentDir(cfg)
	if err != nil {
		return nil, err
	}

	entries, err := os.ReadDir(parent)
	if err != nil {
		return nil, err
	}

	discoveredAt := time.Now()
	bySlug := map[string]Workspace{}
	for _, entry := range entries {
		if !entry.IsDir() || !hasConfiguredPrefix(entry.Name(), cfg.CheckoutPrefixes) {
			continue
		}

		slug, err := SlugFromCheckoutNameWithConfig(entry.Name(), cfg)
		if err != nil {
			continue
		}

		checkoutPath := filepath.Join(parent, entry.Name())
		packagePath := packagePath(checkoutPath, cfg)
		if !IsValidCheckout(checkoutPath, cfg) {
			continue
		}

		if existing, exists := bySlug[slug]; exists {
			existing.Status = StatusInvalid
			existing.Error = fmt.Sprintf("duplicate workspace slug %q", slug)
			bySlug[slug] = existing
			continue
		}

		branch, commit := gitSummary(context.Background(), checkoutPath)
		bundle := RuntimePaths(checkoutPath, cfg.MetadataDirName)
		isMain := samePath(checkoutPath, cfg.MainCheckoutPath) ||
			slug == mainWorkspaceSlug
		logPath := bundle.WebLog
		if isMain {
			logPath = filepath.Join(checkoutPath, "log", "agents-server.log")
		}
		ws := Workspace{
			Slug:            slug,
			DisplayName:     displayNameForSlug(slug),
			CheckoutPath:    checkoutPath,
			PackagePath:     packagePath,
			MetadataDirName: cfg.MetadataDirName,
			Host:            HostForSlug(slug, cfg.Domain),
			URL:             WorkspaceURL(slug, cfg.Domain),
			Status:          StatusStopped,
			Bundle:          bundle,
			Ports:           map[BundleComponent]int{},
			PIDs:            map[BundleComponent]int{},
			LogPath:         logPath,
			StateDir:        bundle.StateDir,
			IsMain:          isMain,
			DiscoveredAt:    discoveredAt,
			Branch:          branch,
			Commit:          commit,
		}
		bySlug[slug] = ws
	}

	for slug, configured := range cfg.ConfiguredCheckouts {
		ws := workspaceFromConfiguredCheckout(slug, configured, cfg, discoveredAt)
		if ws.Error != "" {
			bySlug[ws.Slug] = ws
			continue
		}
		if existing, exists := bySlug[ws.Slug]; exists && !samePath(existing.CheckoutPath, ws.CheckoutPath) {
			existing.Status = StatusInvalid
			existing.Error = fmt.Sprintf("duplicate workspace slug %q", ws.Slug)
			bySlug[ws.Slug] = existing
			continue
		}
		bySlug[ws.Slug] = ws
	}

	out := make([]Workspace, 0, len(bySlug))
	for _, ws := range bySlug {
		out = append(out, ws)
	}
	sortWorkspaces(out)
	return out, nil
}

func workspaceFromConfiguredCheckout(
	slug string,
	configured ConfiguredCheckout,
	cfg DiscoveryConfig,
	discoveredAt time.Time,
) Workspace {
	normalizedSlug, err := NormalizeWorkspaceSlug(slug)
	if err != nil {
		return Workspace{Slug: slug, Status: StatusInvalid, Error: err.Error()}
	}
	checkoutPath := strings.TrimSpace(configured.RootPath)
	if checkoutPath == "" {
		return Workspace{Slug: normalizedSlug, Status: StatusInvalid, Error: "configured checkout root_path is required"}
	}
	packagePath := packagePath(checkoutPath, cfg)
	bundle := RuntimePaths(checkoutPath, cfg.MetadataDirName)
	branch, commit := gitSummary(context.Background(), checkoutPath)
	isMain := configured.IsMain || samePath(checkoutPath, cfg.MainCheckoutPath) || normalizedSlug == mainWorkspaceSlug
	logPath := bundle.WebLog
	if isMain {
		logPath = filepath.Join(checkoutPath, "log", "agents-server.log")
	}
	ws := Workspace{
		Slug:            normalizedSlug,
		DisplayName:     firstNonEmptyString(configured.DisplayName, displayNameForSlug(normalizedSlug)),
		CheckoutPath:    checkoutPath,
		PackagePath:     packagePath,
		MetadataDirName: cfg.MetadataDirName,
		Host:            HostForSlug(normalizedSlug, cfg.Domain),
		URL:             WorkspaceURL(normalizedSlug, cfg.Domain),
		Status:          StatusStopped,
		Bundle:          bundle,
		Ports:           map[BundleComponent]int{},
		PIDs:            map[BundleComponent]int{},
		LogPath:         logPath,
		StateDir:        bundle.StateDir,
		IsMain:          isMain,
		DiscoveredAt:    discoveredAt,
		Branch:          branch,
		Commit:          commit,
	}
	if !IsValidCheckout(checkoutPath, cfg) {
		ws.Status = StatusInvalid
		ws.Error = fmt.Sprintf("configured checkout %q is not a valid checkout", checkoutPath)
	}
	return ws
}

func discoveryParentDir(cfg DiscoveryConfig) (string, error) {
	parent := strings.TrimSpace(cfg.ParentDir)
	if parent == "" && strings.TrimSpace(cfg.MainCheckoutPath) != "" {
		parent = filepath.Dir(filepath.Clean(cfg.MainCheckoutPath))
	}
	if parent == "" || parent == "." {
		cwd, err := os.Getwd()
		if err != nil {
			return "", err
		}
		parent = filepath.Dir(cwd)
	}
	return parent, nil
}

func sortWorkspaces(workspaces []Workspace) {
	sort.Slice(workspaces, func(i, j int) bool {
		if workspaces[i].IsMain != workspaces[j].IsMain {
			return workspaces[i].IsMain
		}
		return workspaces[i].Slug < workspaces[j].Slug
	})
}

func hasConfiguredPrefix(name string, prefixes []string) bool {
	name = strings.TrimSpace(name)
	for _, prefix := range prefixes {
		prefix = strings.TrimSpace(prefix)
		if prefix != "" && (name == prefix || strings.HasPrefix(name, prefix+"-")) {
			return true
		}
	}
	return false
}

func packagePath(checkoutPath string, cfg DiscoveryConfig) string {
	if strings.TrimSpace(cfg.PackageSubdir) != "" {
		return filepath.Join(checkoutPath, filepath.FromSlash(cfg.PackageSubdir))
	}
	marker := strings.TrimSpace(cfg.ModuleMarker)
	if marker == "" {
		marker = defaultModuleMarker
	}
	if st, err := os.Stat(
		filepath.Join(checkoutPath, filepath.FromSlash(marker)),
	); err == nil &&
		!st.IsDir() {
		return checkoutPath
	}
	legacyPackagePath := filepath.Join(checkoutPath, "pkg", "agents")
	if st, err := os.Stat(
		filepath.Join(legacyPackagePath, "go.mod"),
	); err == nil &&
		!st.IsDir() {
		return legacyPackagePath
	}
	return checkoutPath
}

func IsValidCheckout(checkoutPath string, cfg DiscoveryConfig) bool {
	cfg = NormalizeDiscoveryConfig(cfg)
	st, err := os.Stat(
		filepath.Join(
			packagePath(checkoutPath, cfg),
			filepath.FromSlash(cfg.ModuleMarker),
		),
	)
	if err == nil && !st.IsDir() {
		return true
	}
	if strings.TrimSpace(cfg.PackageSubdir) == "" {
		legacy, legacyErr := os.Stat(
			filepath.Join(checkoutPath, "pkg", "agents", "go.mod"),
		)
		return legacyErr == nil && !legacy.IsDir()
	}
	return false
}

func SlugFromCheckoutName(name string) (string, error) {
	return SlugFromCheckoutNameWithConfig(name, DiscoveryConfig{})
}

func SlugFromCheckoutNameWithConfig(name string, cfg DiscoveryConfig) (string, error) {
	cfg = NormalizeDiscoveryConfig(cfg)
	name = strings.TrimSpace(name)
	switch name {
	case cfg.MainCheckoutName, "cn-agents":
		return mainWorkspaceSlug, nil
	case "":
		return "", errInvalidSlug
	}

	for _, prefix := range cfg.CheckoutPrefixes {
		prefix = strings.TrimSpace(prefix)
		if prefix == "" || !strings.HasPrefix(name, prefix+"-") {
			continue
		}
		return NormalizeWorkspaceSlug(strings.TrimPrefix(name, prefix+"-"))
	}
	return "", errInvalidSlug
}

func SlugFromPlanDirName(name string) (string, error) {
	name = strings.TrimSpace(name)
	if name == "" {
		return "", errInvalidSlug
	}
	return NormalizeWorkspaceSlug(name)
}

func NormalizeWorkspaceSlug(raw string) (string, error) {
	slug := strings.ToLower(strings.TrimSpace(raw))
	slug = strings.ReplaceAll(slug, "_", "-")
	slug = nonSlugCharRE.ReplaceAllString(slug, "-")
	slug = repeatedDashRE.ReplaceAllString(slug, "-")
	slug = strings.Trim(slug, "-")
	slug = truncateWorkspaceSlug(slug)
	if err := ValidateSlug(slug); err != nil {
		return "", err
	}
	return slug, nil
}

func truncateWorkspaceSlug(slug string) string {
	if len(slug) <= maxSlugLength {
		return slug
	}
	hash := workspaceSlugHash(slug)
	suffixLength := 1 + slugHashLength
	if matches := timestampedSlugRE.FindStringSubmatch(slug); len(matches) == 3 {
		prefix, title := matches[1], matches[2]
		maxTitleLength := maxSlugLength - len(prefix) - 1 - suffixLength
		if maxTitleLength > 0 {
			title = strings.Trim(title[:min(len(title), maxTitleLength)], "-")
			if title != "" {
				return prefix + "-" + title + "-" + hash
			}
		}
	}
	return strings.Trim(slug[:maxSlugLength-suffixLength], "-") + "-" + hash
}

func workspaceSlugHash(slug string) string {
	sum := sha256.Sum256([]byte(slug))
	return hex.EncodeToString(sum[:])[:slugHashLength]
}

func ValidateSlug(slug string) error {
	if len(slug) > maxSlugLength || !slugRE.MatchString(slug) {
		return fmt.Errorf("%w: %q", errInvalidSlug, slug)
	}
	return nil
}

func HostForSlug(slug, domain string) string {
	slug = strings.TrimSpace(slug)
	domain = strings.Trim(strings.ToLower(strings.TrimSpace(domain)), ".")
	if domain == "" || slug == "" {
		return ""
	}
	return slug + "." + domain
}

func SlugFromHost(host, domain string) (string, bool) {
	h := strings.ToLower(strings.TrimSpace(host))
	if i := strings.Index(h, ":"); i >= 0 {
		h = h[:i]
	}
	h = strings.TrimSuffix(h, ".")
	d := strings.Trim(strings.ToLower(strings.TrimSpace(domain)), ".")
	if d == "" {
		return "", false
	}
	suffix := "." + d
	if !strings.HasSuffix(h, suffix) {
		return "", false
	}
	slug := strings.TrimSuffix(h, suffix)
	if strings.Contains(slug, ".") || ValidateSlug(slug) != nil {
		return "", false
	}
	return slug, true
}

func WorkspaceURL(slug, domain string) string {
	host := HostForSlug(slug, domain)
	if host == "" {
		return ""
	}
	return "https://" + host + "/"
}

func samePath(a, b string) bool {
	if a == "" || b == "" {
		return false
	}
	aa, errA := filepath.Abs(a)
	bb, errB := filepath.Abs(b)
	if errA != nil || errB != nil {
		return filepath.Clean(a) == filepath.Clean(b)
	}
	return filepath.Clean(aa) == filepath.Clean(bb)
}

func displayNameForSlug(slug string) string {
	if slug == mainWorkspaceSlug {
		return mainWorkspaceSlug
	}
	return strings.ReplaceAll(slug, "-", " ")
}

func firstNonEmptyString(values ...string) string {
	for _, value := range values {
		if strings.TrimSpace(value) != "" {
			return value
		}
	}
	return ""
}

func gitSummary(ctx context.Context, checkoutPath string) (branch, commit string) {
	ctx, cancel := context.WithTimeout(ctx, 200*time.Millisecond)
	defer cancel()

	branch = runGit(ctx, checkoutPath, "branch", "--show-current")
	commit = runGit(ctx, checkoutPath, "rev-parse", "--short", "HEAD")
	return branch, commit
}

func runGit(ctx context.Context, checkoutPath string, args ...string) string {
	cmd := exec.CommandContext(
		ctx,
		"git",
		append([]string{"-C", checkoutPath}, args...)...)
	out, err := cmd.Output()
	if err != nil {
		return ""
	}
	return strings.TrimSpace(string(out))
}
