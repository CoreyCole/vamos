const datastarModule = import("@vamos/datastar");

async function mergeWorkbenchPaths(patches) {
  const { mergePaths } = await datastarModule;
  mergePaths(patches);
}

const roots = new WeakSet();

function clamp(value, min, max) {
  return Math.min(Math.max(value, min), max);
}

function allRegions(root) {
  return [...root.querySelectorAll("[data-workbench-region]")];
}

function isVisible(region) {
  return window.getComputedStyle(region).display !== "none";
}

function visibleRegions(root) {
  return allRegions(root).filter(isVisible);
}

function regionBySignal(root, signal) {
  return root.querySelector(`[data-workbench-signal="${CSS.escape(signal)}"]`);
}

function regionsContainer(root) {
  return root.querySelector("#workbench-regions") || root;
}

function visibleHandles(root) {
  return [...root.querySelectorAll("[data-workbench-resize-handle]")].filter(
    (handle) => window.getComputedStyle(handle).display !== "none",
  );
}

function workbenchFocused(root) {
  return (
    root.dataset.workbenchFocused === "true" ||
    root.matches('[data-workbench-focused="true"]')
  );
}

function gapWidth(container) {
  const styles = window.getComputedStyle(container);
  return Number.parseFloat(styles.columnGap || styles.gap || "0") || 0;
}

function availableRegionWidth(root) {
  const container = regionsContainer(root);
  const childCount = visibleRegions(root).length + visibleHandles(root).length;
  const totalGap = Math.max(0, childCount - 1) * gapWidth(container);
  return Math.max(0, container.getBoundingClientRect().width - totalGap);
}

function setRegionWidth(region, pixels) {
  const width = `${Math.max(0, pixels).toFixed(2)}px`;
  region.style.flex = `0 0 ${width}`;
  region.style.width = width;
}

function regionMinWidth(region) {
  return Number(region.dataset.workbenchMinRem || 12) * 16;
}

function clampRegionWidth(region, pixels, availableWidth, reservedMin = 0) {
  const min = regionMinWidth(region);
  const max = Math.max(min, availableWidth - Math.max(0, reservedMin));
  return clamp(pixels, min, max);
}

function regionSlot(region) {
  return region.dataset.workbenchSlot || "";
}

function resizeGroupForHandle(root, before, after) {
  const visible = visibleRegions(root);
  const navigation = [before, after].find(
    (region) => regionSlot(region) === "navigation",
  );
  const content = visible.filter((region) => region !== navigation);
  if (!navigation || content.length === 0) return null;
  return { navigation, content };
}

function canAutoCollapse(region) {
  return regionSlot(region) !== "primary";
}

function clearResizeStyles(region) {
  for (const property of [
    "flex",
    "width",
    "opacity",
    "overflow",
    "transition",
  ]) {
    region.style.removeProperty(property);
  }
}

function collapseRegion(root, region) {
  if (
    !canAutoCollapse(region) ||
    region.dataset.workbenchCollapsing === "true"
  ) {
    return;
  }
  region.dataset.workbenchCollapsing = "true";
  const startWidth = regionWidth(region);
  setRegionWidth(region, startWidth);
  region.style.overflow = "hidden";
  region.style.transition =
    "flex-basis 160ms ease, width 160ms ease, opacity 120ms ease";

  requestAnimationFrame(() => {
    region.style.flex = "0 0 0px";
    region.style.width = "0px";
    region.style.opacity = "0";
  });

  window.setTimeout(async () => {
    await mergeWorkbenchPaths([
      [
        "workbench.regions." + region.dataset.workbenchSignal + ".visible",
        false,
      ],
    ]);
    delete region.dataset.workbenchCollapsing;
    clearResizeStyles(region);
    updateHandles(root);
    requestAnimationFrame(() => applyRegionRatios(root));
    saveConfig(root);
  }, 180);
}

function regionWidth(region) {
  return region.getBoundingClientRect().width;
}


function regionHasSSRFlex(region) {
  const flex = (region.style && region.style.flex) || "";
  // SSR RegionSSRFlexStyle: "0.2200 1 0%" — browsers may normalize 0% → 0px.
  return /^\d/.test(flex.trim()) && /1\s+0(%|px)/.test(flex);
}

