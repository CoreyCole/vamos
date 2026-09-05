// Chat transcript uses CSS flex-col-reverse on #agent-chat-scroll-region so the
// browser anchors at latest with zero JS. Kept as a no-op module for stale caches.
export function scrollAgentChatToBottom() {
  return false;
}

export function scheduleAgentChatScrollToLatest() {}

export function initAgentChatInitialScroll() {}
