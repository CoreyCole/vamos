const initialized = new WeakSet();

export function scrollAgentChatToBottom(root = document) {
  const region =
    root.querySelector?.("#agent-chat-scroll-region") ||
    document.getElementById("agent-chat-scroll-region");
  if (!region) return;
  region.scrollTop = region.scrollHeight;
}

export function initAgentChatInitialScroll(root = document) {
  const regions = root.querySelectorAll?.("#agent-chat-scroll-region") || [];
  for (const region of regions) {
    if (initialized.has(region)) continue;
    initialized.add(region);
    requestAnimationFrame(() => {
      region.scrollTop = region.scrollHeight;
      requestAnimationFrame(() => {
        region.scrollTop = region.scrollHeight;
      });
    });
  }
}

function init(event) {
  initAgentChatInitialScroll(event?.target || document);
}

init();
document.addEventListener("DOMContentLoaded", init);
document.addEventListener("datastar-patch-elements", init);
new MutationObserver(() => init()).observe(document.documentElement, {
  childList: true,
  subtree: true,
});
