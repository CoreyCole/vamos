package workbench

import "net/http"

// ChatOpenCookie stores ephemeral chat-pane visibility across same-origin GETs
// so SSR EncodeWorkbenchSignals keeps $workbench.regions.workbenchV2Chat.visible
// closed when the user minimized chat. Layout prefs stay ratio-only.
const ChatOpenCookie = "wb2_chat_open"

// ChatOpenFromRequest reads wb2_chat_open; missing/invalid => open.
func ChatOpenFromRequest(r *http.Request) bool {
	if r == nil {
		return true
	}
	c, err := r.Cookie(ChatOpenCookie)
	if err != nil || (c.Value != "0" && c.Value != "1") {
		return true
	}
	return c.Value == "1"
}

func chatOpenCookieWriteJS(open bool) string {
	v := "0"
	if open {
		v = "1"
	}
	return "try { document.cookie = '" + ChatOpenCookie + "=" + v +
		"; path=/; SameSite=Lax; Max-Age=31536000'; sessionStorage.setItem(" +
		"'workbench-v2:chat-open', '" + v + "') } catch (e) {}"
}
