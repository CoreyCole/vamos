//go:build !integration || unit
// +build !integration unit

package comments

import "testing"

func TestCountsByDocPathReportsResolvedAndTotal(t *testing.T) {
	t.Parallel()
	svc := newTestCommentsService(t)
	path := "thoughts/plan.md"
	if _, err := svc.createCommentInternal(
		t.Context(),
		"user@example.com",
		CreateCommentRequest{FilePath: path, CommentText: "one", SectionID: "section-1"},
	); err != nil {
		t.Fatalf("createCommentInternal() error = %v", err)
	}
	second, err := svc.createCommentInternal(
		t.Context(),
		"user@example.com",
		CreateCommentRequest{FilePath: path, CommentText: "two", SectionID: "section-1"},
	)
	if err != nil {
		t.Fatalf("createCommentInternal() second error = %v", err)
	}
	third, err := svc.createCommentInternal(
		t.Context(),
		"user@example.com",
		CreateCommentRequest{
			FilePath:    path,
			CommentText: "three",
			SectionID:   "section-1",
		},
	)
	if err != nil {
		t.Fatalf("createCommentInternal() third error = %v", err)
	}
	if err := svc.ResolveComment(t.Context(), second.ID); err != nil {
		t.Fatalf("ResolveComment() error = %v", err)
	}
	if err := svc.ResolveComment(t.Context(), third.ID); err != nil {
		t.Fatalf("ResolveComment() second error = %v", err)
	}

	counts, err := svc.CountsByDocPath(t.Context(), []string{path, "thoughts/other.md"})
	if err != nil {
		t.Fatalf("CountsByDocPath() error = %v", err)
	}
	got, ok := counts[path]
	if !ok {
		t.Fatalf("CountsByDocPath() missing %q: %#v", path, counts)
	}
	if got.Total != 3 || got.Resolved != 2 {
		t.Fatalf("CountsByDocPath() = %#v, want total=3 resolved=2", got)
	}
	if _, ok := counts["thoughts/other.md"]; ok {
		t.Fatalf("CountsByDocPath() included empty path: %#v", counts)
	}
}
