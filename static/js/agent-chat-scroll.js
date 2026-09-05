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
  // Sibling-doc continuity restored a mid-scroll position — don't clobber it
  // unless a caller explicitly forces (mobile tab reveal).
  if (!force && region.dataset.scrollRestored === "true") return;
  if (force || region.dataset.follow === "true" || isNearBottom(region)) {
    region.scrollTop = region.scrollHeight;
    region.dataset.follow = "true";
    delete region.dataset.scrollRestored;
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

function regionIsVisible(region) {
  if (!region) return false;
  if (region.clientHeight <= 0) return false;
  const style = window.getComputedStyle(region);
  if (style.display === "none" || style.visibility === "hidden") return false;
  // Ancestor may be max-md:!hidden while chat tab is inactive.
  let node = region;
  while (node && node !== document.documentElement) {
    const cs = window.getComputedStyle(node);
    if (cs.display === "none") return false;
    node = node.parentElement;
  }
  return true;
}

function bindVisibilityScroll(region) {
  if (!region || region.dataset.visibilityScrollBound === "true") return;
  region.dataset.visibilityScrollBound = "true";
  let wasVisible = regionIsVisible(region);
  const maybeScroll = () => {
    const visible = regionIsVisible(region);
    if (visible && !wasVisible) {
      // Chat mobile tab just became visible — land on latest.
      scrollAgentChatToBottom(region.parentElement || document, {
        force: true,
      });
    }
    wasVisible = visible;
  };
  if (typeof IntersectionObserver === "function") {
    const io = new IntersectionObserver(
      () => {
        maybeScroll();
      },
      { threshold: 0.01 },
    );
    io.observe(region);
  }
  const mo = new MutationObserver(() => maybeScroll());
  const root = document.getElementById("workbench-root") || document.body;
  mo.observe(root, {
    attributes: true,
    subtree: true,
    attributeFilter: ["class", "data-workbench-mobile-active", "style"],
  });
}

function init(event) {
  initAgentChatInitialScroll(event?.target || document);
  const regions = document.querySelectorAll("#agent-chat-scroll-region");
  for (const region of regions) {
    bindVisibilityScroll(region);
  }
}

init();
document.addEventListener("DOMContentLoaded", init);
document.addEventListener("datastar-patch-elements", init);
new MutationObserver(() => init()).observe(document.documentElement, {
  childList: true,
  subtree: true,
});
