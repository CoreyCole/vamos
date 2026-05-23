package commentui

import "time"

type ThreadSource struct {
	ID           string
	AuthorEmail  string
	ActorLabel   string
	CreatedAt    time.Time
	Body         string
	SelectedText string
	SectionID    string
	HeadingHint  string
	Resolved     bool
	Replies      []ReplySource
	HiddenFields map[string]string
}

type ReplySource struct {
	AuthorEmail string
	ActorLabel  string
	CreatedAt   time.Time
	Body        string
}

type TargetInput struct {
	Surface      CommentSurface
	IDPrefix     string
	DocPath      string
	SectionID    string
	HeadingHint  string
	UserEmail    string
	Threads      []CommentThreadView
	Routes       CommentRoutes
	HiddenFields map[string]string
}

func BuildThreadViews(items []ThreadSource) []CommentThreadView {
	views := make([]CommentThreadView, 0, len(items))
	for _, item := range items {
		replies := make([]CommentReplyView, 0, len(item.Replies))
		for _, reply := range item.Replies {
			replies = append(replies, CommentReplyView{
				AuthorEmail: reply.AuthorEmail,
				ActorLabel:  reply.ActorLabel,
				CreatedAt:   reply.CreatedAt,
				Body:        reply.Body,
			})
		}
		views = append(views, CommentThreadView{
			ID:           item.ID,
			AuthorEmail:  item.AuthorEmail,
			ActorLabel:   item.ActorLabel,
			CreatedAt:    item.CreatedAt,
			Body:         item.Body,
			SelectedText: item.SelectedText,
			SectionID:    sectionOrDocument(item.SectionID),
			HeadingHint:  item.HeadingHint,
			Resolved:     item.Resolved,
			Replies:      replies,
			HiddenFields: MergeHidden(item.HiddenFields, nil),
		})
	}
	return views
}

func BuildTargetView(input TargetInput) CommentTargetView {
	sectionID := sectionOrDocument(input.SectionID)
	hidden := MergeCommentHiddenFields(input.HiddenFields, map[string]string{
		"section_hint": sectionID,
		"heading_hint": input.HeadingHint,
	})
	return CommentTargetView{
		ID:           TargetID(input.IDPrefix, sectionID),
		SignalKey:    SignalKey(input.IDPrefix, sectionID),
		Surface:      input.Surface,
		DocPath:      input.DocPath,
		SectionID:    sectionID,
		HeadingHint:  input.HeadingHint,
		UserEmail:    input.UserEmail,
		Threads:      input.Threads,
		Routes:       input.Routes,
		HiddenFields: hidden,
	}
}

func MergeCommentHiddenFields(
	base map[string]string,
	pairs ...map[string]string,
) map[string]string {
	out := MergeHidden(base, nil)
	for _, pair := range pairs {
		out = MergeHidden(out, pair)
	}
	return out
}
