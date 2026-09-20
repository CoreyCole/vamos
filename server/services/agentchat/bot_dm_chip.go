package agentchat

import (
	"bufio"
	"context"
	"database/sql"
	"encoding/json"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/CoreyCole/vamos/pkg/db"
)

type pairwiseWindowCount struct {
	DestSlug        string
	DestName        string
	VisibleMessages int
}

type messageRoomOrigin struct {
	OriginTurnID string
	SpeakerSlug  string
	DestSlugs    []string
}

func deriveLiveBotDMChip(
	originTurnID, speakerSlug string,
	windows []pairwiseWindowCount,
) *BotDMChip {
	originTurnID = strings.TrimSpace(originTurnID)
	speakerSlug = strings.TrimSpace(speakerSlug)
	if originTurnID == "" {
		return nil
	}
	chip := &BotDMChip{
		OriginTurnID: originTurnID,
		SpeakerSlug:  speakerSlug,
	}
	seen := map[string]struct{}{}
	for _, window := range windows {
		slug := strings.TrimSpace(window.DestSlug)
		if slug == "" || slug == speakerSlug {
			continue
		}
		if strings.Contains(slug, "/") {
			continue
		}
		if _, ok := seen[slug]; ok {
			continue
		}
		seen[slug] = struct{}{}
		n := window.VisibleMessages
		if n < 0 {
			n = 0
		}
		name := strings.TrimSpace(window.DestName)
		if name == "" {
			name = slug
		}
		chip.Bots = append(chip.Bots, BotDMChipPeer{
			Name:          name,
			Slug:          slug,
			AuthorInitial: slugAuthorInitial(slug),
			AvatarBg:      inboundA2AAvatarBg,
			Count:         n,
		})
		chip.MessageCount += n
	}
	if len(chip.Bots) == 0 {
		return nil
	}
	return chip
}

func countVisiblePairwiseBubbles(items []TranscriptMessage) int {
	n := 0
	for _, item := range items {
		if strings.TrimSpace(item.Variant) == "bubble" {
			n++
		}
	}
	return n
}

func collectMessageRoomOrigins(items []TranscriptMessage) []messageRoomOrigin {
	var origins []messageRoomOrigin
	indexByTurn := map[string]int{}
	pendingSpeaker := ""
	pendingTurn := ""
	for _, item := range items {
		if strings.TrimSpace(item.Variant) == "bubble" &&
			strings.TrimSpace(item.Role) == "assistant" {
			pendingTurn = originTurnID(item)
			if slug := strings.TrimSpace(item.AuthorName); slug != "" &&
				!strings.Contains(slug, "@") {
				pendingSpeaker = slug
			}
		}
		if !isMessageRoomToolRow(item) {
			continue
		}
		dest := strings.TrimSpace(item.HeaderCode)
		if dest == "" || strings.Contains(dest, "/") {
			continue
		}
		turnID := strings.TrimSpace(item.EntryID)
		if turnID == "" {
			turnID = pendingTurn
		}
		if turnID == "" {
			continue
		}
		speaker := pendingSpeaker
		if slug := strings.TrimSpace(item.AuthorName); slug != "" &&
			!strings.Contains(slug, "@") {
			speaker = slug
		}
		idx, ok := indexByTurn[turnID]
		if !ok {
			idx = len(origins)
			indexByTurn[turnID] = idx
			origins = append(origins, messageRoomOrigin{
				OriginTurnID: turnID,
				SpeakerSlug:  speaker,
			})
		}
		origins[idx].DestSlugs = appendDistinct(origins[idx].DestSlugs, dest)
		if origins[idx].SpeakerSlug == "" {
			origins[idx].SpeakerSlug = speaker
		}
	}
	return origins
}

func isMessageRoomToolRow(item TranscriptMessage) bool {
	if item.ToolResult {
		return false
	}
	return strings.EqualFold(strings.TrimSpace(item.Title), "message_room")
}

func originTurnID(item TranscriptMessage) string {
	if id := strings.TrimSpace(item.EntryID); id != "" {
		return id
	}
	return strings.TrimSpace(item.DOMID)
}

func appendDistinct(values []string, next string) []string {
	next = strings.TrimSpace(next)
	if next == "" {
		return values
	}
	for _, existing := range values {
		if existing == next {
			return values
		}
	}
	return append(values, next)
}

func attachBotDMChipToOriginTurn(
	items []TranscriptMessage,
	chip *BotDMChip,
) []TranscriptMessage {
	if chip == nil || len(items) == 0 {
		return items
	}
	origin := strings.TrimSpace(chip.OriginTurnID)
	if origin == "" {
		return items
	}
	last := -1
	for i, item := range items {
		if strings.TrimSpace(item.Variant) != "bubble" {
			continue
		}
		if strings.TrimSpace(item.Role) != "assistant" {
			continue
		}
		if originTurnID(item) == origin {
			last = i
		}
	}
	if last < 0 {
		return items
	}
	copyChip := *chip
	items[last].BotDMChip = &copyChip
	return items
}

