package workbench

import "net/http"

// ArtifactOpenCookie stores ephemeral desktop artifact-pane visibility across
// same-origin GETs so SSR EncodeWorkbenchSignals keeps
// $workbench.regions.workbenchV2Artifact.visible closed when the user hid it.
// Layout prefs stay ratio-only (ratioOnly / StripDurableInteractionState).
const ArtifactOpenCookie = "wb2_artifact_open"

// ArtifactOpenFromRequest reads wb2_artifact_open; missing/invalid => open.
func ArtifactOpenFromRequest(r *http.Request) bool {
	if r == nil {
		return true
	}
	c, err := r.Cookie(ArtifactOpenCookie)
	if err != nil || (c.Value != "0" && c.Value != "1") {
		return true
	}
	return c.Value == "1"
}

func artifactOpenCookieWriteJS(open bool) string {
	v := "0"
	if open {
		v = "1"
	}
	return "try { document.cookie = '" + ArtifactOpenCookie + "=" + v +
		"; path=/; SameSite=Lax; Max-Age=31536000'; sessionStorage.setItem(" +
		"'workbench-v2:artifact-open', '" + v + "') } catch (e) {}"
}

// ArtifactHideClickAction collapses the artifact pane and persists via cookie.
func ArtifactHideClickAction() string {
	return "$workbench.regions.workbenchV2Artifact.visible = false; " +
		artifactOpenCookieWriteJS(false) + "; " + threadsLayoutReflowJS()
}

// ArtifactShowClickAction reopens the artifact pane and persists via cookie.
func ArtifactShowClickAction() string {
	return "$workbench.regions.workbenchV2Artifact.visible = true; " +
		artifactOpenCookieWriteJS(true) + "; " + threadsLayoutReflowJS()
}

func ArtifactHideControlTitle() string {
	return "Close details"
}

func ArtifactShowControlTitle() string {
	return "Open details"
}

// ArtifactReopenDataClass hides the reopen control unless artifact is explicitly closed.
func ArtifactReopenDataClass() string {
	return "{'hidden': $workbench.regions.workbenchV2Artifact.visible !== false}"
}

func artifactOpenAriaHidden(open bool) string {
	if open {
		return "true"
	}
	return "false"
}
