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
	title = strings.TrimSpace(title)
	if title == "" {
		return "bg-muted text-muted-foreground"
	}
	palette := []string{
		"bg-muted text-muted-foreground",
		"bg-secondary text-secondary-foreground",
	}
	h := 0
	for _, r := range title {
		h = (h*31 + int(r)) & 0xffff
	}
	return palette[h%len(palette)]
}
