function revalidateRestoredThread(event) {
  const navigation = performance.getEntriesByType("navigation")[0];
  const restored = event.persisted || navigation?.type === "back_forward";
  if (
    restored &&
    document.querySelector("#workbench-v2-chat-body #agent-chat-composer-form")
  ) {
    window.location.reload();
  }
}

function reloadThreadArtifactHistory() {
  const threadRoute =
    window.location.pathname === "/threads" ||
    window.location.pathname.startsWith("/threads/");
  if (threadRoute && document.getElementById("thread-artifact-pane")) {
    window.location.reload();
  }
}

window.addEventListener("pageshow", revalidateRestoredThread);
window.addEventListener("popstate", reloadThreadArtifactHistory);
