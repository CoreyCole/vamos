package agentchat

import (
	"context"
	"database/sql"
	"strings"

	"github.com/CoreyCole/vamos/pkg/db"
)

const (
	agentChatThreadFamilyID      = "agent-chat-thread-family"
	agentChatThreadFamilyListID  = "agent-chat-thread-family-list"
	agentChatThreadFamilySheetID = "agent-chat-thread-family-sheet"
)

type ChatThreadFamilyRow struct {
	ID       string
	Title    string
	Href     string
	Status   string
	IsHome   bool
	IsActive bool
}

type ChatThreadFamily struct {
	CurrentID    string
	CurrentTitle string
	Rows         []ChatThreadFamilyRow
}

func (f ChatThreadFamily) SelectedTitle() string {
	for _, row := range f.Rows {
		if row.IsActive {
			return row.Title
		}
	}
	return strings.TrimSpace(f.CurrentTitle)
}

func familyThreadHref(row db.AgentThread) string {
	if strings.TrimSpace(row.ID) == "" {
		return ""
	}
	if strings.TrimSpace(row.RoomKind) == RoomKindPairwise {
		if href := pairwiseA2AHref(
			row.PairAgentSlugA.String,
			row.PairAgentSlugB.String,
		); href != "" {
			return href
		}
	}
	return "/threads/" + row.ID
}

func (s *Service) BuildChatThreadFamily(
	ctx context.Context,
	threadID string,
) (ChatThreadFamily, error) {
	threadID = strings.TrimSpace(threadID)
	if threadID == "" || s == nil || s.queries == nil {
		return ChatThreadFamily{}, nil
	}
	current, err := s.queries.GetSharedAgentThread(ctx, threadID)
	if err != nil {
		return ChatThreadFamily{CurrentID: threadID}, err
	}
	home := current
	if current.ParentThreadID.Valid &&
		strings.TrimSpace(current.ParentThreadID.String) != "" {
		parent, err := s.queries.GetSharedAgentThread(ctx, current.ParentThreadID.String)
		if err == nil {
			home = parent
		}
	}
	children, err := s.queries.ListAgentThreadsByParentThreadID(
		ctx,
		sql.NullString{String: home.ID, Valid: true},
	)
	if err != nil {
		children = nil
	}
	family := ChatThreadFamily{
		CurrentID:    current.ID,
		CurrentTitle: strings.TrimSpace(current.Title),
		Rows:         make([]ChatThreadFamilyRow, 0, 1+len(children)),
	}
	family.Rows = append(family.Rows, s.familyRow(ctx, home, current.ID, true))
	for _, child := range children {
		family.Rows = append(
			family.Rows,
			s.familyRow(ctx, home, current.ID, false, child),
		)
	}
	return family, nil
}

func (s *Service) familyRow(
	ctx context.Context,
	home db.AgentThread,
	currentID string,
	isHome bool,
	child ...db.AgentThread,
) ChatThreadFamilyRow {
	row := home
	if !isHome && len(child) > 0 {
		row = child[0]
	}
	title := strings.TrimSpace(row.Title)
	if title == "" {
		if isHome {
			title = "Room"
		} else {
			title = "Subagent"
		}
	}
	status := ""
	if !isHome {
		status = s.subagentCardStatus(ctx, row)
	}
	return ChatThreadFamilyRow{
		ID:       row.ID,
		Title:    title,
		Href:     familyThreadHref(row),
		Status:   status,
		IsHome:   isHome,
		IsActive: row.ID == currentID,
	}
}

func familyOrCurrent(args EmbeddedFreeformPanelArgs) ChatThreadFamily {
	if len(args.ThreadFamily.Rows) > 0 {
		return args.ThreadFamily
	}
	id := strings.TrimSpace(args.ThreadID)
	if id == "" {
		return ChatThreadFamily{}
	}
	title := strings.TrimSpace(args.ThreadMetadata.Title)
	if title == "" {
		title = "Thread"
	}
	return ChatThreadFamily{
		CurrentID:    id,
		CurrentTitle: title,
		Rows: []ChatThreadFamilyRow{{
			ID:       id,
			Title:    title,
			Href:     "/threads/" + id,
			IsHome:   true,
			IsActive: true,
		}},
	}
}
