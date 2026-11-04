package workbench

import (
	"net/http"
)

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
// Apply DOM visibility before reflow so chat fills without waiting on Datastar.
func ArtifactHideClickAction() string {
	return "$workbench.regions.workbenchV2Artifact.visible = false; " +
		artifactOpenCookieWriteJS(false) + "; " +
		"if (window.workbenchApplyRegionVisible) { workbenchApplyRegionVisible('workbench-v2-artifact', false) }; " +
		"if (window.workbenchReflow) { workbenchReflow() }; " +
		threadsLayoutReflowJS()
}

// ArtifactShowClickAction reopens the artifact pane and persists via cookie.
func ArtifactShowClickAction() string {
	return "$workbench.regions.workbenchV2Artifact.visible = true; " +
		artifactOpenCookieWriteJS(true) + "; " +
		"if (window.workbenchApplyRegionVisible) { workbenchApplyRegionVisible('workbench-v2-artifact', true) }; " +
		"if (window.workbenchReflow) { workbenchReflow() }; " +
		threadsLayoutReflowJS()
}

// ArtifactToggleClickAction opens or closes details and persists the cookie.
func ArtifactToggleClickAction() string {
	return "if ($workbench.regions.workbenchV2Artifact.visible === false) { " +
		ArtifactShowClickAction() +
		" } else { " +
		ArtifactHideClickAction() +
		" }"
}

func ArtifactHideControlTitle() string {
	return "Close details"
}

func ArtifactToggleControlTitle() string {
	return "Details"
}
