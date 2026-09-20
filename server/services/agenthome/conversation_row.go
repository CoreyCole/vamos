package agenthome

// ConversationRowArgs is the shared Slack-style conversation list row
// used by the Bots navigator and scoped chat-column thread lists.
type ConversationRowArgs struct {
	ID              string
	Href            string
	Title           string
	Preview         string
	Time            string
	Initial         string
	AccentClass     string
	Selected        bool
	TestID          string
	DataRosterID    string
	DataRosterTitle string
}
