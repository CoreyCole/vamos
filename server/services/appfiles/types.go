package appfiles

import (
	"context"

	"github.com/a-h/templ"
)

// BrowserConfig scopes file browsing and rendering to one app-owned root.
type BrowserConfig struct {
	Root          string
	HiddenPaths   []string
	RoutePrefix   string
	RendererRoute string
}

type FileNode struct {
	Path        string
	Name        string
	IsDir       bool
	Renderable  bool
	DownloadURL string
}

type FilesViewModel struct {
	Title        string
	CurrentPath  string
	Nodes        []FileNode
	Preview      templ.Component
	ErrorMessage string
}

type RenderRequest struct {
	Root string
	Path string
}

type Service interface {
	List(ctx context.Context, cfg BrowserConfig, relPath string) (FilesViewModel, error)
	Render(ctx context.Context, req RenderRequest) (templ.Component, error)
	SafeOpenPath(root, relPath string) (string, error)
}
