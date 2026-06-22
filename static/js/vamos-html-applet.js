const allowedVamosThemes = new Set(["dark", "light"]);

function normalizeVamosTheme(theme) {
  return allowedVamosThemes.has(theme) ? theme : "";
}

function applyVamosTheme(theme) {
  const normalized = normalizeVamosTheme(theme);
  if (!normalized) return;
  document.documentElement.classList.toggle("dark", normalized === "dark");
}

function themeFromQuery(search) {
  return new URLSearchParams(search).get("theme") || "";
}

function handleVamosThemeMessage(event) {
  const data = event.data;
  if (!data || data.type !== "vamos:theme") return;
  applyVamosTheme(data.theme);
}

applyVamosTheme(themeFromQuery(window.location.search));
window.addEventListener("message", handleVamosThemeMessage);

export {
  normalizeVamosTheme,
  applyVamosTheme,
  themeFromQuery,
  handleVamosThemeMessage,
};
