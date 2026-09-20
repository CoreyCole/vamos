const allowedVamosThemes = new Set(["dark", "light"]);

function normalizeVamosTheme(theme) {
  return allowedVamosThemes.has(theme) ? theme : "";
}

function applyVamosTheme(theme) {
  const normalized = normalizeVamosTheme(theme);
  if (!normalized) return;
  document.documentElement.classList.toggle("dark", normalized === "dark");
  document.documentElement.style.colorScheme = normalized;
}

function themeFromQuery(search) {
  return new URLSearchParams(search).get("theme") || "";
}

function handleVamosThemeMessage(event) {
  const data = event.data;
  if (!data || data.type !== "vamos:theme") return;
  applyVamosTheme(data.theme);
}

function thoughtsPageHrefForParent(href, currentHref) {
  let target;
  let current;
  try {
    target = new URL(href, currentHref);
    current = new URL(currentHref);
  } catch {
    return "";
  }
  if (target.origin !== current.origin) return "";
  if (!target.pathname.startsWith("/thoughts/")) return "";
  if (
    target.pathname.startsWith("/thoughts/raw/") ||
    target.pathname.startsWith("/thoughts/_render/") ||
    target.pathname.startsWith("/thoughts/_assets/")
  ) {
    return "";
  }
  if (
    target.pathname === current.pathname &&
    target.search === current.search
  ) {
    return "";
  }
  return target.href;
}

function handleThoughtsPageClick(event) {
  if (
    event.defaultPrevented ||
    event.button !== 0 ||
    event.metaKey ||
    event.ctrlKey ||
    event.shiftKey ||
    event.altKey
  ) {
    return;
  }
  const anchor = event.target?.closest?.("a[href]");
  if (!anchor || anchor.hasAttribute("download")) return;
  const href = thoughtsPageHrefForParent(
    anchor.getAttribute("href"),
    window.location.href,
  );
  if (!href) return;
  event.preventDefault();
  window.top.location.assign(href);
}

applyVamosTheme(themeFromQuery(window.location.search));
window.addEventListener("message", handleVamosThemeMessage);
document.addEventListener("click", handleThoughtsPageClick);

export {
  normalizeVamosTheme,
  applyVamosTheme,
  themeFromQuery,
  handleVamosThemeMessage,
  thoughtsPageHrefForParent,
  handleThoughtsPageClick,
};
