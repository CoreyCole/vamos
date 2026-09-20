package agentchat

import (
	"context"
	"database/sql"
	"fmt"
	"net/url"
	"strings"
	"time"

	"github.com/google/uuid"

	"github.com/CoreyCole/vamos/pkg/db"
)

const (
	agentChatMessageThreadID         = "agent-chat-message-thread"
	agentChatTranscriptColumnID      = "agent-chat-transcript-column"
	agentChatTranscriptColumnInnerID = "agent-chat-transcript-column-inner"
	messageThreadReplyAvatarCap      = 4
)

// ThreadReplyAuthor is one distinct reply author for the Slack summary stack.
type ThreadReplyAuthor struct {
	Initial  string
	Name     string
	AvatarBg string
}

// ThreadReplySummary is under-bubble Slack chrome (count >= 1 only).
type ThreadReplySummary struct {
	ParentEntryID string
	ReplyCount    int
	LastReplyAt   time.Time
	Authors       []ThreadReplyAuthor
}

// MessageThreadReply is one chronological reply in the full-column panel.
type MessageThreadReply struct {
	ID        string
	Body      string
	Author    ThreadReplyAuthor
	CreatedAt time.Time
}

// MessageThreadView is the open-thread full-column panel payload.
type MessageThreadView struct {
	ThreadID         string
	Parent           TranscriptMessage
	Replies          []MessageThreadReply
	ComposerDisabled bool
	Open             bool
}

func transcriptEntryID(msg TranscriptMessage) string {
	id := strings.TrimSpace(msg.EntryID)
	if id != "" {
		return id
	}
	return strings.TrimSpace(msg.DOMID)
}

func messageThreadSummaryID(entryID string) string {
	entryID = strings.TrimSpace(entryID)
	if entryID == "" {
		return ""
	}
	return "msg-" + entryID + "-thread-summary"
}

func messageThreadReplyRowID(parentID, replyID string) string {
	return "msg-" + strings.TrimSpace(
		parentID,
	) + "-thread-reply-" + strings.TrimSpace(
		replyID,
	)
}

func formatReplyCount(n int) string {
	if n == 1 {
		return "1 reply"
	}
	return fmt.Sprintf("%d replies", n)
}

func formatLastReply(at time.Time) string {
	if at.IsZero() {
		return ""
	}
	elapsed := time.Since(at)
	if elapsed < 0 {
		elapsed = 0
	}
	switch {
	case elapsed < 45*time.Second:
		return "Last reply just now"
	case elapsed < 90*time.Second:
		return "Last reply 1 minute ago"
	case elapsed < 60*time.Minute:
		return fmt.Sprintf("Last reply %d minutes ago", int(elapsed.Minutes()))
	case elapsed < 90*time.Minute:
		return "Last reply 1 hour ago"
	case elapsed < 24*time.Hour:
		return fmt.Sprintf("Last reply %d hours ago", int(elapsed.Hours()))
	default:
		return "Last reply " + at.Local().Format("Jan 2")
	}
}

func (s *Service) attachThreadReplySummaries(
	ctx context.Context,
	threadID string,
	messages []TranscriptMessage,
) []TranscriptMessage {
	if s == nil || s.queries == nil || strings.TrimSpace(threadID) == "" ||
		len(messages) == 0 {
		return messages
	}
	rows, err := s.queries.ListAgentThreadEntriesByThread(ctx, threadID)
	if err != nil || len(rows) == 0 {
		return messages
	}
	summaries := summarizeThreadReplies(rows)
	for i := range messages {
		id := strings.TrimSpace(messages[i].EntryID)
		if id == "" {
			id = strings.TrimSpace(messages[i].DOMID)
		}
		if sum, ok := summaries[id]; ok && sum.ReplyCount > 0 {
			messages[i].ThreadSummary = &sum
		}
	}
	return messages
}

func summarizeThreadReplies(rows []db.AgentThreadEntry) map[string]ThreadReplySummary {
	out := map[string]ThreadReplySummary{}
	seenAuthor := map[string]map[string]struct{}{}
	for _, row := range rows {
		parent := strings.TrimSpace(row.ParentEntryID.String)
		if parent == "" {
			continue
		}
		sum := out[parent]
		sum.ParentEntryID = parent
		sum.ReplyCount++
		if row.CreatedAt.After(sum.LastReplyAt) {
			sum.LastReplyAt = row.CreatedAt
		}
		key := strings.ToLower(strings.TrimSpace(row.AuthorName))
		if key == "" {
			key = strings.ToLower(strings.TrimSpace(row.AuthorEmail))
		}
		if key == "" {
			key = strings.TrimSpace(row.ID)
		}
		if seenAuthor[parent] == nil {
			seenAuthor[parent] = map[string]struct{}{}
		}
		if _, ok := seenAuthor[parent][key]; !ok {
			seenAuthor[parent][key] = struct{}{}
			initial := strings.TrimSpace(row.AuthorInitial)
			if initial == "" && strings.TrimSpace(row.AuthorName) != "" {
				initial = strings.ToUpper(string([]rune(row.AuthorName)[0:1]))
			}
			sum.Authors = append(sum.Authors, ThreadReplyAuthor{
				Initial:  initial,
				Name:     strings.TrimSpace(row.AuthorName),
				AvatarBg: strings.TrimSpace(row.AvatarBg),
			})
		}
		out[parent] = sum
	}
	return out
}

