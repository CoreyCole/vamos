package agentchat

import (
	"database/sql"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/labstack/echo/v4"

	"github.com/CoreyCole/vamos/pkg/db"
	serverauth "github.com/CoreyCole/vamos/server/services/auth"
	serverdb "github.com/CoreyCole/vamos/server/services/db"
)

func TestCLIQuoteCommentsRequiresMachineAuthAndReturnsExactArtifactQueue(t *testing.T) {
	handler, queries, token, artifactPath := newCLIQuoteCommentsFixture(t)
	createCLIQuoteComment(t, queries, "00-unicode-space", artifactPath, "\u00a0", "unicode space")
	createCLIQuoteComment(t, queries, "comment-b", artifactPath, "second by id", "body b")
	createCLIQuoteComment(t, queries, "comment-a", artifactPath, "first by id", "body a")
	createCLIQuoteComment(t, queries, "blank", artifactPath, "", "whole document")
	createCLIQuoteComment(t, queries, "spaces", artifactPath, "   ", "spaces")
	createCLIQuoteComment(t, queries, "tabs", artifactPath, "\t\r\n", "tabs and lines")
	createCLIQuoteComment(
		t,
		queries,
		"other",
		"thoughts/other.html",
		"other quote",
		"other body",
	)
	createCLIQuoteComment(
		t,
		queries,
		"resolved",
		artifactPath,
		"resolved quote",
		"resolved body",
	)
	if err := queries.ResolveDocumentComment(
		t.Context(),
		db.ResolveDocumentCommentParams{ID: "resolved"},
	); err != nil {
		t.Fatal(err)
	}

	e := echo.New()
	handler.RegisterMachineAPIRoutes(e.Group("/agent-chat/api"))

	unauthorized := httptest.NewRecorder()
	e.ServeHTTP(
		unauthorized,
		httptest.NewRequest(
			http.MethodGet,
			"/agent-chat/api/comments?artifact_path="+artifactPath,
			http.NoBody,
		),
	)
	if unauthorized.Code != http.StatusUnauthorized {
		t.Fatalf(
			"unauthorized status = %d body=%q",
			unauthorized.Code,
			unauthorized.Body.String(),
		)
	}

	req := httptest.NewRequest(
		http.MethodGet,
		"/agent-chat/api/comments?artifact_path="+artifactPath+"&limit=2",
		http.NoBody,
	)
	req.Header.Set("Authorization", token)
	rec := httptest.NewRecorder()
	e.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d body=%q", rec.Code, rec.Body.String())
	}
	var response QuoteCommentsResponse
	if err := json.Unmarshal(rec.Body.Bytes(), &response); err != nil {
		t.Fatal(err)
	}
	if len(response.Comments) != 2 {
		t.Fatalf("comments = %+v", response.Comments)
	}
	if response.Comments[0].ID != "comment-a" || response.Comments[1].ID != "comment-b" {
		t.Fatalf("comment order = %+v", response.Comments)
	}
	for _, comment := range response.Comments {
		if comment.ArtifactPath != artifactPath || comment.Quote == "" {
			t.Fatalf("unexpected comment = %+v", comment)
		}
	}
}

func TestCLIQuoteCommentsRejectsInvalidArtifactPaths(t *testing.T) {
	handler, _, token, _ := newCLIQuoteCommentsFixture(t)
	e := echo.New()
	handler.RegisterMachineAPIRoutes(e.Group("/agent-chat/api"))

	outside := filepath.Join(t.TempDir(), "outside.html")
	if err := os.WriteFile(outside, []byte("outside"), 0o600); err != nil {
		t.Fatal(err)
	}
	symlink := filepath.Join(handler.service.thoughtsRoot, "escape.html")
	if err := os.Symlink(outside, symlink); err != nil {
		t.Fatal(err)
	}

	for _, artifactPath := range []string{
		"",
		outside,
		"thoughts/../outside.html",
		"thoughts/missing.html",
		"thoughts",
		"thoughts/escape.html",
	} {
		req := httptest.NewRequest(
			http.MethodGet,
			"/agent-chat/api/comments?artifact_path="+artifactPath,
			http.NoBody,
		)
		req.Header.Set("Authorization", token)
		rec := httptest.NewRecorder()
		e.ServeHTTP(rec, req)
		if rec.Code != http.StatusBadRequest {
			t.Fatalf(
				"artifact %q status = %d body=%q",
				artifactPath,
				rec.Code,
				rec.Body.String(),
			)
		}
		if body := rec.Body.String(); strings.Contains(body, handler.service.thoughtsRoot) || strings.Contains(body, outside) {
			t.Fatalf("artifact %q leaked filesystem path in %q", artifactPath, body)
		}
	}
}

func TestCLIQuoteCommentsHidesInternalErrors(t *testing.T) {
	handler, _, token, artifactPath := newCLIQuoteCommentsFixture(t)
	if err := handler.service.db.Close(); err != nil {
		t.Fatal(err)
	}
	e := echo.New()
	handler.RegisterMachineAPIRoutes(e.Group("/agent-chat/api"))
	req := httptest.NewRequest(http.MethodGet, "/agent-chat/api/comments?artifact_path="+artifactPath, http.NoBody)
	req.Header.Set("Authorization", token)
	rec := httptest.NewRecorder()
	e.ServeHTTP(rec, req)
	if rec.Code != http.StatusInternalServerError {
		t.Fatalf("status = %d body=%q", rec.Code, rec.Body.String())
	}
	body := rec.Body.String()
	if strings.Contains(body, handler.service.thoughtsRoot) || strings.Contains(strings.ToLower(body), "database is closed") {
		t.Fatalf("internal error leaked in %q", body)
	}
}

func newCLIQuoteCommentsFixture(
	t *testing.T,
) (*Handler, *db.Queries, string, string) {
	t.Helper()
	thoughtsRoot := t.TempDir()
	for name, content := range map[string]string{
		"demo.html":  "<p>demo</p>",
		"other.html": "<p>other</p>",
	} {
		if err := os.WriteFile(
			filepath.Join(thoughtsRoot, name),
			[]byte(content),
			0o600,
		); err != nil {
			t.Fatal(err)
		}
	}
	database, err := serverdb.NewService(filepath.Join(t.TempDir(), "comments.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = database.Close() })
	service := &Service{
		db:           database.DB(),
		queries:      database.Queries,
		thoughtsRoot: thoughtsRoot,
	}
	store := serverauth.NewMemoryMachineCredentialStore()
	created, err := store.Create(t.Context(), serverauth.CreateMachineCredentialInput{
		DefaultActorEmail: "agent@example.com",
	})
	if err != nil {
		t.Fatal(err)
	}
	handler := NewHandler(service, nil, HandlerOptions{MachineCredentials: store})
	token := "Bearer vamos_machine_" + created.Credential.ID + "." + created.Secret
	return handler, database.Queries, token, "thoughts/demo.html"
}

func createCLIQuoteComment(
	t *testing.T,
	queries *db.Queries,
	id, artifactPath, quote, body string,
) {
	t.Helper()
	_, err := queries.CreateDocumentComment(t.Context(), db.CreateDocumentCommentParams{
		ID:            id,
		WorkspaceRoot: "",
		WorkspaceID:   sql.NullString{},
		DocPath:       artifactPath,
		UserEmail:     "reviewer@example.com",
		CommentText:   body,
		SelectedText:  quote,
		SectionHint:   sql.NullString{String: "document", Valid: true},
		HeadingHint:   sql.NullString{String: "Document", Valid: true},
	})
	if err != nil {
		t.Fatal(err)
	}
}
