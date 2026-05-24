package runtime

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

type ViewportClass string

const (
	ViewportMobile      ViewportClass = "mobile"
	ViewportDesktopHalf ViewportClass = "desktop-half"
	ViewportDesktopFull ViewportClass = "desktop-full"
)

type Config struct {
	RepoRoot     string
	PackageRoot  string
	BaseURL      string
	AuthToken    string
	ArtifactsDir string
	Workspace    WorkspaceIdentity
	Headless     bool
	Viewports    []ViewportClass
}

type RuntimeConfig = Config

type WorkspaceIdentity struct {
	Slug         string
	CheckoutPath string
	DBPath       string
	ManagerURL   string
}

func LoadConfigFromEnv(cwd string) (Config, error) {
	root := findPackageRoot(cwd)
	baseURL := strings.TrimSpace(os.Getenv("VAMOS_BASE_URL"))
	if baseURL == "" {
		baseURL = "http://localhost:4200"
	}

	artifactsDir := strings.TrimSpace(os.Getenv("VAMOS_E2E_ARTIFACTS_DIR"))
	if artifactsDir == "" {
		artifactsDir = filepath.Join(root, ".e2e-runs")
	}

	repoRoot := root

	workspace, err := ReadWorkspaceEnv(repoRoot)
	if err != nil && !os.IsNotExist(err) {
		return Config{}, err
	}

	viewports := []ViewportClass{ViewportDesktopFull}
	if raw := strings.TrimSpace(os.Getenv("VAMOS_E2E_VIEWPORTS")); raw != "" {
		viewports = []ViewportClass{}
		for _, part := range strings.Split(raw, ",") {
			part = strings.TrimSpace(part)
			if part != "" {
				viewports = append(viewports, ViewportClass(part))
			}
		}
	}

	return Config{
		RepoRoot:     repoRoot,
		PackageRoot:  root,
		BaseURL:      strings.TrimRight(baseURL, "/"),
		AuthToken:    strings.TrimSpace(os.Getenv("VAMOS_E2E_AUTH_TOKEN")),
		ArtifactsDir: artifactsDir,
		Workspace:    workspace,
		Headless:     os.Getenv("E2E_HEADLESS") != "false",
		Viewports:    viewports,
	}, nil
}

func findPackageRoot(cwd string) string {
	cur, err := filepath.Abs(cwd)
	if err != nil {
		return cwd
	}
	for {
		if _, err := os.Stat(filepath.Join(cur, "go.mod")); err == nil {
			return cur
		}
		parent := filepath.Dir(cur)
		if parent == cur {
			return cwd
		}
		cur = parent
	}
}

func (c Config) ValidateBrowserConfig() error {
	if strings.TrimSpace(c.BaseURL) == "" {
		return fmt.Errorf("base URL is empty")
	}
	if strings.TrimSpace(c.ArtifactsDir) == "" {
		return fmt.Errorf("artifacts dir is empty")
	}
	return nil
}
