function revalidateRestoredThread(event) {
  // Native Back/Forward (including bfcache via event.persisted) must stay a
  // traverse so the browser can restore without a white flash.
  if (
    event.persisted &&
    document.querySelector("#workbench-v2-chat-body #agent-chat-composer-form")
  ) {
    return;
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
