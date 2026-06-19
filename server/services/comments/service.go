package comments

import (
	"context"
	"database/sql"
	"fmt"
	"log"
	"os"
	"path/filepath"
	"strconv"
	"strings"

	"github.com/gofrs/uuid"

	"github.com/CoreyCole/vamos/pkg/db"
)

type CommentScopeKind string

const (
	CommentScopeSingleFile CommentScopeKind = "single_file"
	CommentScopeQRSPIRoot  CommentScopeKind = "qrspi_root"
)

type CommentScope struct {
	Kind        CommentScopeKind
	RootDocPath string
	CurrentPath string
}

type Service struct {
	queries          *db.Queries
	gitCommit        string
	githubBaseURL    string
	markdownBasePath string
}

// NewService creates a comment service with cached git commit.
func NewService(
	database *sql.DB,
	gitCommit, githubBaseURL string,
	markdownBasePath ...string,
) *Service {
	basePath := ""
	if len(markdownBasePath) > 0 {
		basePath = markdownBasePath[0]
	}
	return &Service{
		queries:          db.New(database),
		gitCommit:        gitCommit,
		githubBaseURL:    githubBaseURL,
		markdownBasePath: basePath,
	}
}

// createCommentInternal creates a new document-scoped comment.
func (s *Service) createCommentInternal(
	ctx context.Context,
	userEmail string,
	req CreateCommentRequest,
) (*db.WorkspaceDocComment, error) {
	documentPath, err := canonicalThoughtsPath(req.FilePath)
	if err != nil {
		return nil, err
	}
	log.Printf("[DEBUG] Creating comment with document path: %q", documentPath)

	workspaceRoot, _ := s.inferWorkspaceRoot(documentPath)

	comment, err := s.queries.CreateDocumentComment(
		ctx,
		db.CreateDocumentCommentParams{
			ID:            uuid.Must(uuid.NewV4()).String(),
			WorkspaceRoot: workspaceRoot,
			DocPath:       documentPath,
			UserEmail:     strings.TrimSpace(userEmail),
			CommentText:   strings.TrimSpace(req.CommentText),
			SelectedText:  strings.TrimSpace(req.SelectedText),
			SectionHint: sql.NullString{
				String: req.SectionID,
				Valid:  req.SectionID != "",
			},
			HeadingHint: sql.NullString{
				String: strings.TrimSpace(req.HeadingHint),
				Valid:  strings.TrimSpace(req.HeadingHint) != "",
			},
			StartLine:   int64(req.StartLine),
			StartColumn: int64(req.StartColumn),
			EndLine:     int64(req.EndLine),
			EndColumn:   int64(req.EndColumn),
		},
	)
	if err != nil {
		return nil, fmt.Errorf("failed to create comment: %w", err)
	}

	log.Printf(
		"[DEBUG] Created comment %s for document path: %q",
		comment.ID,
		comment.DocPath,
	)
	return &comment, nil
}

func (s *Service) createReplyInternal(
	ctx context.Context,
	userEmail string,
	req CreateReplyRequest,
) (*db.WorkspaceDocCommentReply, error) {
	if _, err := s.queries.GetDocumentComment(ctx, req.CommentID); err != nil {
		return nil, fmt.Errorf("comment not found: %w", err)
	}
	reply, err := s.queries.CreateDocumentCommentReply(
		ctx,
		db.CreateDocumentCommentReplyParams{
			ID:        uuid.Must(uuid.NewV4()).String(),
			CommentID: req.CommentID,
			UserEmail: strings.TrimSpace(userEmail),
			ActorType: "user",
			ReplyText: strings.TrimSpace(req.ReplyText),
		},
	)
	if err != nil {
		return nil, fmt.Errorf("failed to create reply: %w", err)
	}
	return &reply, nil
}

// GetCommentsForFileInternal retrieves all comments for a canonical thoughts/...
// document.
func (s *Service) GetDocumentComment(
	ctx context.Context,
	commentID string,
) (db.WorkspaceDocComment, error) {
	commentID = strings.TrimSpace(commentID)
	if commentID == "" {
		return db.WorkspaceDocComment{}, fmt.Errorf("comment_id is required")
	}
	comment, err := s.queries.GetDocumentComment(ctx, commentID)
	if err != nil {
		return db.WorkspaceDocComment{}, fmt.Errorf("comment not found: %w", err)
	}
	return comment, nil
}

