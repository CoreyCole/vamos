function agentChatScrollRegion(root = document) {
  return (
    root.querySelector?.("#agent-chat-scroll-region") ||
    document.getElementById("agent-chat-scroll-region")
  );
}

function isNearBottom(region) {
  return region.scrollHeight - region.scrollTop - region.clientHeight < 80;
}

function hasMessageHash() {
  return window.location.hash?.startsWith("#msg-");
}

function bindFollow(region) {
  if (!region || region.dataset.followBound === "true") return;
  region.dataset.followBound = "true";
  if (!region.dataset.follow) region.dataset.follow = "true";
  region.addEventListener("scroll", () => {
    region.dataset.follow = String(isNearBottom(region));
  });
}

export function scrollAgentChatToBottom(
  root = document,
  { force = false } = {},
) {
  const region = agentChatScrollRegion(root);
  if (!region) return;
  bindFollow(region);
  if (hasMessageHash() && !force) return;
  if (force || region.dataset.follow === "true" || isNearBottom(region)) {
    region.scrollTop = region.scrollHeight;
    region.dataset.follow = "true";
  }
}

export function initAgentChatInitialScroll(root = document) {
  const regions = root.querySelectorAll?.("#agent-chat-scroll-region") || [];
  for (const region of regions) {
    bindFollow(region);
    requestAnimationFrame(() => {
      scrollAgentChatToBottom(region.parentElement || document);
      requestAnimationFrame(() => {
        scrollAgentChatToBottom(region.parentElement || document);
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
