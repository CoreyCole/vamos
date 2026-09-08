package workbench

import "strings"

func ChatHeaderInitial(title string) string {
	runes := []rune(strings.TrimSpace(title))
	if len(runes) == 0 {
		return ""
	}
	return strings.ToUpper(string(runes[0:1]))
}

func ChatHeaderAvatarClass(title string) string {
	switch strings.TrimSpace(title) {
	case "Bot":
		return "bg-fuchsia-500/90 text-white"
	case "Research agent":
		return "bg-sky-500/80 text-white"
	case "Vamos dev":
		return "bg-emerald-500/90 text-white"
	default:
		return "bg-muted text-muted-foreground"
	}
}