func (s *Service) GetCommentsForFileInternal(
	ctx context.Context,
	filePath string,
) (*GetCommentsResponse, error) {
	documentPath, err := canonicalThoughtsPath(filePath)
	if err != nil {
		return nil, err
	}
	log.Printf("[DEBUG] Looking up comments for document path: %q", documentPath)

	comments, err := s.queries.ListDocumentComments(
		ctx,
		db.ListDocumentCommentsParams{
			DocPath:         documentPath,
			IncludeResolved: int64(0),
		},
	)
	if err != nil {
		return nil, fmt.Errorf("failed to get comments: %w", err)
	}

	return s.commentsResponseFromRows(ctx, documentPath, comments)
}

func (s *Service) ResolveCommentScope(
	_ context.Context,
	filePath string,
) (CommentScope, error) {
	documentPath, err := canonicalThoughtsPath(filePath)
	if err != nil {
		return CommentScope{}, err
	}
	root, ok := s.inferWorkspaceRoot(documentPath)
	if !ok {
		return CommentScope{Kind: CommentScopeSingleFile, CurrentPath: documentPath}, nil
	}
	return CommentScope{
		Kind:        CommentScopeQRSPIRoot,
		RootDocPath: root,
		CurrentPath: documentPath,
	}, nil
}

func (s *Service) sectionTitlesForFile(filePath string) map[string]string {
	titles := map[string]string{}
	documentPath, err := canonicalThoughtsPath(filePath)
	if err != nil || strings.TrimSpace(s.markdownBasePath) == "" {
		return titles
	}
	cleanDoc := strings.Trim(
		strings.TrimPrefix(strings.Trim(documentPath, "/"), "thoughts/"),
		"/",
	)
	base, err := filepath.Abs(s.markdownBasePath)
	if err != nil {
		return titles
	}
	abs := filepath.Join(base, filepath.FromSlash(cleanDoc))
	rel, err := filepath.Rel(base, abs)
	if err != nil || rel == "." || strings.HasPrefix(rel, "..") {
		return titles
	}
	data, err := os.ReadFile(abs)
	if err != nil {
		return titles
	}
	return markdownSectionTitles(data)
}

func markdownSectionTitles(data []byte) map[string]string {
	titles := map[string]string{}
	lines := strings.Split(string(data), "\n")
	inFence := false
	inFrontmatter := false
	if len(lines) > 0 && strings.TrimSpace(lines[0]) == "---" {
		inFrontmatter = true
	}
	sectionID := 0
	contentBeforeHeading := false
	for i, line := range lines {
		trimmed := strings.TrimSpace(line)
		if inFrontmatter {
			if i > 0 && trimmed == "---" {
				inFrontmatter = false
			}
			continue
		}
		if strings.HasPrefix(trimmed, "```") || strings.HasPrefix(trimmed, "~~~") {
			inFence = !inFence
			continue
		}
		if inFence || trimmed == "" {
			continue
		}
		if title, ok := markdownHeadingTitle(trimmed); ok {
			if contentBeforeHeading {
				sectionID++
				contentBeforeHeading = false
			}
			titles["section-"+strconv.Itoa(sectionID)] = title
			sectionID++
			continue
		}
		if len(titles) == 0 {
			contentBeforeHeading = true
		}
	}
	return titles
}

func markdownHeadingTitle(trimmed string) (string, bool) {
	level := 0
	for level < len(trimmed) && trimmed[level] == '#' {
		level++
	}
	if level == 0 || level > 6 || level >= len(trimmed) || trimmed[level] != ' ' {
		return "", false
	}
	title := strings.TrimSpace(trimmed[level:])
	title = strings.TrimSpace(strings.TrimRight(title, "#"))
	title = strings.Trim(title, "`*_ ")
	return title, title != ""
}

