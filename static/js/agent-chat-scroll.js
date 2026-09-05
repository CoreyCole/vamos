// Chat transcript uses SSR #chat-latest autofocus/focus so the browser
// anchors at latest with zero scroll machinery. Kept as a no-op module for stale caches.
export function scrollAgentChatToBottom() {
  return false;
}

export function scheduleAgentChatScrollToLatest() {}

export function initAgentChatInitialScroll() {}
