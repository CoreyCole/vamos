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

func TestResolveProjectCheckoutUsesRepoDefaultCheckout(t *testing.T) {
	t.Parallel()

	root := t.TempDir()
	projects := ProjectsConfig{
		Repos: map[string]RepoConfig{
			"github.com/coreycole/vamos": {
				DefaultCheckout: "stage",
				Checkouts: map[string]CheckoutConfig{
					"main":  {RootPath: filepath.Join(root, "vamos-main"), Role: CheckoutRoleMain, MustBeClean: true},
					"stage": {RootPath: filepath.Join(root, "vamos")},
				},
			},
		},
	}

	got, err := ResolveProjectCheckout(projects, "github.com/coreycole/vamos")
	if err != nil {
		t.Fatalf("ResolveProjectCheckout() error = %v", err)
	}
	if got.CheckoutName != "stage" || got.RootPath != filepath.Join(root, "vamos") {
		t.Fatalf("resolution = %+v", got)
	}
}

func TestResolveProjectCheckoutFallsBackToGlobalDefaultCheckout(t *testing.T) {
	t.Parallel()

	root := t.TempDir()
	projects := ProjectsConfig{
		DefaultCheckout: "local",
		Repos: map[string]RepoConfig{
			"github.com/coreycole/vamos": {
				Checkouts: map[string]CheckoutConfig{
					"local": {RootPath: filepath.Join(root, "vamos")},
				},
			},
		},
	}

	got, err := ResolveProjectCheckout(projects, "github.com/coreycole/vamos")
	if err != nil {
		t.Fatalf("ResolveProjectCheckout() error = %v", err)
	}
	if got.CheckoutName != "local" {
		t.Fatalf("checkout name = %q, want local", got.CheckoutName)
	}
}

func TestResolveProjectCheckoutRejectsUnknownProject(t *testing.T) {
	t.Parallel()

	_, err := ResolveProjectCheckout(ProjectsConfig{Repos: map[string]RepoConfig{}}, "missing")
	if err == nil {
		t.Fatalf("ResolveProjectCheckout() error = nil, want error")
	}
}

func TestResolveProjectCheckoutRejectsProtectedCheckout(t *testing.T) {
	t.Parallel()

	root := t.TempDir()
	for _, tc := range []struct {
		name     string
		checkout CheckoutConfig
	}{
		{name: "main role", checkout: CheckoutConfig{RootPath: filepath.Join(root, "main"), Role: CheckoutRoleMain}},
		{name: "must clean", checkout: CheckoutConfig{RootPath: filepath.Join(root, "clean"), MustBeClean: true}},
	} {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			projects := ProjectsConfig{
				Repos: map[string]RepoConfig{
					"github.com/coreycole/vamos": {
						DefaultCheckout: "target",
						Checkouts:       map[string]CheckoutConfig{"target": tc.checkout},
					},
				},
			}
			if _, err := ResolveProjectCheckout(projects, "github.com/coreycole/vamos"); err == nil {
				t.Fatalf("ResolveProjectCheckout() error = nil, want error")
			}
		})
	}
}

func TestResolveProjectCheckoutRejectsBaselineCheckout(t *testing.T) {
	t.Parallel()

	root := t.TempDir()
	projects := ProjectsConfig{
		DefaultBaselineCheckout: "baseline",
		Repos: map[string]RepoConfig{
			"github.com/coreycole/vamos": {
				DefaultCheckout: "baseline",
				Checkouts: map[string]CheckoutConfig{
					"baseline": {RootPath: filepath.Join(root, "baseline")},
				},
			},
		},
	}
	if _, err := ResolveProjectCheckout(projects, "github.com/coreycole/vamos"); err == nil {
		t.Fatalf("ResolveProjectCheckout() error = nil, want error")
	}
}

func TestResolveProjectCheckoutDeterministicallyChoosesInteractiveCheckout(t *testing.T) {
	t.Parallel()

	root := t.TempDir()
	projects := ProjectsConfig{
		DefaultBaselineCheckout: "aaa-baseline",
		Repos: map[string]RepoConfig{
			"github.com/coreycole/vamos": {
				Checkouts: map[string]CheckoutConfig{
					"zzz-local":    {RootPath: filepath.Join(root, "local")},
					"aaa-baseline": {RootPath: filepath.Join(root, "baseline")},
					"mmm-stage":    {RootPath: filepath.Join(root, "stage")},
				},
			},
		},
	}

	got, err := ResolveProjectCheckout(projects, "github.com/coreycole/vamos")
	if err != nil {
		t.Fatalf("ResolveProjectCheckout() error = %v", err)
	}
	if got.CheckoutName != "mmm-stage" {
		t.Fatalf("checkout name = %q, want mmm-stage", got.CheckoutName)
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
