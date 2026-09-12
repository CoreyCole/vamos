package workbench

import (
	"fmt"
	"regexp"
	"strconv"
	"strings"
	"time"
	"unicode"
)

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

// ChatHeaderPlanSwatchClass is a stable accent for plan-room header dots (no glyph).
func ChatHeaderPlanSwatchClass(title string) string {
	title = strings.TrimSpace(title)
	palette := []string{
		"bg-sky-500/90",
		"bg-violet-500/90",
		"bg-rose-500/90",
		"bg-emerald-500/90",
		"bg-amber-500/90",
	}
	if title == "" {
		return palette[0]
	}
	h := 0
	for _, r := range title {
		h = (h*31 + int(r)) & 0xffff
	}
	return palette[h%len(palette)]
}

// ChatHeaderTitleParts is the parsed plan-slug header: human title + muted datetime.
// Non-matching titles (bot names) keep Display as-is with empty Datetime.
type ChatHeaderTitleParts struct {
	Display  string
	Datetime string
}

var chatHeaderPlanSlugRE = regexp.MustCompile(
	`^(\d{4})-(\d{2})-(\d{2})_(\d{2})-(\d{2})-(\d{2})_(.+)$`,
)

// ParseChatHeaderTitle turns YYYY-MM-DD_HH-MM-SS_<slug> into a readable title
// and muted datetime (e.g. "Sep 8, 2006 · 10:10"). Other titles pass through.
func ParseChatHeaderTitle(title string) ChatHeaderTitleParts {
	title = strings.TrimSpace(title)
	if title == "" {
		return ChatHeaderTitleParts{}
	}
	m := chatHeaderPlanSlugRE.FindStringSubmatch(title)
	if m == nil {
		return ChatHeaderTitleParts{Display: title}
	}
	year, _ := strconv.Atoi(m[1])
	month, _ := strconv.Atoi(m[2])
	day, _ := strconv.Atoi(m[3])
	hour, _ := strconv.Atoi(m[4])
	min, _ := strconv.Atoi(m[5])
	if month < 1 || month > 12 || day < 1 || day > 31 || hour > 23 || min > 59 {
		return ChatHeaderTitleParts{Display: title}
	}
	t := time.Date(year, time.Month(month), day, hour, min, 0, 0, time.UTC)
	if t.Year() != year || int(t.Month()) != month || t.Day() != day {
		return ChatHeaderTitleParts{Display: title}
	}
	display := humanizeChatHeaderSlug(m[7])
	if display == "" {
		display = title
	}
	return ChatHeaderTitleParts{
		Display:  display,
		Datetime: fmt.Sprintf("%s · %02d:%02d", t.Format("Jan 2, 2006"), hour, min),
	}
}

func humanizeChatHeaderSlug(slug string) string {
	slug = strings.TrimSpace(slug)
	if slug == "" {
		return ""
	}
	replacer := strings.NewReplacer("-", " ", "_", " ")
	words := strings.Fields(replacer.Replace(slug))
	for i, w := range words {
		runes := []rune(strings.ToLower(w))
		if len(runes) == 0 {
			continue
		}
		runes[0] = unicode.ToUpper(runes[0])
		words[i] = string(runes)
	}
	return strings.Join(words, " ")
}
