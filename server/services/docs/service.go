package docs

import (
	"context"
	"database/sql"
	"errors"
	"strings"

	"github.com/a-h/templ"

	"github.com/CoreyCole/vamos/pkg/db"
	"github.com/CoreyCole/vamos/server/layouts/workbench"
)

type Doc struct {
	Path     DocPath
	Title    string
	IsDir    bool
	HTML     templ.Component
	Sections []DocSection
	TOC      []workbench.DocumentTOCItem
}

type DocSection struct {
	ID    string
	Title string
}

type ResolvedWorkspace struct {
	ID       string
	RootPath DocPath
	RelPath  string
	Title    string
	ThreadID string
}

type LoadDocResult struct {
	Doc Doc
}

type WorkspaceResolver interface {
	ResolveWorkspaceForDocPath(
		ctx context.Context,
		arg db.ResolveWorkspaceForDocPathParams,
	) (db.ResolveWorkspaceForDocPathRow, error)
}

type MarkdownLoader interface {
	LoadDoc(ctx context.Context, path DocPath) (Doc, error)
}

type Service struct {
	queries WorkspaceResolver
	loader  MarkdownLoader
}

func NewService(queries WorkspaceResolver, loader MarkdownLoader) *Service {
	return &Service{queries: queries, loader: loader}
}

func (s *Service) LoadDoc(ctx context.Context, docPath DocPath) (LoadDocResult, error) {
	if s == nil || s.loader == nil {
		return LoadDocResult{}, nil
	}
	doc, err := s.loader.LoadDoc(ctx, docPath)
	if err != nil {
		return LoadDocResult{}, err
	}
	return LoadDocResult{Doc: doc}, nil
}

func (s *Service) ResolveWorkspace(
	ctx context.Context,
	userEmail string,
	docPath DocPath,
) (*ResolvedWorkspace, error) {
	if s == nil || s.queries == nil {
		return nil, nil
	}
	row, err := s.queries.ResolveWorkspaceForDocPath(
		ctx,
		db.ResolveWorkspaceForDocPathParams{
			DocPath:   string(docPath),
			UserEmail: strings.TrimSpace(userEmail),
		},
	)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	return &ResolvedWorkspace{
		ID:       row.ID,
		RootPath: DocPath(row.RootDocPath),
		RelPath:  row.RelPath,
		Title:    row.Title,
		ThreadID: row.SelectedThreadID.String,
	}, nil
}
