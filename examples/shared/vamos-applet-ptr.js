/**
 * Vamos Applet Pull-to-Refresh Bootstrap
 *
 * Lightweight touch-based pull-to-refresh for demo applets.
 * Lives in the iframe; no parent dependency.
 *
 * Default action: location.reload() for honest SSR + Datastar boot + SSE.
 * Applets may override by setting window.VamosApplet.onPullRefresh before this script runs.
 *
 * Usage:
 *   <script src="shared/vamos-applet-ptr.js"></script>
 * or inline this file's contents in your page template.
 */

(function () {
  'use strict';

  // Initialize namespace
  window.VamosApplet = window.VamosApplet || {};

  // Pull-to-refresh state
  let startY = 0;
  let currentY = 0;
  let pulling = false;
  const PULL_THRESHOLD = 80; // pixels to trigger refresh
  const SCROLL_TOLERANCE = 5; // pixels of scroll tolerance for detecting scrollTop≈0

  // Find the main scrollable element
  function getScrollElement() {
    // Typically the <main> element or body
    const main = document.querySelector('main');
    if (main) return main;
    return document.documentElement || document.body;
  }

  function isAtTop() {
    const el = getScrollElement();
    const candidates = [
      el && el.scrollTop,
      window.pageYOffset,
      document.documentElement && document.documentElement.scrollTop,
      document.body && document.body.scrollTop,
    ];
    const scrollTop = Math.min(
      ...candidates.map((v) => (typeof v === 'number' && !Number.isNaN(v) ? v : Number.POSITIVE_INFINITY))
    );
    return scrollTop <= SCROLL_TOLERANCE;
  }

  function handleTouchStart(e) {
    if (!isAtTop()) return;
    startY = e.touches[0].clientY;
    pulling = false;
  }

  function handleTouchMove(e) {
    if (!isAtTop()) {
      pulling = false;
      return;
    }

    currentY = e.touches[0].clientY;
    const pullDistance = currentY - startY;

    if (pullDistance > 10) {
      pulling = true;
    }
  }

  function handleTouchEnd() {
    if (!pulling) return;

    const pullDistance = currentY - startY;
    if (pullDistance >= PULL_THRESHOLD) {
      triggerRefresh();
    }

    pulling = false;
    startY = 0;
    currentY = 0;
  }

  function triggerRefresh() {
    // Call custom handler if defined, otherwise reload
    if (typeof window.VamosApplet.onPullRefresh === 'function') {
      window.VamosApplet.onPullRefresh();
    } else {
      location.reload();
    }
  }

  // Apply soft overscroll containment
  function applyOverscrollContainment() {
    const scrollEl = getScrollElement();
    if (scrollEl && scrollEl.style) {
      scrollEl.style.overscrollBehaviorY = 'contain';
    }
  }

  // Initialize on DOM ready
  function init() {
    applyOverscrollContainment();

    document.addEventListener('touchstart', handleTouchStart, { passive: true });
    document.addEventListener('touchmove', handleTouchMove, { passive: true });
    document.addEventListener('touchend', handleTouchEnd, { passive: true });
  }

  // Run on DOMContentLoaded or immediately if already loaded
  if (document.readyState === 'loading') {
    document.addEventListener('DOMContentLoaded', init);
  } else {
    init();
  }
})();

