package workbench

import (
	"net/http"
	"strings"
)

const ViewportClassHeader = "X-Vamos-Viewport-Class"

func ResolveViewportClass(header http.Header, userAgent string) ViewportClass {
	if parsed, ok := ParseViewportClass(header.Get(ViewportClassHeader)); ok {
		return parsed
	}
	if parsed, ok := ParseViewportClass(header.Get("Sec-CH-Viewport-Class")); ok {
		return parsed
	}
	if strings.TrimSpace(userAgent) == "" {
		userAgent = header.Get("User-Agent")
	}
	if isMobileUserAgent(userAgent) {
		return ViewportMobile
	}
	return ViewportDesktopFull
}

func isMobileUserAgent(userAgent string) bool {
	userAgent = strings.ToLower(userAgent)
	for _, keyword := range []string{
		"mobile",
		"android",
		"iphone",
		"ipod",
		"ipad",
		"windows phone",
		"blackberry",
		"opera mini",
		"opera mobi",
	} {
		if strings.Contains(userAgent, keyword) {
			return true
		}
	}
	return false
}