func countVisibleJSONLMessages(path string) int {
	path = strings.TrimSpace(path)
	if path == "" {
		return 0
	}
	file, err := os.Open(path)
	if err != nil {
		return 0
	}
	defer file.Close()
	scanner := bufio.NewScanner(file)
	scanner.Buffer(make([]byte, 0, 64*1024), 1024*1024)
	n := 0
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line == "" {
			continue
		}
		var envelope struct {
			Type    string `json:"type"`
			Message struct {
				Role string `json:"role"`
			} `json:"message"`
		}
		if err := json.Unmarshal([]byte(line), &envelope); err != nil {
			continue
		}
		if envelope.Type != "message" {
			continue
		}
		switch strings.TrimSpace(envelope.Message.Role) {
		case "user", "assistant":
			n++
		}
	}
	return n
}

func (s *Service) attachDerivedBotDMChipsForThreadID(
	threadID string,
	items []TranscriptMessage,
) []TranscriptMessage {
	if s == nil || s.queries == nil {
		return items
	}
	thread, err := s.queries.GetAgentThread(
		context.Background(),
		strings.TrimSpace(threadID),
	)
	if err != nil {
		return items
	}
	return s.attachDerivedBotDMChips(thread, items)
}

func (s *Service) attachDerivedBotDMChips(
	thread db.AgentThread,
	items []TranscriptMessage,
) []TranscriptMessage {
	if strings.TrimSpace(thread.RoomKind) == RoomKindPairwise {
		return items
	}
	origins := collectMessageRoomOrigins(items)
	if len(origins) == 0 {
		return items
	}
	for _, origin := range origins {
		windows := make([]pairwiseWindowCount, 0, len(origin.DestSlugs))
		for _, dest := range origin.DestSlugs {
			windows = append(windows, pairwiseWindowCount{
				DestSlug:        dest,
				DestName:        dest,
				VisibleMessages: s.pairwiseWindowVisibleCount(origin.SpeakerSlug, dest),
			})
		}
		items = attachBotDMChipToOriginTurn(
			items,
			deriveLiveBotDMChip(origin.OriginTurnID, origin.SpeakerSlug, windows),
		)
	}
	return items
}

func (s *Service) pairwiseWindowVisibleCount(speaker, dest string) int {
	id := RoomIdentity{Kind: RoomKindPairwise, PairA: speaker, PairB: dest}
	rel, err := id.CurrentJSONLRel()
	if err != nil {
		return 0
	}
	abs, err := AbsFromThoughtsRel(s.thoughtsRoot, rel)
	if err != nil {
		return 0
	}
	if n := countVisibleJSONLMessages(abs); n > 0 {
		return n
	}
	return countLatestPairwiseHistoryMessages(s.thoughtsRoot, id)
}

func countLatestPairwiseHistoryMessages(thoughtsRoot string, id RoomIdentity) int {
	rel, err := id.HistoryRel()
	if err != nil {
		return 0
	}
	abs, err := AbsFromThoughtsRel(thoughtsRoot, rel)
	if err != nil {
		return 0
	}
	entries, err := os.ReadDir(abs)
	if err != nil {
		return 0
	}
	names := make([]string, 0, len(entries))
	for _, entry := range entries {
		if entry.IsDir() {
			continue
		}
		name := entry.Name()
		if !strings.HasSuffix(name, ".jsonl") {
			continue
		}
		names = append(names, name)
	}
	if len(names) == 0 {
		return 0
	}
	sort.Strings(names)
	return countVisibleJSONLMessages(filepath.Join(abs, names[len(names)-1]))
}

func (s *Service) notifyPairwiseOriginTranscripts(
	ctx context.Context,
	workspaceID, pairwiseThreadID string,
) {
	if s == nil || s.queries == nil {
		return
	}
	pairwiseThreadID = strings.TrimSpace(pairwiseThreadID)
	if pairwiseThreadID == "" {
		return
	}
	thread, err := s.queries.GetAgentThread(ctx, pairwiseThreadID)
	if err != nil || strings.TrimSpace(thread.RoomKind) != RoomKindPairwise {
		return
	}
	seen := map[string]struct{}{pairwiseThreadID: {}}
	notify := func(id string) {
		id = strings.TrimSpace(id)
		if id == "" {
			return
		}
		if _, ok := seen[id]; ok {
			return
		}
		seen[id] = struct{}{}
		if strings.TrimSpace(workspaceID) != "" {
			s.notifyLiveTranscriptDirty(workspaceID, id)
		}
		s.notifyThreadScope(ctx, id, PatchLiveTranscript)
	}
	for _, agentSlug := range []string{
		strings.TrimSpace(thread.PairAgentSlugA.String),
		strings.TrimSpace(thread.PairAgentSlugB.String),
	} {
		if agentSlug == "" {
			continue
		}
		origins, err := s.queries.ListAgentThreadsByAgentSlug(
			ctx,
			sql.NullString{String: agentSlug, Valid: true},
		)
		if err != nil {
			continue
		}
		for _, origin := range origins {
			notify(origin.ID)
		}
	}
	listed, err := s.queries.ListAgentThreads(ctx, db.ListAgentThreadsParams{
		UserEmail: "",
		Limit:     500,
	})
	if err != nil {
		return
	}
	for _, row := range listed {
		if strings.TrimSpace(row.RoomKind) == RoomKindPlan {
			notify(row.ID)
		}
	}
}
