package docs

import (
	"context"
	"database/sql"
	"errors"
	"path"
	"sort"
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

type WorkspaceDocLister interface {
	ListWorkspaceDocs(ctx context.Context, workspaceID string) ([]db.WorkspaceDoc, error)
}

type MarkdownLoader interface {
	LoadDoc(ctx context.Context, path DocPath) (Doc, error)
}

type Service struct {
	queries WorkspaceResolver
	loader  MarkdownLoader
	docs    WorkspaceDocLister
}

func NewService(queries WorkspaceResolver, loader MarkdownLoader) *Service {
	service := &Service{queries: queries, loader: loader}
	if lister, ok := queries.(WorkspaceDocLister); ok {
		service.docs = lister
	}
	return service
}

func NewServiceWithWorkspaceDocs(
	queries WorkspaceResolver,
	loader MarkdownLoader,
	docs WorkspaceDocLister,
) *Service {
	return &Service{queries: queries, loader: loader, docs: docs}
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

func (s *Service) ListWorkspaceDocTree(
	ctx context.Context,
	workspaceID string,
	current DocPath,
) (*workbench.WorkspaceDocTreeArgs, error) {
	workspaceID = strings.TrimSpace(workspaceID)
	if s == nil || s.docs == nil || workspaceID == "" {
		return nil, nil
	}
	rows, err := s.docs.ListWorkspaceDocs(ctx, workspaceID)
	if err != nil {
		return nil, err
	}
	args := &workbench.WorkspaceDocTreeArgs{
		WorkspaceID: workspaceID,
		CurrentPath: string(current),
		Nodes:       buildWorkspaceDocTreeNodes(rows, string(current)),
	}
	if len(rows) > 0 {
		args.RootPath = rows[0].DocPath
	}
	return args, nil
}

func buildWorkspaceDocTreeNodes(
	rows []db.WorkspaceDoc,
	current string,
) []workbench.WorkspaceDocNode {
	current = strings.Trim(strings.TrimSpace(path.Clean("/"+current)), "/")
	type nodeRef struct {
		node     *workbench.WorkspaceDocNode
		children []string
	}
	refs := make(map[string]*nodeRef, len(rows))
	for _, row := range rows {
		rel := cleanRelPath(row.RelPath)
		if rel == "" {
			continue
		}
		label := strings.TrimSpace(row.Title)
		if label == "" {
			label = path.Base(rel)
			if rel == "." {
				label = path.Base(strings.Trim(row.DocPath, "/"))
			}
		}
		kind := workbench.WorkspaceDocKind(row.Kind)
		if kind != workbench.WorkspaceDocKindDir {
			kind = workbench.WorkspaceDocKindFile
		}
		docPath := strings.Trim(strings.TrimSpace(row.DocPath), "/")
		refs[rel] = &nodeRef{node: &workbench.WorkspaceDocNode{
			Path:     docPath,
			RelPath:  rel,
			Label:    label,
			Kind:     kind,
			IsActive: docPath == current,
		}}
	}
	roots := make([]string, 0)
	for rel, ref := range refs {
		parent := path.Dir(rel)
		if rel == "." {
			roots = append(roots, rel)
			continue
		}
		if parent == "." {
			if refs[parent] != nil {
				refs[parent].children = append(refs[parent].children, rel)
				if ref.node.IsActive {
					refs[parent].node.IsExpanded = true
				}
				continue
			}
			roots = append(roots, rel)
			continue
		}
		if refs[parent] == nil {
			roots = append(roots, rel)
			continue
		}
		refs[parent].children = append(refs[parent].children, rel)
		if ref.node.IsActive {
			refs[parent].node.IsExpanded = true
		}
	}
	var build func(rel string) workbench.WorkspaceDocNode
	build = func(rel string) workbench.WorkspaceDocNode {
		ref := refs[rel]
		sort.Strings(ref.children)
		for _, childRel := range ref.children {
			child := build(childRel)
			if child.IsActive || child.IsExpanded {
				ref.node.IsExpanded = true
			}
			ref.node.Children = append(ref.node.Children, child)
		}
		if len(ref.node.Children) > 0 {
			ref.node.IsExpanded = ref.node.IsExpanded ||
				ref.node.Kind == workbench.WorkspaceDocKindDir
		}
		return *ref.node
	}
	sort.Strings(roots)
	out := make([]workbench.WorkspaceDocNode, 0, len(roots))
	for _, root := range roots {
		out = append(out, build(root))
	}
	return out
}

func cleanRelPath(rel string) string {
	rel = strings.TrimSpace(rel)
	if rel == "" || rel == "." {
		return "."
	}
	return strings.Trim(strings.TrimSpace(path.Clean("/"+rel)), "/")
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
