package vamos

import "github.com/coreycole/datastarui/e2e/spec"

func Feature(name string) spec.Locator { return spec.SelectorAlias("feature." + name) }
func Sidebar() spec.Locator            { return spec.SelectorAlias("thoughts.workbench.sidebar") }
func CenterPane() spec.Locator         { return spec.SelectorAlias("thoughts.workbench.center") }
func RightRail() spec.Locator          { return spec.SelectorAlias("thoughts.workbench.right") }
func RightRailChatTab() spec.Locator   { return spec.SelectorAlias("thoughts.rightRail.chat") }
func Composer() spec.Locator           { return spec.SelectorAlias("agent-chat.composer") }
func Transcript() spec.Locator         { return spec.SelectorAlias("agent-chat.transcript") }
func TranscriptBottom() spec.Locator   { return spec.SelectorAlias("agent-chat.transcript.bottom") }
