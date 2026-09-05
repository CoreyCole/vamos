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

function bindFollow(region) {
  if (!region || region.dataset.followBound === "true") return;
  region.dataset.followBound = "true";
  if (!region.dataset.follow) region.dataset.follow = "true";
  region.addEventListener(
    "scroll",
    () => {
      region.dataset.follow = String(isNearBottom(region));
    },
    { passive: true },
  );
}

function pinScrollToBottom(region) {
  if (!region) return false;
  const top = region.scrollHeight;
  region.scrollTop = top;
  try {
    region.scrollTo(0, top);
  } catch (_) {}
  // iOS sometimes needs a content anchor when scrollHeight was just growing.
  const last = region.lastElementChild;
  if (
    last &&
    region.scrollHeight - region.scrollTop - region.clientHeight > 40
  ) {
    try {
      last.scrollIntoView({ block: "end", inline: "nearest" });
    } catch (_) {}
  }
  return region.scrollHeight - region.scrollTop - region.clientHeight < 80;
}

export function scrollAgentChatToBottom(
  root = document,
  { force = false } = {},
) {
  const region = agentChatScrollRegion(root);
  if (!region) return false;
  bindFollow(region);
  if (hasMessageHash() && !force) return false;
  // Sibling-doc continuity restored a mid-scroll position — don't clobber it
  // unless a caller explicitly forces (mobile tab reveal / first open).
  if (!force && region.dataset.scrollRestored === "true") return false;
  if (force || region.dataset.follow === "true" || isNearBottom(region)) {
    pinScrollToBottom(region);
    region.dataset.follow = "true";
    delete region.dataset.scrollRestored;
    return true;
  }
  return false;
}

export function scheduleAgentChatScrollToLatest(
  root = document,
  { force = true } = {},
) {
  const region = agentChatScrollRegion(root);
  if (!region) return;
  const run = () => scrollAgentChatToBottom(root, { force });
  run();
  requestAnimationFrame(() => {
    run();
    requestAnimationFrame(run);
  });
  for (const ms of [50, 150, 350, 700]) {
    setTimeout(run, ms);
  }
}

export function initAgentChatInitialScroll(root = document) {
  const regions = root.querySelectorAll?.("#agent-chat-scroll-region") || [];
  for (const region of regions) {
    bindFollow(region);
    bindResizeFollow(region);
    bindVisibilityScroll(region);
    if (regionIsVisible(region)) {
      scheduleAgentChatScrollToLatest(region.parentElement || document, {
        force: true,
      });
    } else {
      // Hidden on mobile Docs-default: mark that first reveal should pin latest
      // unless sibling-nav restore already claimed the scroll.
      if (region.dataset.scrollRestored !== "true") {
        region.dataset.pendingLatest = "true";
      }
    }
  }
}

function bindResizeFollow(region) {
  if (!region || region.dataset.resizeScrollBound === "true") return;
  region.dataset.resizeScrollBound = "true";
  if (typeof ResizeObserver !== "function") return;
  const ro = new ResizeObserver(() => {
    if (!regionIsVisible(region)) return;
    if (region.dataset.follow === "true") {
      pinScrollToBottom(region);
    }
  });
  ro.observe(region);
  if (region.firstElementChild) {
    ro.observe(region.firstElementChild);
  }
}

function bindVisibilityScroll(region) {
  if (!region || region.dataset.visibilityScrollBound === "true") return;
  region.dataset.visibilityScrollBound = "true";
  let wasVisible = regionIsVisible(region);
  const maybeScroll = () => {
    const visible = regionIsVisible(region);
    if (visible && !wasVisible) {
      // Chat mobile tab just became visible — land on latest unless an
      // intentional sibling-nav restore already applied a mid position.
      if (region.dataset.scrollRestored === "true") {
        region.dataset.pendingLatest = "false";
      } else {
        region.dataset.pendingLatest = "false";
        scheduleAgentChatScrollToLatest(region.parentElement || document, {
          force: true,
        });
      }
    }
    wasVisible = visible;
  };
  if (typeof IntersectionObserver === "function") {
    const io = new IntersectionObserver(
      () => {
        maybeScroll();
      },
      { threshold: [0, 0.01, 0.1] },
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
}

init();
document.addEventListener("DOMContentLoaded", init);
document.addEventListener("datastar-patch-elements", init);
new MutationObserver(() => init()).observe(document.documentElement, {
  childList: true,
  subtree: true,
});
