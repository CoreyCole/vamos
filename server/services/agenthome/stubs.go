package agenthome

import (
	"context"
	"io"

	"github.com/a-h/templ"
)

// StubPane is a minimal pane body for AI-470 room sketches.
func StubPane(title, subtitle, href, body string) templ.Component {
	return templ.ComponentFunc(func(ctx context.Context, w io.Writer) error {
		_, err := io.WriteString(
			w,
			`<div class="flex h-full min-h-0 flex-col gap-2 p-4">`,
		)
		if err != nil {
			return err
		}
		_, err = io.WriteString(
			w,
			`<div class="text-xs font-semibold uppercase tracking-wide text-muted-foreground">`,
		)
		if err != nil {
			return err
		}
		_, err = io.WriteString(w, templ.EscapeString(title))
		if err != nil {
			return err
		}
		_, err = io.WriteString(w, `</div>`)
		if err != nil {
			return err
		}
		if subtitle != "" {
			_, err = io.WriteString(w, `<div class="text-sm text-muted-foreground">`)
			if err != nil {
				return err
			}
			_, err = io.WriteString(w, templ.EscapeString(subtitle))
			if err != nil {
				return err
			}
			_, err = io.WriteString(w, `</div>`)
			if err != nil {
				return err
			}
		}
		if href != "" {
			_, err = io.WriteString(w, `<p><a class="text-sm underline" href="`)
			if err != nil {
				return err
			}
			_, err = io.WriteString(w, templ.EscapeString(href))
			if err != nil {
				return err
			}
			_, err = io.WriteString(w, `">Back to roster</a></p>`)
			if err != nil {
				return err
			}
		}
		_, err = io.WriteString(w, `<p class="text-sm text-foreground">`)
		if err != nil {
			return err
		}
		_, err = io.WriteString(w, templ.EscapeString(body))
		if err != nil {
			return err
		}
		_, err = io.WriteString(w, `</p></div>`)
		return err
	})
}

func roomComposerPlaceholder(_ RoomKind, _, title string) string {
	if title != "" {
		return "Message " + title
	}
	return "Message…"
}

func artPreviewInitial(RoomKind, string) string {
	return ""
}

