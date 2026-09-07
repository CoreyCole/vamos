package agentchat

import "strings"

const (
	defaultBubbleAuthorInitial = "B"
	defaultBubbleAuthorName    = "Bot"
	defaultBubbleAvatarBg      = "bg-fuchsia-500/90"
	defaultBubbleNameColor     = "text-fuchsia-300"
)

func bubbleAuthorChrome(args ChatMessageArgs) (initial, name, avatarBg, nameColor string) {
	initial = strings.TrimSpace(args.AuthorInitial)
	name = strings.TrimSpace(args.AuthorName)
	avatarBg = strings.TrimSpace(args.AvatarBg)
	nameColor = strings.TrimSpace(args.NameColor)
	if initial == "" {
		initial = defaultBubbleAuthorInitial
	}
	if name == "" {
		name = defaultBubbleAuthorName
	}
	if avatarBg == "" {
		avatarBg = defaultBubbleAvatarBg
	}
	if nameColor == "" {
		nameColor = defaultBubbleNameColor
	}
	return initial, name, avatarBg, nameColor
}

func streamingBubbleAuthorChrome(args ChatMessageStreamingArgs) (initial, name, avatarBg, nameColor string) {
	return bubbleAuthorChrome(ChatMessageArgs{
		AuthorInitial: args.AuthorInitial,
		AuthorName:    args.AuthorName,
		AvatarBg:      args.AvatarBg,
		NameColor:     args.NameColor,
	})
}

func completeBubbleRole(args ChatMessageCompleteArgs) string {
	role := strings.TrimSpace(args.Role)
	if role == "" {
		return "assistant"
	}
	return role
}

func chatMessageArgsFromTranscript(msg TranscriptMessage) ChatMessageArgs {
	return ChatMessageArgs{
		ID:             msg.DOMID,
		Role:           msg.Role,
		Content:        msg.Content,
		HTMLContent:    msg.HTMLContent,
		Attachments:    msg.Attachments,
		AuthorInitial:  msg.AuthorInitial,
		AuthorName:     msg.AuthorName,
		AvatarBg:       msg.AvatarBg,
		NameColor:      msg.NameColor,
		QuoteInitial:   msg.QuoteInitial,
		QuoteName:      msg.QuoteName,
		QuoteAvatarBg:  msg.QuoteAvatarBg,
		QuoteNameColor: msg.QuoteNameColor,
		QuoteText:      msg.QuoteText,
	}
}

// withDefaultAssistantBubbleChrome fills Bot chrome when assistant author fields are empty.
func withDefaultAssistantBubbleChrome(msg TranscriptMessage) TranscriptMessage {
	if strings.TrimSpace(msg.Role) != "assistant" && strings.TrimSpace(msg.Role) != "" {
		return msg
	}
	if strings.TrimSpace(msg.Role) == "" {
		msg.Role = "assistant"
	}
	if strings.TrimSpace(msg.AuthorInitial) == "" {
		msg.AuthorInitial = defaultBubbleAuthorInitial
	}
	if strings.TrimSpace(msg.AuthorName) == "" {
		msg.AuthorName = defaultBubbleAuthorName
	}
	if strings.TrimSpace(msg.AvatarBg) == "" {
		msg.AvatarBg = defaultBubbleAvatarBg
	}
	if strings.TrimSpace(msg.NameColor) == "" {
		msg.NameColor = defaultBubbleNameColor
	}
	return msg
}
