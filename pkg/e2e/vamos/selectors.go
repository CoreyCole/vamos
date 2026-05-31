package vamos

import "github.com/coreycole/datastarui/e2e/spec"

type expectation struct{ step spec.Step }

func (e expectation) CheckStep() spec.Step { return e.step }

type stepFixture struct{ step spec.Step }

func (f stepFixture) SetupStep() spec.Step { return f.step }

var Thoughts thoughtsFeature
var AgentChat agentChatFeature
var Console consoleExpectations

type thoughtsFeature struct{}

func (thoughtsFeature) Page() page { return Pages.Thoughts() }
func (thoughtsFeature) Ready() expectation {
	return expectation{spec.All(spec.Visible(Thoughts.Sidebar()), spec.Visible(Thoughts.CenterPane()))}
}
func (thoughtsFeature) SidebarVisible() expectation {
	return expectation{spec.Visible(Thoughts.Sidebar())}
}
func (thoughtsFeature) CenterPaneVisible() expectation {
	return expectation{spec.Visible(Thoughts.CenterPane())}
}
func (thoughtsFeature) RightRailVisible() expectation {
	return expectation{spec.Visible(Thoughts.RightRail())}
}
func (thoughtsFeature) RightRailChatTabVisible() expectation {
	return expectation{spec.Visible(Thoughts.RightRailChatTab())}
}
func (thoughtsFeature) Document(title string) expectation {
	return expectation{spec.Visible(spec.Text(title))}
}
func (thoughtsFeature) Sidebar() spec.Locator {
	return spec.CSS("#doc-workbench-sidebar-region, #thoughts-shared-sidebar, #thoughts-workbench-sidebar, [data-e2e='thoughts.workbench.sidebar']")
}
func (thoughtsFeature) CenterPane() spec.Locator {
	return spec.CSS("#doc-workbench-center, #thoughts-workbench-center, main[data-e2e='thoughts.workbench.center'], [data-e2e='thoughts.workbench.center']")
}
func (thoughtsFeature) RightRail() spec.Locator {
	return spec.CSS("#doc-workbench-right-rail, #thoughts-workbench-right, aside[data-e2e='thoughts.workbench.right'], [data-e2e='thoughts.workbench.right']")
}
func (thoughtsFeature) RightRailChatTab() spec.Locator {
	return spec.CSS("[role='tab'][aria-controls*='chat'], button:has-text('Chat'), [data-e2e='thoughts.rightRail.chat']")
}
func (thoughtsFeature) WorkspacesTab() spec.Locator {
	return spec.CSS("[role='tab'][aria-controls*='workspace'], button:has-text('Workspaces'), [data-e2e='thoughts.sidebar.workspaces']")
}

type agentChatFeature struct{}

func (agentChatFeature) Composer() spec.Locator {
	return spec.CSS("#agent-chat-composer-input, textarea[name='message'], textarea")
}
func (agentChatFeature) Transcript() spec.Locator {
	return spec.CSS("#agent-chat-messages, #agent-chat-transcript, [data-testid='agent-chat-transcript']")
}
func (agentChatFeature) TranscriptBottom() spec.Locator {
	return spec.CSS("#agent-chat-transcript-bottom, [data-testid='agent-chat-transcript-bottom']")
}
func (agentChatFeature) TranscriptContains(text string) expectation { return TranscriptContains(text) }

type consoleExpectations struct{}

func (consoleExpectations) Clean() expectation { return expectation{spec.ConsoleClean()} }

func locatorForKey(key string) spec.Locator {
	switch key {
	case "thoughts.workbench.sidebar":
		return Thoughts.Sidebar()
	case "thoughts.workbench.center":
		return Thoughts.CenterPane()
	case "thoughts.workbench.right":
		return Thoughts.RightRail()
	case "thoughts.rightRail.chat":
		return Thoughts.RightRailChatTab()
	case "thoughts.sidebar.workspaces":
		return Thoughts.WorkspacesTab()
	case "agent-chat.composer":
		return AgentChat.Composer()
	case "agent-chat.transcript":
		return AgentChat.Transcript()
	case "agent-chat.transcript.bottom":
		return AgentChat.TranscriptBottom()
	default:
		return spec.CSS("[data-e2e='" + key + "'], [data-testid='" + key + "']")
	}
}

func Sidebar() spec.Locator          { return Thoughts.Sidebar() }
func CenterPane() spec.Locator       { return Thoughts.CenterPane() }
func RightRail() spec.Locator        { return Thoughts.RightRail() }
func RightRailChatTab() spec.Locator { return Thoughts.RightRailChatTab() }
func Composer() spec.Locator         { return AgentChat.Composer() }
func Transcript() spec.Locator       { return AgentChat.Transcript() }
func TranscriptBottom() spec.Locator { return AgentChat.TranscriptBottom() }
