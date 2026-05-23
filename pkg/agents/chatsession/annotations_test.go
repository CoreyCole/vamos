package chatsession

import (
	"context"
	"strings"
	"testing"

	"github.com/CoreyCole/vamos/pkg/db"
)

func TestCreateAnnotationAnchorsToMessageNode(t *testing.T) {
	ctx := context.Background()
	dbConn, q := openTestDB(t)
	insertServiceTestFixtures(t, ctx, q)
	svc := NewService(dbConn, q)

	annotation, err := svc.CreateAnnotation(ctx, CreateAnnotationInput{
		WorkspaceID:  "workspace-1",
		SessionID:    "session-1",
		NodeID:       "session-1:7",
		EventSeq:     7,
		AuthorEmail:  "reviewer@example.com",
		BodyMarkdown: "Please expand this result.",
	})
	if err != nil {
		t.Fatalf("CreateAnnotation() error = %v", err)
	}
	if annotation.Status != "open" {
		t.Fatalf("status = %q, want open", annotation.Status)
	}

	annotations, err := q.ListChatAnnotationsBySession(ctx, "session-1")
	if err != nil {
		t.Fatalf("ListChatAnnotationsBySession() error = %v", err)
	}
	if len(annotations) != 1 {
		t.Fatalf("annotations len = %d, want 1", len(annotations))
	}
	if annotations[0].NodeID != "session-1:7" || annotations[0].EventSeq != 7 {
		t.Fatalf(
			"annotation anchor = (%s,%d), want message node",
			annotations[0].NodeID,
			annotations[0].EventSeq,
		)
	}
}

func TestBuildReplyContextIncludesSelectedAnnotations(t *testing.T) {
	contextText := BuildReplyContext(
		[]db.ChatAnnotation{{
			ID:           "annotation-1",
			WorkspaceID:  "workspace-1",
			SessionID:    "session-1",
			NodeID:       "session-1:7",
			EventSeq:     7,
			AuthorEmail:  "reviewer@example.com",
			BodyMarkdown: "Please expand this result.",
			Status:       "open",
		}},
		[]ChatAnchor{{
			WorkspaceID: "workspace-1",
			SessionID:   "session-1",
			NodeID:      "session-1:7",
			EventSeq:    7,
		}},
	)
	for _, want := range []string{
		"Selected chat annotations:",
		"reviewer@example.com",
		"Please expand this result.",
		"session-1:7",
		"seq 7",
	} {
		if !strings.Contains(contextText, want) {
			t.Fatalf("context = %q, want %q", contextText, want)
		}
	}
}
