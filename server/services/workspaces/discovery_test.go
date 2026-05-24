package workspaces

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func makeCheckout(t *testing.T, parent, name string) string {
	t.Helper()
	checkout := filepath.Join(parent, name)
	if err := os.MkdirAll(checkout, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(
		filepath.Join(checkout, "go.mod"),
		[]byte("module github.com/CoreyCole/vamos\n"),
		0o644,
	); err != nil {
		t.Fatal(err)
	}
	return checkout
}

func TestSlugFromCheckoutName(t *testing.T) {
	cases := map[string]string{
		"vamos":         "main",
		"vamos-foo":     "foo",
		"vamos-Foo_Bar": "foo-bar",
		"vamos-2026-05-07_19-13-13_reorganize-agentchat-templ-files-implement": "2026-05-07-19-13-13-reorganize-agentchat-templ-files-implement",
	}
	for in, want := range cases {
		got, err := SlugFromCheckoutName(in)
		if err != nil {
			t.Fatalf("SlugFromCheckoutName(%q): %v", in, err)
		}
		if got != want {
			t.Fatalf("SlugFromCheckoutName(%q)=%q want %q", in, got, want)
		}
	}
}

func TestSlugFromCheckoutNameRejectsInvalid(t *testing.T) {
	for _, in := range []string{"", "agents", "vamos-!!!", "vamos-_-"} {
		if got, err := SlugFromCheckoutName(in); err == nil {
			t.Fatalf("SlugFromCheckoutName(%q)=%q, nil error", in, got)
		}
	}
}

func TestSlugFromPlanDirName(t *testing.T) {
	cases := map[string]string{
		"2026-05-16_20-48-11_qrspi-auto-mode-workspace-config-ux": "2026-05-16-20-48-11-qrspi-auto-mode-workspace-config-ux",
		"Review Follow_Up": "review-follow-up",
	}
	for in, want := range cases {
		got, err := SlugFromPlanDirName(in)
		if err != nil {
			t.Fatalf("SlugFromPlanDirName(%q): %v", in, err)
		}
		if got != want {
			t.Fatalf("SlugFromPlanDirName(%q)=%q want %q", in, got, want)
		}
	}
}

func TestSlugFromPlanDirNameRejectsInvalid(t *testing.T) {
	for _, in := range []string{"", "!!!", "_-"} {
		if got, err := SlugFromPlanDirName(in); err == nil {
			t.Fatalf("SlugFromPlanDirName(%q)=%q, nil error", in, got)
		}
	}
}

func TestNormalizeWorkspaceSlugTruncatesLongTimestampedNames(t *testing.T) {
	in := "2026-05-17_15-18-02_qrspi-auto-mode-workspace-config-ux-implementation-review"
	got, err := NormalizeWorkspaceSlug(in)
	if err != nil {
		t.Fatal(err)
	}
	if len(got) > maxSlugLength {
		t.Fatalf("len(slug) = %d, want <= %d: %q", len(got), maxSlugLength, got)
	}
	if !strings.HasPrefix(got, "2026-05-17-15-18-02-") {
		t.Fatalf("slug = %q, want timestamp prefix preserved", got)
	}
	lastDash := strings.LastIndex(got, "-")
	if lastDash < 0 || len(got[lastDash+1:]) != slugHashLength {
		t.Fatalf("slug = %q, want %d-char hash suffix", got, slugHashLength)
	}
	again, err := NormalizeWorkspaceSlug(in)
	if err != nil {
		t.Fatal(err)
	}
	if got != again {
		t.Fatalf("slug not stable: %q then %q", got, again)
	}
}

func TestNormalizeWorkspaceSlugHashDistinguishesLongNamesWithSamePrefix(t *testing.T) {
	base := "2026-05-17_15-18-02_qrspi-auto-mode-workspace-config-ux-"
	first, err := NormalizeWorkspaceSlug(base + "implementation-review-alpha")
	if err != nil {
		t.Fatal(err)
	}
	second, err := NormalizeWorkspaceSlug(base + "implementation-review-beta")
	if err != nil {
		t.Fatal(err)
	}
	if first == second {
		t.Fatalf("long slugs collided: %q", first)
	}
	for _, slug := range []string{first, second} {
		if len(slug) > maxSlugLength {
			t.Fatalf("len(slug) = %d, want <= %d: %q", len(slug), maxSlugLength, slug)
		}
		if !strings.HasPrefix(slug, "2026-05-17-15-18-02-") {
			t.Fatalf("slug = %q, want timestamp prefix preserved", slug)
		}
	}
}

func TestDiscoverIncludesConfiguredCheckoutWithStableSlug(t *testing.T) {
	parent := t.TempDir()
	checkout := makeCheckout(t, parent, "vamos")

	got, err := Discover(DiscoveryConfig{
		ParentDir:        parent,
		Domain:           "workspaces.example.test",
		CheckoutPrefixes: []string{"vamos"},
		MainCheckoutName: "vamos-main",
		MainCheckoutPath: filepath.Join(parent, "vamos-main"),
		ConfiguredCheckouts: map[string]ConfiguredCheckout{
			"work": {RootPath: checkout, DisplayName: "Working checkout"},
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 1 {
		t.Fatalf("discovered %d workspace(s): %#v", len(got), got)
	}
	if got[0].Slug != "work" || got[0].CheckoutPath != checkout || got[0].Host != "work.workspaces.example.test" || got[0].IsMain {
		t.Fatalf("workspace = %#v", got[0])
	}
}

func TestHostMapping(t *testing.T) {
	if got := HostForSlug("foo", "cn-agents.test"); got != "foo.cn-agents.test" {
		t.Fatal(got)
	}
	if got := HostForSlug("foo", "CN-AGENTS.TEST."); got != "foo.cn-agents.test" {
		t.Fatal(got)
	}

	slug, ok := SlugFromHost("foo.cn-agents.test:443", "cn-agents.test")
	if !ok || slug != "foo" {
		t.Fatalf("SlugFromHost got %q %v", slug, ok)
	}
	if _, ok := SlugFromHost("foo.bar.cn-agents.test", "cn-agents.test"); ok {
		t.Fatal("nested slug accepted")
	}
	if _, ok := SlugFromHost("foo.other.test", "cn-agents.test"); ok {
		t.Fatal("wrong domain accepted")
	}
}

func TestDiscover(t *testing.T) {
	parent := t.TempDir()
	state := filepath.Join(parent, "state")
	main := makeCheckout(t, parent, "vamos")
	copyPath := makeCheckout(
		t,
		parent,
		"vamos-2026-05-07_19-13-13_reorganize-agentchat-templ-files-implement",
	)
	if err := os.Mkdir(filepath.Join(parent, "vamos-bad"), 0o755); err != nil {
		t.Fatal(err)
	}

	got, err := Discover(DiscoveryConfig{
		ParentDir:        parent,
		Domain:           "cn-agents.test",
		StateDir:         state,
		MainCheckoutPath: main,
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 2 {
		t.Fatalf("len=%d want 2: %#v", len(got), got)
	}
	if !got[0].IsMain || got[0].Slug != "main" {
		t.Fatalf("first workspace = %#v, want main first", got[0])
	}
	if got[0].LogPath != filepath.Join(main, "log", "agents-server.log") {
		t.Fatalf("main log path=%q", got[0].LogPath)
	}

	ws := got[1]
	if ws.Slug != "2026-05-07-19-13-13-reorganize-agentchat-templ-files-implement" {
		t.Fatalf("slug=%q", ws.Slug)
	}
	if ws.CheckoutPath != copyPath {
		t.Fatalf("checkout=%q want %q", ws.CheckoutPath, copyPath)
	}
	if ws.PackagePath != copyPath {
		t.Fatalf("package=%q", ws.PackagePath)
	}
	if ws.Host != "2026-05-07-19-13-13-reorganize-agentchat-templ-files-implement.cn-agents.test" {
		t.Fatalf("host=%q", ws.Host)
	}
	if ws.URL != "https://2026-05-07-19-13-13-reorganize-agentchat-templ-files-implement.cn-agents.test/" {
		t.Fatalf("url=%q", ws.URL)
	}
	if ws.Bundle.Root != filepath.Join(copyPath, ".vamos") {
		t.Fatalf("bundle root=%q", ws.Bundle.Root)
	}
	if ws.LogPath != ws.Bundle.WebLog {
		t.Fatalf("log path=%q want %q", ws.LogPath, ws.Bundle.WebLog)
	}
	if ws.StateDir != ws.Bundle.StateDir {
		t.Fatalf("state=%q want %q", ws.StateDir, ws.Bundle.StateDir)
	}
	if ws.Status != StatusStopped {
		t.Fatalf("status=%q", ws.Status)
	}
	if ws.DiscoveredAt.IsZero() {
		t.Fatal("DiscoveredAt is zero")
	}
	if ws.Stack.Available || ws.Stack.Detail != "" {
		t.Fatalf(
			"Discover should not inspect expensive stack metadata by default: %#v",
			ws.Stack,
		)
	}
}

func TestSlugFromCheckoutNamePreservesTimestampIdentity(t *testing.T) {
	got, err := SlugFromCheckoutName(
		"vamos-2026-05-11_06-13-26_workspace-subdomains-https-caddy-review",
	)
	if err != nil {
		t.Fatal(err)
	}
	want := "2026-05-11-06-13-26-workspace-subdomains-https-caddy-review"
	if got != want {
		t.Fatalf("slug = %q, want %q", got, want)
	}
}

func TestDiscoverDuplicateSlugMarksInvalid(t *testing.T) {
	parent := t.TempDir()
	makeCheckout(t, parent, "vamos-foo")
	makeCheckout(t, parent, "vamos-Foo")

	got, err := Discover(
		DiscoveryConfig{
			ParentDir: parent,
			Domain:    "cn-agents.test",
			StateDir:  filepath.Join(parent, "state"),
		},
	)
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 1 {
		t.Fatalf("len=%d want 1: %#v", len(got), got)
	}
	if got[0].Slug != "foo" || got[0].Status != StatusInvalid ||
		!strings.Contains(got[0].Error, "duplicate") {
		t.Fatalf("duplicate not marked invalid: %#v", got[0])
	}
}
