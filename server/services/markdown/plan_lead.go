package markdown

import (
	"net/url"
	"path/filepath"
	"strings"
)

func planLeadRoomID(docPath string) string {
	path := filepath.ToSlash(strings.TrimSpace(docPath))
	path = strings.Trim(path, "/")
	if path == "" {
		return ""
	}
	if i := strings.Index(path, "thoughts/"); i >= 0 {
		path = path[i:]
	} else if !strings.HasPrefix(path, "thoughts/") {
		path = "thoughts/" + path
	}
	parts := strings.Split(path, "/")
	for i := 0; i+3 < len(parts); i++ {
		if parts[i] != "thoughts" || parts[i+2] != "plans" {
			continue
		}
		id := strings.TrimSpace(parts[i+3])
		if id == "" || id == "." {
			return ""
		}
		return id
	}
	return ""
}

func planLeadChatHref(docPath string) string {
	id := planLeadRoomID(docPath)
	if id == "" {
		return ""
	}
	href := "/rooms/plan/" + url.PathEscape(id)
	canonical, err := CanonicalThoughtsDocPath(docPath)
	if err != nil {
		canonical, err = CanonicalThoughtsDirPath(docPath)
		if err != nil {
			return href
		}
	}
	if canonical == "" {
		return href
	}
	return href + "?artifact=" + url.QueryEscape("thoughts/"+canonical)
}