func RosterChromeScript() templ.Component {
	return templ.ComponentFunc(func(ctx context.Context, w io.Writer) error {
		_, err := io.WriteString(w, `<script data-roster-chrome="1">
(() => {
  const root = document.getElementById("workbench-v2-roster");
  const menu = document.getElementById("workbench-v2-roster-context-menu");
  const sheet = document.getElementById("workbench-v2-edit-profile-sheet");
  if (!root || !menu || !sheet || root.dataset.rosterChromeBound) return;
  root.dataset.rosterChromeBound = "1";

  function rosterIds() {
    const ids = [];
    root.querySelectorAll("a[data-roster-id]").forEach((a) => {
      const id = a.getAttribute("data-roster-id");
      if (id && !ids.includes(id)) ids.push(id);
    });
    return ids;
  }
  function rosterMeta(id) {
    const a = root.querySelector('a[data-roster-id="' + id + '"]');
    const title = (a && (a.getAttribute("data-roster-title") || a.textContent || id).trim()) || id;
    return {
      name: title,
      initial: title ? title.slice(0, 1).toUpperCase() : "?",
      bg: "bg-muted",
      href: a ? a.getAttribute("href") : "#"
    };
  }
  let selected = new Set();
  let anchor = null;
  let focusId = rosterIds()[0] || "";
  const PIN_KEY = "wb2-roster-pinned";
  function loadPinned() {
    try {
      const raw = sessionStorage.getItem(PIN_KEY);
      if (raw) {
        const parsed = JSON.parse(raw);
        if (Array.isArray(parsed)) return new Set(parsed);
      }
    } catch (_) {}
    return new Set();
  }
  let pinned = loadPinned();
  function savePinned() {
    try { sessionStorage.setItem(PIN_KEY, JSON.stringify([...pinned])); } catch (_) {}
  }

  function rowEl(id) {
    const [kind, rid] = id.split(":");
    return document.getElementById("roster-row-" + kind + "-" + rid);
  }
  function pinEl(id) {
    return root.querySelector('#workbench-v2-roster-pins a[data-roster-id="' + id + '"]');
  }
  function paintPins() {
    for (const id of rosterIds()) {
      const pin = pinEl(id);
      if (pin) pin.classList.toggle("hidden", !pinned.has(id));
      const li = rowEl(id);
      if (li) li.classList.toggle("hidden", pinned.has(id));
    }
    const pinLabel = menu.querySelector('[data-roster-label="pin"]');
    if (pinLabel) pinLabel.textContent = pinned.has(focusId) ? "Unpin" : "Pin";
  }
  function paint() {
    for (const id of rosterIds()) {
      const li = rowEl(id);
      if (!li) continue;
      const a = li.querySelector("a[data-roster-id]");
      if (!a) continue;
      const on = selected.has(id);
      a.classList.toggle("roster-multi-selected", on);
      a.setAttribute("aria-selected", on ? "true" : "false");
    }
    const n = Math.max(selected.size, 1);
    const move = menu.querySelector('[data-roster-label="move"]');
    const hide = menu.querySelector('[data-roster-label="hide"]');
    const del = menu.querySelector('[data-roster-label="delete"]');
    if (move) move.textContent = n === 1 ? "Move to" : ("Move " + n + " Bots to");
    if (hide) hide.textContent = n === 1 ? "Hide from sidebar" : ("Hide " + n + " Bots from sidebar");
    if (del) del.textContent = n === 1 ? "Delete" : ("Delete " + n + " Bots");
  }
  function ensurePortaled(el) {
    if (!el) return el;
    // Morph can re-SSR a duplicate under the clipped left region; keep one on body.
    if (el.id) {
      document.querySelectorAll("#" + CSS.escape(el.id)).forEach((node) => {
        if (node !== el) node.remove();
      });
    }
    if (el.parentElement !== document.body) document.body.appendChild(el);
    el.style.position = "fixed";
    return el;
  }
  function liveMenu() {
    return ensurePortaled(document.getElementById("workbench-v2-roster-context-menu") || menu);
  }
  function liveSheet() {
    return ensurePortaled(document.getElementById("workbench-v2-edit-profile-sheet") || sheet);
  }
  // Escape the VT/overflow containing block before first interaction.
  ensurePortaled(menu);
  ensurePortaled(sheet);
  function hideMenu() {
    const m = liveMenu();
    if (m) m.style.display = "none";
  }
  function openMenu(x, y) {
    // Portal out of #workbench-v2-threads (view-transition-name + overflow clips fixed).
    const m = liveMenu();
    if (!m) return;
    m.style.display = "block";
    m.style.zIndex = "1000";
    m.style.position = "fixed";
    const pad = 8;
    const w = m.offsetWidth || 220;
    const h = m.offsetHeight || 280;
    m.style.left = Math.min(x, window.innerWidth - w - pad) + "px";
    m.style.top = Math.min(y, window.innerHeight - h - pad) + "px";
  }
  function openProfile() {
    const s = liveSheet();
    if (!s) return;
    const m = rosterMeta(focusId);
    const avatar = document.getElementById("workbench-v2-edit-profile-avatar");
    const name = document.getElementById("workbench-v2-edit-profile-name");
    if (avatar) { avatar.textContent = m.initial; avatar.className = "flex h-14 w-14 items-center justify-center rounded-2xl text-lg font-semibold text-white " + m.bg; }
    if (name) name.value = m.name;
    s.classList.remove("hidden");
    s.classList.add("flex");
    hideMenu();
  }
  function closeProfile() {
    const s = liveSheet();
    if (!s) return;
    s.classList.add("hidden");
    s.classList.remove("flex");
  }

  root.addEventListener("click", (evt) => {
    const a = evt.target.closest("a[data-roster-id]");
    if (!a || !root.contains(a)) return;
    const id = a.getAttribute("data-roster-id");
    if (!id) return;
    if (evt.metaKey || evt.ctrlKey) {
      evt.preventDefault();
      if (selected.has(id)) selected.delete(id); else selected.add(id);
      anchor = id;
      focusId = id;
      paint();
      return;
    }
    if (evt.shiftKey && anchor) {
      evt.preventDefault();
      const ids = rosterIds();
      const i0 = ids.indexOf(anchor);
      const i1 = ids.indexOf(id);
      if (i0 >= 0 && i1 >= 0) {
        const lo = Math.min(i0, i1), hi = Math.max(i0, i1);
        selected = new Set(ids.slice(lo, hi + 1));
        focusId = id;
        paint();
      }
      return;
    }
    selected = new Set([id]);
    anchor = id;
    focusId = id;
    paint();
  }, true);

  root.addEventListener("contextmenu", (evt) => {
    const a = evt.target.closest("a[data-roster-id]");
    if (!a || !root.contains(a)) return;
    evt.preventDefault();
    const id = a.getAttribute("data-roster-id");
    focusId = id;
    if (!selected.has(id)) {
      selected = new Set([id]);
      anchor = id;
    }
    paint();
    paintPins();
    openMenu(evt.clientX, evt.clientY);
  });

  document.addEventListener("click", (evt) => {
    if (!menu.contains(evt.target)) hideMenu();
  });
  document.addEventListener("keydown", (evt) => {
    if (evt.key === "Escape") { hideMenu(); closeProfile(); }
  });

  menu.addEventListener("click", (evt) => {
    const btn = evt.target.closest("[data-roster-action]");
    if (!btn) return;
    const action = btn.getAttribute("data-roster-action");
    if (action === "edit-profile") {
      openProfile();
      return;
    }
    if (action === "pin" && focusId) {
      if (pinned.has(focusId)) pinned.delete(focusId);
      else if (pinEl(focusId)) pinned.add(focusId);
      savePinned();
      paintPins();
    }
    hideMenu();
  });

  document.getElementById("workbench-v2-edit-profile-close")?.addEventListener("click", closeProfile);
  document.getElementById("workbench-v2-edit-profile-save")?.addEventListener("click", closeProfile);
  sheet.addEventListener("click", (evt) => { if (evt.target === sheet) closeProfile(); });
  const notify = document.getElementById("workbench-v2-edit-profile-notify");
  notify?.addEventListener("click", () => {
    const on = notify.getAttribute("aria-pressed") === "true";
    notify.setAttribute("aria-pressed", on ? "false" : "true");
    notify.classList.toggle("bg-primary", !on);
    notify.classList.toggle("bg-muted", on);
    const knob = notify.querySelector("span");
    if (knob) knob.style.transform = on ? "translateX(0)" : "translateX(20px)";
  });

  paint();
  paintPins();
})();
</script>`)
		return err
	})
}
