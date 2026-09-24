package workbench

import "net/http"

// ChatOpenCookie stores ephemeral chat-pane visibility across same-origin GETs
// so SSR EncodeWorkbenchSignals keeps $workbench.regions.workbenchV2Chat.visible
// closed when the user minimized chat. Layout prefs stay ratio-only.
const ChatOpenCookie = "wb2_chat_open"

// ChatOpenFromRequest reads wb2_chat_open; missing/invalid => open.
func ChatOpenFromRequest(r *http.Request) bool {
	return ChatOpenFromRequestDefault(r, true)
}

// ChatOpenFromRequestDefault reads wb2_chat_open; missing/invalid => defaultOpen.
func ChatOpenFromRequestDefault(r *http.Request, defaultOpen bool) bool {
	if r == nil {
		return defaultOpen
	}
	c, err := r.Cookie(ChatOpenCookie)
	if err != nil || (c.Value != "0" && c.Value != "1") {
		return defaultOpen
	}
	return c.Value == "1"
}

// ChatOpenFromState is the SSR chat-open bool used to coalesce Datastar
// $workbench.regions.workbenchV2Chat.visible before the first hydrate tick.
func ChatOpenFromState(state WorkbenchState) bool {
	for _, region := range state.Regions {
		if region.ID == WorkbenchV2ChatRegionID {
			return region.Visible
		}
	}
	return true
}

// ChatVisibleCoalesce keeps data-class from treating undefined as false.
func ChatVisibleCoalesce(ssrOpen bool) string {
	lit := "false"
	if ssrOpen {
		lit = "true"
	}
	return "$workbench.regions.workbenchV2Chat.visible ?? " + lit
}

func ChatToggleDataClass(ssrOpen bool) string {
	return "{ 'bg-muted text-foreground': " + ChatVisibleCoalesce(ssrOpen) + " }"
}

func ChatToggleAriaPressed(ssrOpen bool) string {
	return ChatVisibleCoalesce(ssrOpen) + " ? 'true' : 'false'"
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
