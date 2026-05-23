package workspaces

import "strings"

func cleanPathKey(value string) string {
	value = strings.TrimSpace(value)
	if value == "" {
		return ""
	}
	return strings.TrimRight(value, "/")
}
