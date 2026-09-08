package workbench

import "net/http"

// ThreadsOpenCookie stores ephemeral desktop threads-sidebar visibility across
// same-origin sibling artifact GETs so SSR EncodeWorkbenchSignals keeps
// $workbench.regions.workbenchV2Threads.visible closed when the user hid it.
// Workbench v2 layout prefs intentionally keep ratios only (ratioOnly /
// StripDurableInteractionState); this cookie mirrors wb2_artifact_browser.
const ThreadsOpenCookie = "wb2_threads_open"

// ThreadsOpenFromRequest reads wb2_threads_open; missing/invalid => open.
func ThreadsOpenFromRequest(r *http.Request) bool {
	if r == nil {
		return true
	}
	c, err := r.Cookie(ThreadsOpenCookie)
	if err != nil || (c.Value != "0" && c.Value != "1") {
		return true
	}
	return c.Value == "1"
}

func threadsOpenCookieWriteJS(open bool) string {
	v := "0"
	if open {
		v = "1"
	}
	return "try { document.cookie = '" + ThreadsOpenCookie + "=" + v +
		"; path=/; SameSite=Lax; Max-Age=31536000'; sessionStorage.setItem(" +
		"'workbench-v2:threads-open', '" + v + "') } catch (e) {}"
}

func threadsLayoutReflowJS() string {
	return "requestAnimationFrame(() => document.getElementById('workbench-root')?.dispatchEvent(new CustomEvent('workbench-layout-reflow')))"
}

// ThreadsHideClickAction collapses the threads sidebar and persists via cookie.
func ThreadsHideClickAction() string {
	return "$workbench.regions.workbenchV2Threads.visible = false; " +
		threadsOpenCookieWriteJS(false) + "; " + threadsLayoutReflowJS()
}

// ThreadsShowClickAction reopens the threads sidebar and persists via cookie.
func ThreadsShowClickAction(signalKey string) string {
	return "$workbench.regions." + signalKey + ".visible = true; " +
		threadsOpenCookieWriteJS(true) + "; " + threadsLayoutReflowJS()
}