func (s *Service) inferWorkspaceRoot(documentPath string) (string, bool) {
	if strings.TrimSpace(s.markdownBasePath) == "" {
		return "", false
	}
	cleanDoc := strings.Trim(
		strings.TrimPrefix(strings.Trim(documentPath, "/"), "thoughts/"),
		"/",
	)
	base, err := filepath.Abs(s.markdownBasePath)
	if err != nil {
		return "", false
	}
	abs := filepath.Join(base, filepath.FromSlash(cleanDoc))
	dir := abs
	if info, err := os.Stat(abs); err == nil && !info.IsDir() {
		dir = filepath.Dir(abs)
	}
	for {
		if _, err := os.Stat(filepath.Join(dir, "AGENTS.md")); err == nil {
			rel, relErr := filepath.Rel(base, dir)
			if relErr == nil && rel != "." && !strings.HasPrefix(rel, "..") {
				return filepath.ToSlash(rel), true
			}
		}
		if dir == base || dir == filepath.Dir(dir) {
			break
		}
		dir = filepath.Dir(dir)
	}
	return "", false
}

func (s *Service) GetCommentsForScopeInternal(
	ctx context.Context,
	filePath string,
) (*GetCommentsResponse, error) {
	scope, err := s.ResolveCommentScope(ctx, filePath)
	if err != nil {
		return nil, err
	}
	if scope.Kind != CommentScopeQRSPIRoot {
		return s.GetCommentsForFileInternal(ctx, filePath)
	}
	comments, err := s.queries.ListWorkspaceDocumentComments(
		ctx,
		db.ListWorkspaceDocumentCommentsParams{
			WorkspaceRoot:   scope.RootDocPath,
			IncludeResolved: int64(0),
		},
	)
	if err != nil {
		return nil, fmt.Errorf("failed to get comments: %w", err)
	}
	current := make([]db.WorkspaceDocComment, 0, len(comments))
	other := make([]db.WorkspaceDocComment, 0, len(comments))
	for _, comment := range comments {
		if comment.DocPath == scope.CurrentPath {
			current = append(current, comment)
		} else {
			other = append(other, comment)
		}
	}
	comments = append(current, other...)
	return s.commentsResponseFromRows(ctx, scope.CurrentPath, comments)
}

func (s *Service) commentsResponseFromRows(
	ctx context.Context,
	currentPath string,
	comments []db.WorkspaceDocComment,
) (*GetCommentsResponse, error) {
	commentsWithReplies := make([]CommentWithReplies, len(comments))
	for i, comment := range comments {
		replies, err := s.queries.ListDocumentCommentReplies(ctx, comment.ID)
		if err != nil {
			return nil, fmt.Errorf("failed to get replies: %w", err)
		}
		commentsWithReplies[i] = CommentWithReplies{Comment: comment, Replies: replies}
	}

	permalink := fmt.Sprintf("%s/%s/%s", s.githubBaseURL, s.gitCommit, currentPath)
	return &GetCommentsResponse{
		Comments:        commentsWithReplies,
		GitCommit:       s.gitCommit,
		GitHubPermalink: permalink,
	}, nil
}

func (s *Service) BuildPermalink(filePath string, startLine, endLine int) string {
	if startLine == endLine {
		return fmt.Sprintf(
			"%s/%s/%s#L%d",
			s.githubBaseURL,
			s.gitCommit,
			filePath,
			startLine,
		)
	}
	return fmt.Sprintf(
		"%s/%s/%s#L%d-L%d",
		s.githubBaseURL,
		s.gitCommit,
		filePath,
		startLine,
		endLine,
	)
}

func (s *Service) ResolveComment(ctx context.Context, commentID string) error {
	if err := s.queries.ResolveDocumentComment(
		ctx,
		db.ResolveDocumentCommentParams{ID: commentID},
	); err != nil {
		return fmt.Errorf("failed to resolve comment: %w", err)
	}
	return nil
}

func (s *Service) UnresolveComment(ctx context.Context, commentID string) error {
	if err := s.queries.ReopenDocumentComment(ctx, commentID); err != nil {
		return fmt.Errorf("failed to unresolve comment: %w", err)
	}
	return nil
}
