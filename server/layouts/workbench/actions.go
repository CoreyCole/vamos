package workbench

import (
	"context"
	"io"
	"sort"
	"strings"

	"github.com/a-h/templ"

	"github.com/CoreyCole/vamos/pkg/datastarui/components/toast"
)

type OverflowActionKind string

type OverflowActionSubmitMode string

const (
	OverflowActionButton OverflowActionKind = "button"
	OverflowActionLink   OverflowActionKind = "link"
	OverflowActionForm   OverflowActionKind = "form"

	OverflowActionSubmitNative   OverflowActionSubmitMode = "native"
	OverflowActionSubmitDatastar OverflowActionSubmitMode = "datastar"
)

type OverflowAction struct {
	Label        string
	Description  string
	Kind         OverflowActionKind
	Href         string
	FormAction   string
	FormMethod   string
	SubmitMode   OverflowActionSubmitMode
	Target       string
	Rel          string
	ClientAction string
	TestID       string
	HiddenFields map[string]string
	Disabled     bool
}

type OverflowActionGroup struct {
	Label   string
	Actions []OverflowAction
}

type OverflowActionsArgs struct {
	Label  string
	Groups []OverflowActionGroup
}

func overflowActionLabel(action OverflowAction) string {
	return strings.TrimSpace(action.Label)
}

func overflowFormMethod(action OverflowAction) string {
	method := strings.ToLower(strings.TrimSpace(action.FormMethod))
	if method == "" {
		return "post"
	}
	return method
}

func overflowCloseMenuExpr() string {
	return "el.closest('[data-overflow-menu]')?.style.setProperty('display','none')"
}

func overflowDatastarSubmit(action OverflowAction) string {
	formAction := strings.TrimSpace(action.FormAction)
	if formAction == "" {
		return ""
	}
	return overflowCloseMenuExpr() + "; @post('" + escapeDatastarString(
		formAction,
	) + "', {contentType: 'form'})"
}

func overflowHiddenFieldNames(fields map[string]string) []string {
	names := make([]string, 0, len(fields))
	for name := range fields {
		names = append(names, name)
	}
	sort.Strings(names)
	return names
}

