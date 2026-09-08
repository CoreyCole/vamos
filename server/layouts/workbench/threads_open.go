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

func ThreadsToggleHotkeyAction() string {
	return "if ((evt.ctrlKey || evt.metaKey) && !evt.altKey && !evt.shiftKey && (evt.key === 'b' || evt.key === 'B')) { " +
		"var n = evt.target; if (n && (n.tagName === 'INPUT' || n.tagName === 'TEXTAREA' || n.tagName === 'SELECT' || n.isContentEditable)) { return } " +
		"evt.preventDefault(); " +
		"if ($workbench.regions.workbenchV2Threads.visible === false) { " +
		ThreadsShowClickAction(
			"workbenchV2Threads",
		) +
		" } else { " +
		ThreadsHideClickAction() +
		" } }"
}

func ThreadsHideControlTitle() string {
	return "Hide roster sidebar (Ctrl+B)"
}

func ThreadsShowControlTitle() string {
	return "Show roster sidebar (Ctrl+B)"
}

// ThreadsReopenDataClass hides the hamburger unless threads are explicitly closed.
// Missing/undefined visible must stay hidden so Datastar hydrate cannot flash the
// control open for a frame on thread/room GET (default cookie is threads open).
func ThreadsReopenDataClass() string {
	return "{'hidden': $workbench.regions.workbenchV2Threads.visible !== false}"
}