function applyRegionRatios(root) {
  updateHandles(root);
  const regions = visibleRegions(root);
  const availableWidth = availableRegionWidth(root);
  if (availableWidth <= 0) return;

  // Keep SSR flex (SavedConfig ratios) until the user drags a grip.
  // Seeding/rewriting pixels on load caused measurable width drift on room switch.
  if (
    root.dataset.workbenchPixelLock !== "1" &&
    regions.length > 0 &&
    regions.every((region) => regionHasSSRFlex(region))
  ) {
    return;
  }

  const primary = regions.find((region) => regionSlot(region) === "primary");
  if (!primary) {
    const total =
      regions.reduce(
        (sum, region) => sum + Number(region.dataset.workbenchRatio || 0),
        0,
      ) || 1;
    for (const region of regions) {
      const ratio = Number(region.dataset.workbenchRatio || 0) / total;
      setRegionWidth(region, ratio * availableWidth);
    }
    return;
  }

  const primaryMin = regionMinWidth(primary);
  const fixedRegions = regions.filter((region) => region !== primary);
  const visibleRatioTotal =
    regions.reduce(
      (sum, region) => sum + Number(region.dataset.workbenchRatio || 0),
      0,
    ) || 1;
  let reservedForOthers = primaryMin;
  let fixedWidth = 0;
  const widths = new Map();
  for (const region of fixedRegions) {
    const othersMin = fixedRegions
      .filter((other) => other !== region)
      .reduce((sum, other) => sum + regionMinWidth(other), 0);
    const storedWidth = Number(region.dataset.workbenchWidthPx || 0);
    const ratioWidth =
      (Number(region.dataset.workbenchRatio || 0) / visibleRatioTotal) *
      availableWidth;
    const width = clampRegionWidth(
      region,
      storedWidth > 0 ? storedWidth : ratioWidth,
      availableWidth,
      reservedForOthers + othersMin,
    );
    widths.set(region, width);
    fixedWidth += width;
  }
  if (fixedWidth > availableWidth - primaryMin) {
    let overflow = fixedWidth - (availableWidth - primaryMin);
    for (const region of [...fixedRegions].reverse()) {
      if (overflow <= 0) break;
      const min = regionMinWidth(region);
      const current = widths.get(region);
      const reducible = Math.max(0, current - min);
      const cut = Math.min(reducible, overflow);
      widths.set(region, current - cut);
      overflow -= cut;
      fixedWidth -= cut;
    }
  }
  for (const region of fixedRegions) {
    const width = widths.get(region);
    region.dataset.workbenchWidthPx = width.toFixed(2);
    setRegionWidth(region, width);
  }
  setRegionWidth(
    primary,
    Math.max(primaryMin, availableWidth - fixedWidth),
  );
}

function updateHandles(root) {
  for (const handle of root.querySelectorAll(
    "[data-workbench-resize-handle]",
  )) {
    const before = regionBySignal(root, handle.dataset.workbenchBefore);
    const after = regionBySignal(root, handle.dataset.workbenchAfter);
    const show = Boolean(
      before && after && isVisible(before) && isVisible(after),
    );
    handle.classList.toggle("md:!block", show);
    handle.classList.toggle("md:!hidden", !show);
  }
}

function currentViewportClass(root) {
  return root.dataset.workbenchViewportClass || "desktop-full";
}

function activeRegionID(root) {
  const activeSignal = root.dataset.workbenchMobileActive || "";
  if (!activeSignal) return "";
  return regionBySignal(root, activeSignal)?.dataset.workbenchRegion || "";
}

function activeRegionSignal(root) {
  return root.dataset.workbenchMobileActive || "";
}

function isWorkbenchV2(root) {
  return root.dataset.workbenchPage === "threads";
}

function visibleRegionSpecs(root) {
  const ratioOnly = isWorkbenchV2(root);
  return allRegions(root).map((region) => ({
    id: region.dataset.workbenchRegion,
    slot: region.dataset.workbenchSlot,
    kind: region.dataset.workbenchKind,
    ratio: Number(region.dataset.workbenchRatio || 0),
    visible: ratioOnly ? undefined : isVisible(region),
  }));
}

