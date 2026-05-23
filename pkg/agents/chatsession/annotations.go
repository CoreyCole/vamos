package chatsession

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"github.com/google/uuid"

	"github.com/CoreyCole/vamos/pkg/db"
)

type ChatAnchor struct {
	WorkspaceID string
	SessionID   string
	EventSeq    int64
	NodeID      string
}

type CreateAnnotationInput struct {
	WorkspaceID  string
	SessionID    string
	NodeID       string
	EventSeq     int64
	AuthorEmail  string
	BodyMarkdown string
}

type ChatAnnotation struct {
	ID           string
	WorkspaceID  string
	SessionID    string
	NodeID       string
	EventSeq     int64
	AuthorEmail  string
	BodyMarkdown string
	Status       string
}

func (s *Service) CreateAnnotation(
	ctx context.Context,
	input CreateAnnotationInput,
) (db.ChatAnnotation, error) {
	body := strings.TrimSpace(input.BodyMarkdown)
	if body == "" {
		return db.ChatAnnotation{}, errors.New("annotation body is required")
	}
	return s.q.CreateChatAnnotation(ctx, db.CreateChatAnnotationParams{
		ID:           uuid.NewString(),
		WorkspaceID:  strings.TrimSpace(input.WorkspaceID),
		SessionID:    strings.TrimSpace(input.SessionID),
		NodeID:       strings.TrimSpace(input.NodeID),
		EventSeq:     input.EventSeq,
		AuthorEmail:  strings.TrimSpace(input.AuthorEmail),
		BodyMarkdown: body,
		Status:       "open",
	})
}

func (s *Service) ResolveAnnotation(
	ctx context.Context,
	id string,
	actorEmail string,
) error {
	_ = strings.TrimSpace(actorEmail)
	return s.q.ResolveChatAnnotation(ctx, strings.TrimSpace(id))
}

func (s *Service) AnnotationContext(ctx context.Context, ids []string) (string, error) {
	trimmed := make([]string, 0, len(ids))
	for _, id := range ids {
		if id = strings.TrimSpace(id); id != "" {
			trimmed = append(trimmed, id)
		}
	}
	if len(trimmed) == 0 {
		return "", nil
	}
	annotations, err := s.q.ListOpenChatAnnotationsByIDs(ctx, trimmed)
	if err != nil {
		return "", err
	}
	anchors := make([]ChatAnchor, 0, len(annotations))
	for _, annotation := range annotations {
		anchors = append(anchors, ChatAnchor{
			WorkspaceID: annotation.WorkspaceID,
			SessionID:   annotation.SessionID,
			EventSeq:    annotation.EventSeq,
			NodeID:      annotation.NodeID,
		})
	}
	return BuildReplyContext(annotations, anchors), nil
}

func ApplyAnnotationCounts(
	projection ChatProjection,
	annotations []db.ChatAnnotation,
) ChatProjection {
	counts := map[string]struct{ total, unresolved int }{}
	for _, annotation := range annotations {
		key := strings.TrimSpace(annotation.NodeID)
		if key == "" {
			continue
		}
		entry := counts[key]
		entry.total++
		if strings.TrimSpace(annotation.Status) == "open" {
			entry.unresolved++
		}
		counts[key] = entry
	}
	for i := range projection.Tree.Nodes {
		entry := counts[projection.Tree.Nodes[i].ID]
		projection.Tree.Nodes[i].AnnotationCount = entry.total
		projection.Tree.Nodes[i].UnresolvedCount = entry.unresolved
	}
	return projection
}

func BuildReplyContext(
	annotations []db.ChatAnnotation,
	anchors []ChatAnchor,
) string {
	if len(annotations) == 0 {
		return ""
	}
	anchorByKey := map[string]ChatAnchor{}
	for _, anchor := range anchors {
		anchorByKey[annotationAnchorKey(anchor.SessionID, anchor.NodeID, anchor.EventSeq)] = anchor
	}
	var b strings.Builder
	b.WriteString("Selected chat annotations:\n")
	for _, annotation := range annotations {
		anchor := anchorByKey[annotationAnchorKey(
			annotation.SessionID,
			annotation.NodeID,
			annotation.EventSeq,
		)]
		nodeID := firstNonEmpty(annotation.NodeID, anchor.NodeID)
		sessionID := firstNonEmpty(annotation.SessionID, anchor.SessionID)
		eventSeq := annotation.EventSeq
		if eventSeq == 0 {
			eventSeq = anchor.EventSeq
		}
		fmt.Fprintf(
			&b,
			"- %s on %s seq %d (%s): %s\n",
			strings.TrimSpace(annotation.AuthorEmail),
			nodeID,
			eventSeq,
			sessionID,
			strings.TrimSpace(annotation.BodyMarkdown),
		)
	}
	return strings.TrimSpace(b.String())
}

func annotationAnchorKey(sessionID, nodeID string, eventSeq int64) string {
	return strings.TrimSpace(
		sessionID,
	) + "\x00" + strings.TrimSpace(
		nodeID,
	) + fmt.Sprintf(
		"\x00%d",
		eventSeq,
	)
}