func escapeDatastarString(value string) string {
	value = strings.ReplaceAll(value, `\`, `\\`)
	return strings.ReplaceAll(value, `'`, `\'`)
}

// ChatHeaderShareOverflow is the desktop chat-header ⋯ fallback (Share artifact / Share chat)
// when callers pass nil overflow. Prefer markdown.BuildChatHeaderOverflow for full flat menu.
func ChatHeaderShareOverflow() templ.Component {
	show := toast.ShowToastExpr("clipboard_success", 2000)
	closeMenu := "el.closest('[data-overflow-menu]')?.style.setProperty('display','none')"
	return OverflowActions(OverflowActionsArgs{
		Label: "Share",
		Groups: []OverflowActionGroup{{
			Actions: []OverflowAction{
				{
					Label:        "Share artifact",
					Kind:         OverflowActionButton,
					ClientAction: "navigator.clipboard.writeText('thoughts/').then(() => { " + show + " }); " + closeMenu,
				},
				{
					Label:        "Share chat",
					Kind:         OverflowActionButton,
					ClientAction: "navigator.clipboard.writeText(location.href.replace(/#.*$/, '')).then(() => { " + show + " }); " + closeMenu,
				},
			},
		}},
	})
}

func OverflowActionsScript() templ.Component {
	return templ.ComponentFunc(func(ctx context.Context, w io.Writer) error {
		_, err := io.WriteString(w, `<script data-overflow-chrome="1">
(() => {
  function closeMenu(menu) {
    if (menu) menu.style.display = "none";
  }
  function placeMenu(trigger, menu) {
    if (!trigger || !menu) return;
    if (menu.parentElement !== document.body) document.body.appendChild(menu);
    menu.style.display = "block";
    menu.style.position = "fixed";
    menu.style.zIndex = "1000";
    const r = trigger.getBoundingClientRect();
    const w = menu.offsetWidth || 192;
    const pad = 8;
    menu.style.top = (r.bottom + 4) + "px";
    menu.style.left = Math.max(pad, Math.min(r.right - w, window.innerWidth - w - pad)) + "px";
  }
  function bindRoot(root) {
    if (!root || root.dataset.overflowBound) return;
    root.dataset.overflowBound = "1";
    const trigger = root.querySelector("[data-overflow-trigger]");
    const menu = root.querySelector("[data-overflow-menu]");
    if (!trigger || !menu) return;
    trigger.addEventListener("click", (evt) => {
      evt.preventDefault();
      evt.stopPropagation();
      const open = menu.style.display === "block";
      document.querySelectorAll("[data-overflow-menu]").forEach((node) => closeMenu(node));
      if (!open) placeMenu(trigger, menu);
    });
    menu.addEventListener("click", (evt) => {
      if (evt.target.closest("a,button")) closeMenu(menu);
    });
  }
  document.querySelectorAll("[data-overflow-root]").forEach(bindRoot);
  if (document.documentElement.dataset.overflowChromeBound) return;
  document.documentElement.dataset.overflowChromeBound = "1";
  document.addEventListener("click", (evt) => {
    if (evt.target.closest("[data-overflow-root], [data-overflow-menu]")) return;
    document.querySelectorAll("[data-overflow-menu]").forEach((node) => closeMenu(node));
  });
  document.addEventListener("keydown", (evt) => {
    if (evt.key === "Escape") {
      document.querySelectorAll("[data-overflow-menu]").forEach((node) => closeMenu(node));
    }
  });
})();
</script>`)
		return err
	})
}

// ArtifactReloadJS is the shared iframe.src reset body (icon click, pane PTR, vamos:ptr).
// Restart (process) is separate — this never posts /forms/applets/*/restart.
// Debounces in-flight reloads via window.__vamosArtifactReloadBusy (load once + 2s fallback).
func ArtifactReloadJS() string {
	return `if(window.__vamosArtifactReloadBusy)return;var host=document.getElementById("thread-artifact-document")||document.getElementById("workbench-v2-artifact-body");if(!host)return;var f=host.querySelector("iframe[data-vamos-html-applet],[id^=\"applet-frame-\"] iframe,iframe");if(!f)return;window.__vamosArtifactReloadBusy=true;var done=function(){window.__vamosArtifactReloadBusy=false;};f.addEventListener("load",done,{once:true});setTimeout(done,2000);var s=f.getAttribute("src")||f.src;f.src=s;`
}

// ArtifactReloadClickAction reassigns the artifact iframe src (full document reload).
func ArtifactReloadClickAction() string {
	return `(function(){` + ArtifactReloadJS() + `})()`
}

// ArtifactReloadOverflowAction is Reload in the path-header kebab (not chat ⋯).
func ArtifactReloadOverflowAction() OverflowAction {
	return OverflowAction{
		Label:        "Reload",
		Kind:         OverflowActionButton,
		ClientAction: ArtifactReloadClickAction() + "; " + overflowCloseMenuExpr(),
		TestID:       "artifact-reload",
	}
}

// ArtifactReloadPanePTRBootstrap installs parent-owned pane PTR + allowlisted vamos:ptr listener.
// Idempotent (window.__vamosArtifactPTRBound). Does not intercept #agent-chat-scroll-region.
func ArtifactReloadPanePTRBootstrap() string {
	return `(function(){
  if (window.__vamosArtifactPTRBound) return;
  window.__vamosArtifactPTRBound = true;
  function reloadArtifact(){` + ArtifactReloadJS() + `}
  function artifactIframe(){
    var host=document.getElementById("thread-artifact-document")||document.getElementById("workbench-v2-artifact-body");
    if(!host)return null;
    return host.querySelector("iframe[data-vamos-html-applet],[id^=\"applet-frame-\"] iframe,iframe");
  }
  function bindPane(pane){
    if(!pane || pane.dataset.vamosPtrBound) return;
    pane.dataset.vamosPtrBound = "1";
    pane.style.overscrollBehaviorY = "contain";
    var doc = document.getElementById("thread-artifact-document");
    if (doc) doc.style.overscrollBehaviorY = "contain";
    var startY = 0, pulling = false, armed = false;
    var threshold = 56;
    function scrollTopAtParent(){
      var el = document.getElementById("thread-artifact-document") || pane;
      return !el || el.scrollTop <= 0;
    }
    pane.addEventListener("touchstart", function(evt){
      if (!evt.touches || evt.touches.length !== 1) return;
      if (evt.target && evt.target.closest && evt.target.closest("#agent-chat-scroll-region")) return;
      armed = scrollTopAtParent();
      startY = evt.touches[0].clientY;
      pulling = false;
    }, {passive: true});
    pane.addEventListener("touchmove", function(evt){
      if (!armed || !evt.touches || evt.touches.length !== 1) return;
      if (!scrollTopAtParent()) { armed = false; return; }
      var dy = evt.touches[0].clientY - startY;
      if (dy > threshold) pulling = true;
    }, {passive: true});
    pane.addEventListener("touchend", function(){
      if (pulling && armed) reloadArtifact();
      armed = false; pulling = false;
    }, {passive: true});
  }
  function bindAll(){
    document.querySelectorAll("#thread-artifact-pane").forEach(bindPane);
  }
  bindAll();
  window.addEventListener("message", function(e){
    if (!e || !e.data || e.data.type !== "vamos:ptr") return;
    var f = artifactIframe();
    if (!f || e.source !== f.contentWindow) return;
    reloadArtifact();
  });
  if (typeof MutationObserver !== "undefined") {
    var mo = new MutationObserver(function(){ bindAll(); });
    mo.observe(document.documentElement, {childList: true, subtree: true});
  }
})();`
}

// ArtifactReloadPTRScript injects the one-shot pane PTR + vamos:ptr bootstrap (not /static).
func ArtifactReloadPTRScript() templ.Component {
	return templ.ComponentFunc(func(ctx context.Context, w io.Writer) error {
		_, err := io.WriteString(
			w,
			`<script data-vamos-artifact-ptr="1">`+"\n"+ArtifactReloadPanePTRBootstrap()+"\n</script>",
		)
		return err
	})
}