func (s *Service) loadMessageThreadView(
	ctx context.Context,
	threadID, parentEntryID string,
	parent TranscriptMessage,
	composerDisabled bool,
) (MessageThreadView, error) {
	view := MessageThreadView{
		ThreadID:         strings.TrimSpace(threadID),
		Parent:           parent,
		ComposerDisabled: composerDisabled,
		Open:             strings.TrimSpace(parentEntryID) != "",
	}
	if s == nil || s.queries == nil || !view.Open {
		return view, nil
	}
	rows, err := s.queries.ListAgentThreadEntriesByParent(
		ctx,
		db.ListAgentThreadEntriesByParentParams{
			ThreadID: view.ThreadID,
			ParentEntryID: sql.NullString{
				String: strings.TrimSpace(parentEntryID),
				Valid:  true,
			},
		},
	)
	if err != nil {
		return view, err
	}
	view.Replies = make([]MessageThreadReply, 0, len(rows))
	for _, row := range rows {
		initial := strings.TrimSpace(row.AuthorInitial)
		if initial == "" && strings.TrimSpace(row.AuthorName) != "" {
			initial = strings.ToUpper(string([]rune(row.AuthorName)[0:1]))
		}
		view.Replies = append(view.Replies, MessageThreadReply{
			ID:   row.ID,
			Body: row.Body,
			Author: ThreadReplyAuthor{
				Initial:  initial,
				Name:     strings.TrimSpace(row.AuthorName),
				AvatarBg: strings.TrimSpace(row.AvatarBg),
			},
			CreatedAt: row.CreatedAt,
		})
	}
	return view, nil
}

func (s *Service) insertMessageThreadReply(
	ctx context.Context,
	threadID, parentEntryID, userEmail, body string,
) (db.AgentThreadEntry, error) {
	body = strings.TrimSpace(body)
	if body == "" {
		return db.AgentThreadEntry{}, fmt.Errorf("reply body is required")
	}
	parentEntryID = strings.TrimSpace(parentEntryID)
	if parentEntryID == "" {
		return db.AgentThreadEntry{}, fmt.Errorf("parent_entry_id is required")
	}
	name := strings.TrimSpace(userEmail)
	if i := strings.Index(name, "@"); i > 0 {
		name = name[:i]
	}
	if name == "" {
		name = "You"
	}
	initial := strings.ToUpper(string([]rune(name)[0:1]))
	id := uuid.NewString()
	if err := s.queries.InsertAgentThreadEntry(ctx, db.InsertAgentThreadEntryParams{
		ID:            id,
		ThreadID:      strings.TrimSpace(threadID),
		ParentEntryID: sql.NullString{String: parentEntryID, Valid: true},
		AuthorKind:    "user",
		AuthorName:    name,
		AuthorInitial: initial,
		AuthorEmail:   strings.TrimSpace(userEmail),
		AvatarBg:      "bg-slate-600",
		Body:          body,
	}); err != nil {
		return db.AgentThreadEntry{}, err
	}
	return s.queries.GetAgentThreadEntry(ctx, id)
}

func findTranscriptMessage(
	messages []TranscriptMessage,
	entryID string,
) TranscriptMessage {
	entryID = strings.TrimSpace(entryID)
	if entryID == "" {
		return TranscriptMessage{}
	}
	for _, msg := range messages {
		if strings.TrimSpace(msg.EntryID) == entryID ||
			strings.TrimSpace(msg.DOMID) == entryID {
			return msg
		}
	}
	return TranscriptMessage{DOMID: entryID, EntryID: entryID}
}

func visibleReplyAuthors(
	authors []ThreadReplyAuthor,
) (shown []ThreadReplyAuthor, extra int) {
	if len(authors) <= messageThreadReplyAvatarCap {
		return authors, 0
	}
	return authors[:messageThreadReplyAvatarCap], len(
		authors,
	) - messageThreadReplyAvatarCap
}

func openMessageThreadExpr(threadID, entryID string) string {
	return "@get('/agent-chat/thread/" + url.PathEscape(threadID) +
		"/message-thread?parent=" + url.QueryEscape(entryID) + "')"
}

func closeMessageThreadExpr(threadID string) string {
	return "@get('/agent-chat/thread/" + url.PathEscape(threadID) + "/message-thread')"
}

func transcriptColumnSignals(hidden bool) string {
	if hidden {
		return "{messageThreadOpen: true}"
	}
	return "{messageThreadOpen: false}"
}

func postMessageThreadReplyExpr(threadID, parentEntryID string) string {
	return "@post('/agent-chat/thread/" + url.PathEscape(threadID) +
		"/replies', {contentType: 'form'})"
}
