package layouts

// PageType represents the type/category of a page
type PageType string

const (
	// PageTypePlans represents a plan document page
	PageTypePlans PageType = "plans"

	// PageTypeResearch represents a research document page
	PageTypeResearch PageType = "research"

	// PageTypeSpec represents a specification document page
	PageTypeSpec PageType = "spec"

	// PageTypeDirectory represents a directory listing page
	PageTypeDirectory PageType = "directory"

	// PageTypeMarkdown represents a generic markdown page
	PageTypeMarkdown PageType = "markdown"

	// PageTypeHome represents the home page
	PageTypeHome PageType = "home"

	// PageTypeAgentChat represents the AgentChat page.
	PageTypeAgentChat PageType = "agentchat"

	// PageTypeSystem represents the system health dashboard page
	PageTypeSystem PageType = "system"

	// PageTypeStorybook represents the component storybook page
	PageTypeStorybook PageType = "storybook"

	// PageTypeWorkspaces represents the dev workspaces page.
	PageTypeWorkspaces PageType = "workspaces"
)

// String returns the string representation of the PageType
func (pt PageType) String() string {
	return string(pt)
}
