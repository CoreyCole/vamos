function reloadThreadArtifactHistory() {
  const navigation = performance.getEntriesByType("navigation")[0];
  // Same-document popstate after Enter/Up pushState keeps type "navigate".
  // Native document Back/Forward is type "back_forward" -- do not blast it.
  if (navigation && navigation.type === "back_forward") {
    return;
  }
  const threadRoute =
    window.location.pathname === "/threads" ||
    window.location.pathname.startsWith("/threads/");
  if (threadRoute && document.getElementById("thread-artifact-pane")) {
    window.location.reload();
  }
}

window.addEventListener("popstate", reloadThreadArtifactHistory);
