package selectors

import "fmt"

type Key string

type Entry struct {
	Key         Key
	CSS         string
	Description string
	StableID    string
}

type Catalog struct{ entries map[Key]Entry }

func DefaultCatalog() Catalog {
	entries := []Entry{
		{
			Key:         "thoughts.workbench.sidebar",
			CSS:         "[data-e2e='thoughts.workbench.sidebar'], #thoughts-workbench-sidebar",
			Description: "Thoughts workbench sidebar",
		},
		{
			Key:         "thoughts.workbench.center",
			CSS:         "[data-e2e='thoughts.workbench.center'], #thoughts-workbench-center",
			Description: "Thoughts document pane",
		},
		{
			Key:         "thoughts.workbench.right",
			CSS:         "[data-e2e='thoughts.workbench.right'], #thoughts-workbench-right",
			Description: "Thoughts right rail",
		},
		{
			Key:         "thoughts.sidebar.workspaces",
			CSS:         "[data-e2e='thoughts.sidebar.workspaces'], #thoughts-workbench-sidebar [role='tab']:has-text('Workspaces'), #thoughts-workbench-sidebar button:has-text('Workspaces')",
			Description: "Thoughts sidebar Workspaces tab",
		},
		{
			Key:         "thoughts.rightRail.chat",
			CSS:         "[data-e2e='thoughts.rightRail.chat'][aria-selected='true'], [role='tab'][aria-selected='true'][data-e2e='thoughts.rightRail.chat'], [data-e2e='thoughts.rightRail.chat']",
			Description: "Right rail Chat tab selected",
		},
		{
			Key:         "feature.thoughts.workbench",
			CSS:         "[data-feature='thoughts.workbench'], #thoughts-workbench",
			Description: "Workbench ready marker",
		},
		{
			Key:         "feature.agent-chat.workspace",
			CSS:         "[data-feature='agent-chat.workspace'], #agent-chat-workspace-shell, #agent-chat-workbench-shell",
			Description: "Agent Chat workspace ready marker",
		},
		{
			Key:         "feature.workspaces.list",
			CSS:         "[data-feature='workspaces.list'], #workspaces-list, [data-e2e='workspaces.list']",
			Description: "Workspaces list ready marker",
		},
		{
			Key:         "agent-chat.composer",
			CSS:         "#agent-chat-composer-input, textarea[name='message'], textarea",
			Description: "Agent Chat composer input",
		},
		{
			Key:         "agent-chat.transcript",
			CSS:         "#agent-chat-scroll-region, [data-e2e='agent-chat.transcript']",
			Description: "Agent Chat transcript region",
		},
		{
			Key:         "agent-chat.transcript.bottom",
			CSS:         "#agent-chat-scroll-region, [data-e2e='agent-chat.transcript']",
			Description: "Agent Chat transcript scrolled near bottom",
		},
	}
	out := Catalog{entries: map[Key]Entry{}}
	for _, e := range entries {
		out.entries[e.Key] = e
	}
	return out
}

func LoadCatalog(path string) (Catalog, error) { return DefaultCatalog(), nil }

func (c Catalog) Resolve(key string) (Entry, error) {
	e, ok := c.entries[Key(key)]
	if !ok {
		return Entry{}, fmt.Errorf("unknown selector key %q", key)
	}
	return e, nil
}

func (c Catalog) ValidateKeys(keys []string) error {
	for _, key := range keys {
		if _, err := c.Resolve(key); err != nil {
			return err
		}
	}
	return nil
}
