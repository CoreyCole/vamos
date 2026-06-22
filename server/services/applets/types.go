package applets

import "github.com/a-h/templ"

// AppletConfig describes a reusable self-modifying applet workbench.
type AppletConfig struct {
	ID              string
	DisplayName     string
	FilesRoot       string
	CurrentAppDir   string
	IterationsDir   string
	AppRoutePrefix  string
	UserLabels      AppletLabels
	HideDiagnostics bool
}

type AppletLabels struct {
	FilesTitle      string
	ChatTitle       string
	PromptHelp      string
	SuccessTemplate string
	FailureTemplate string
}

type FilesViewModel struct {
	Title       string
	CurrentPath string
	Component   templ.Component
}

type AppProxyViewModel struct {
	Title     string
	URL       string
	Healthy   bool
	Component templ.Component
}

type ChatViewModel struct {
	Title     string
	Component templ.Component
}

type UserNotice struct {
	Kind    string
	Message string
}

type WorkbenchState struct {
	Config       AppletConfig
	Files        FilesViewModel
	App          AppProxyViewModel
	Chat         ChatViewModel
	Notice       UserNotice
	MobileActive string
}