function currentConfig(root) {
  const viewportClass = currentViewportClass(root);
  const base = {
    version: 1,
    page: root.dataset.workbenchPage,
    view: root.dataset.workbenchView,
    viewportClass,
    regions: visibleRegionSpecs(root),
  };
  if (isWorkbenchV2(root)) return { ...base, mobile: {} };
  return { ...base, mobile: { activeRegionID: activeRegionID(root) } };
}

function saveConfig(root) {
  if (root?.dataset?.workbenchPixelLock === "1") {
    syncRatiosFromPaint(root);
  }
  fetch("/api/layout-preferences", {
    method: "POST",
    headers: {
      "Content-Type": "application/json",
      Accept: "text/event-stream",
    },
    body: JSON.stringify({
      viewportClass: currentViewportClass(root),
      config: currentConfig(root),
    }),
  }).catch(() => {});
}

function saveConfigForEvent(event) {
  const root = event.target?.closest?.("#workbench-root");
  if (root) saveConfig(root);
}

document.addEventListener("workbench-layout-save", saveConfigForEvent);

// Visible columns only: ratios must sum to ~1 so RegionSSRFlexStyle shares
// match painted widths after sibling GETs (agent/room switch).
function syncRatiosFromPaint(root) {
  const regions = visibleRegions(root);
  if (regions.length === 0) return;
  let total = 0;
  const widths = [];
  for (const region of regions) {
    const w = regionWidth(region);
    widths.push(w);
    total += w;
  }
  const denom = total > 0 ? total : 1;
  regions.forEach((region, i) => {
    const w = widths[i];
    region.dataset.workbenchWidthPx = w.toFixed(2);
    region.dataset.workbenchRatio = (w / denom).toFixed(4);
  });
}

function lockPixelWidthsFromPaint(root) {
  const regions = visibleRegions(root);
  const availableWidth = availableRegionWidth(root);
  if (availableWidth <= 0 || regions.length === 0) return;
  for (const region of regions) {
    setRegionWidth(region, regionWidth(region));
  }
  syncRatiosFromPaint(root);
  root.dataset.workbenchPixelLock = "1";
}

