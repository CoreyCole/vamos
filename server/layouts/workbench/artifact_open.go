package workbench

import (
	"net/http"
	"time"
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

// WriteArtifactOpenCookie persists desktop artifact visibility for later GETs.
func WriteArtifactOpenCookie(w http.ResponseWriter, open bool) {
	if w == nil {
		return
	}
	v := "0"
	if open {
		v = "1"
	}
	http.SetCookie(w, &http.Cookie{
		Name:     ArtifactOpenCookie,
		Value:    v,
		Path:     "/",
		SameSite: http.SameSiteLaxMode,
		MaxAge:   31536000,
		Expires:  time.Now().Add(365 * 24 * time.Hour),
	})
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

func artifactLayoutReflowJS() string {
	return "if (window.workbenchApplyRegionVisible) { workbenchApplyRegionVisible('workbench-v2-artifact', $workbench.regions.workbenchV2Artifact.visible) }; " +
		threadsLayoutReflowJS()
}

// ArtifactHideClickAction collapses the artifact pane and persists via cookie.
// Hide via applyRegionVisible (display none) + layout reflow so chat flex-grows.
// Do not call workbenchReflow() — that pixel-locks and slides the header toggle.
func ArtifactHideClickAction() string {
	return "$workbench.regions.workbenchV2Artifact.visible = false; " +
		artifactOpenCookieWriteJS(false) + "; " +
		artifactLayoutReflowJS()
}

// ArtifactShowClickAction reopens the artifact pane and persists via cookie.
func ArtifactShowClickAction() string {
	return "$workbench.regions.workbenchV2Artifact.visible = true; " +
		artifactOpenCookieWriteJS(true) + "; " +
		artifactLayoutReflowJS()
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

func ArtifactShowControlTitle() string {
	return "Open details"
}

func ArtifactToggleControlTitle() string {
	return "Details"
}
