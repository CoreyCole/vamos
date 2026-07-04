package workbench

import (
	"sort"
	"strings"
)

type OverflowActionKind string

type OverflowActionSubmitMode string

const (
	OverflowActionLink OverflowActionKind = "link"
	OverflowActionForm OverflowActionKind = "form"

	OverflowActionSubmitNative   OverflowActionSubmitMode = "native"
	OverflowActionSubmitDatastar OverflowActionSubmitMode = "datastar"
)

type OverflowAction struct {
	Label        string
	Description  string
	Kind         OverflowActionKind
	Href         string
	FormAction   string
	FormMethod   string
	SubmitMode   OverflowActionSubmitMode
	Target       string
	Rel          string
	HiddenFields map[string]string
	Disabled     bool
}

type OverflowActionGroup struct {
	Label   string
	Actions []OverflowAction
}

type OverflowActionsArgs struct {
	Label  string
	Groups []OverflowActionGroup
}

func overflowActionLabel(action OverflowAction) string {
	return strings.TrimSpace(action.Label)
}

func overflowFormMethod(action OverflowAction) string {
	method := strings.ToLower(strings.TrimSpace(action.FormMethod))
	if method == "" {
		return "post"
	}
	return method
}

func overflowDatastarSubmit(action OverflowAction) string {
	formAction := strings.TrimSpace(action.FormAction)
	if formAction == "" {
		return ""
	}
	return "@post('" + escapeDatastarString(formAction) + "', {contentType: 'form'})"
}

func overflowHiddenFieldNames(fields map[string]string) []string {
	names := make([]string, 0, len(fields))
	for name := range fields {
		names = append(names, name)
	}
	sort.Strings(names)
	return names
}

func escapeDatastarString(value string) string {
	value = strings.ReplaceAll(value, `\`, `\\`)
	return strings.ReplaceAll(value, `'`, `\'`)
}