function startResize(event) {
  if (event.button !== 0) return;
  const handle = event.currentTarget;
  const root = handle.closest("#workbench-root");
  if (!root) return;
  const before = regionBySignal(root, handle.dataset.workbenchBefore);
  const after = regionBySignal(root, handle.dataset.workbenchAfter);
  if (!before || !after || !isVisible(before) || !isVisible(after)) return;

  // First grip drag: convert SSR flex to pixel lock from current painted widths.
  if (root.dataset.workbenchPixelLock !== "1") {
    lockPixelWidthsFromPaint(root);
  }

  event.preventDefault();
  const availableWidth = availableRegionWidth(root);
  if (availableWidth <= 0) return;
  handle.setPointerCapture(event.pointerId);
  handle.dataset.resizing = "true";
  document.documentElement.classList.add("workbench-resizing");
  document.body.classList.add("select-none", "cursor-col-resize");

  const beforeStart = regionWidth(before);
  const afterStart = regionWidth(after);
  const pairWidth = beforeStart + afterStart;
  const startX = event.clientX;

  const onMove = (moveEvent) => {
    const dx = moveEvent.clientX - startX;
    const navigationGroup = resizeGroupForHandle(root, before, after);
    if (navigationGroup) {
      const navigationStart =
        navigationGroup.navigation === before ? beforeStart : afterStart;
      const navigationNext =
        navigationGroup.navigation === before
          ? navigationStart + dx
          : navigationStart - dx;
      if (navigationNext <= regionMinWidth(navigationGroup.navigation) / 2) {
        collapseRegion(root, navigationGroup.navigation);
        return;
      }
    }

    const beforeIsPrimary = regionSlot(before) === "primary";
    const afterIsPrimary = regionSlot(after) === "primary";

    if (beforeIsPrimary && !afterIsPrimary) {
      const nextAfter = clampRegionWidth(
        after,
        afterStart - dx,
        availableWidth,
        regionMinWidth(before),
      );
      after.dataset.workbenchRatio = (nextAfter / availableWidth).toFixed(4);
      after.dataset.workbenchWidthPx = nextAfter.toFixed(2);
      applyRegionRatios(root);
      return;
    }

    if (!beforeIsPrimary && afterIsPrimary) {
      const nextBefore = clampRegionWidth(
        before,
        beforeStart + dx,
        availableWidth,
        regionMinWidth(after),
      );
      before.dataset.workbenchRatio = (nextBefore / availableWidth).toFixed(4);
      before.dataset.workbenchWidthPx = nextBefore.toFixed(2);
      applyRegionRatios(root);
      return;
    }

    const beforeMin = regionMinWidth(before);
    const afterMin = regionMinWidth(after);
    const rawBefore = beforeStart + dx;
    const nextBefore = clamp(rawBefore, beforeMin, Math.max(beforeMin, pairWidth - afterMin));
    const nextAfter = pairWidth - nextBefore;
    before.dataset.workbenchRatio = (nextBefore / availableWidth).toFixed(4);
    after.dataset.workbenchRatio = (nextAfter / availableWidth).toFixed(4);
    before.dataset.workbenchWidthPx = nextBefore.toFixed(2);
    after.dataset.workbenchWidthPx = nextAfter.toFixed(2);
    setRegionWidth(before, nextBefore);
    setRegionWidth(after, nextAfter);
  };

  const onUp = () => {
    handle.removeEventListener("pointermove", onMove);
    handle.removeEventListener("pointerup", onUp);
    handle.removeEventListener("pointercancel", onUp);
    delete handle.dataset.resizing;
    document.documentElement.classList.remove("workbench-resizing");
    document.body.classList.remove("select-none", "cursor-col-resize");
    // Drag paths only rewrite the gripped pair's ratio attrs — resync all
    // visible columns from paint so layoutprefs survive the next room GET.
    syncRatiosFromPaint(root);
    saveConfig(root);
  };

  handle.addEventListener("pointermove", onMove);
  handle.addEventListener("pointerup", onUp);
  handle.addEventListener("pointercancel", onUp);
}

function bindResizeHandles(root) {
  for (const handle of root.querySelectorAll(
    "[data-workbench-resize-handle]",
  )) {
    // Datastar morphs can replace handle nodes while keeping #workbench-root.
    // Rebind any unbound handle; WeakSet-on-root alone left detached listeners.
    if (handle.dataset.workbenchResizeBound === "1") continue;
    handle.dataset.workbenchResizeBound = "1";
    handle.addEventListener("pointerdown", startResize);
  }
}

function initWorkbench(root) {
  // Don't fight an in-progress grip drag (MutationObserver style/class churn).
  if (!document.documentElement.classList.contains("workbench-resizing")) {
    applyRegionRatios(root);
  }
  bindResizeHandles(root);
  roots.add(root);
}

function init() {
  for (const root of document.querySelectorAll("#workbench-root")) {
    initWorkbench(root);
  }
}

let windowResizeFrame;
function reflowWorkbenchesAfterWindowResize() {
  if (windowResizeFrame) cancelAnimationFrame(windowResizeFrame);
  windowResizeFrame = requestAnimationFrame(() => {
    windowResizeFrame = undefined;
    for (const root of document.querySelectorAll("#workbench-root")) {
      if (root.dataset.workbenchPixelLock !== "1") {
        // SSR flex scales with container — do not snap to pixels on window resize.
        updateHandles(root);
        continue;
      }
      for (const region of allRegions(root)) {
        delete region.dataset.workbenchWidthPx;
      }
      applyRegionRatios(root);
    }
  });
}

window.addEventListener("resize", reflowWorkbenchesAfterWindowResize, {
  passive: true,
});

init();
document.addEventListener("datastar-patch-elements", init);
new MutationObserver(init).observe(document.documentElement, {
  childList: true,
  subtree: true,
  attributes: true,
  attributeFilter: ["class", "style", "data-workbench-focused"],
});
