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

// ArtifactReloadClickAction reassigns the artifact iframe src (full document reload).
// Restart (process) is separate — this never posts /forms/applets/*/restart.
func ArtifactReloadClickAction() string {
	return `(function(){var host=document.getElementById("thread-artifact-document")||document.getElementById("workbench-v2-artifact-body");if(!host)return;var f=host.querySelector("iframe[data-vamos-html-applet],[id^=\"applet-frame-\"] iframe,iframe");if(!f)return;var s=f.getAttribute("src")||f.src;f.src=s;})()`
}
