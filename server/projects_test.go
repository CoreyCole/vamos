package server

import (
	"path/filepath"
	"testing"
)

func TestDefaultWorkingDirUsesConfiguredDefaultCheckout(t *testing.T) {
	root := t.TempDir()
	projects := ProjectsConfig{
		DefaultRepo:     "vamos",
		DefaultCheckout: "local",
		Repos: map[string]RepoConfig{
			"vamos": {
				Checkouts: map[string]CheckoutConfig{
					"local": {RootPath: filepath.Join(root, "vamos")},
				},
			},
		},
	}

	got, err := DefaultWorkingDir(projects)
	if err != nil {
		t.Fatalf("DefaultWorkingDir() error = %v", err)
	}
	want := filepath.Join(root, "vamos")
	if got != want {
		t.Fatalf("DefaultWorkingDir() = %q, want %q", got, want)
	}
}

func TestBaselineCheckoutUsesRepoAndGlobalDefaults(t *testing.T) {
	root := t.TempDir()
	projects := ProjectsConfig{
		DefaultBaselineCheckout: "main-copy",
		Repos: map[string]RepoConfig{
			"monorepo": {
				DefaultBranch: "develop",
				Checkouts: map[string]CheckoutConfig{
					"main-copy": {RootPath: filepath.Join(root, "monorepo-main")},
				},
			},
		},
	}

	repo, name, checkout, err := BaselineCheckout(projects, "monorepo")
	if err != nil {
		t.Fatalf("BaselineCheckout() error = %v", err)
	}
	if name != "main-copy" {
		t.Fatalf("baseline name = %q, want main-copy", name)
	}
	if checkout.RootPath != filepath.Join(root, "monorepo-main") {
		t.Fatalf("checkout = %+v", checkout)
	}
	if branch := BaselineBranch(repo, checkout); branch != "develop" {
		t.Fatalf("BaselineBranch() = %q, want develop", branch)
	}
}
