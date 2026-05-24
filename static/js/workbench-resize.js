const datastarModule = import("/js/datastar-pro-v1.js").catch((error) => {
  console.warn(
    "Datastar Pro asset unavailable; falling back to public Datastar bundle",
    error,
  );
  return import("https://cdn.jsdelivr.net/gh/starfederation/datastar@v1.0.1/bundles/datastar.js");
});

async function mergeWorkbenchPaths(patches) {
  const { mergePaths } = await datastarModule;
  mergePaths(patches);
}

const roots = new WeakSet();
const pendingSaves = new WeakMap();
let navigationSaveInstalled = false;

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

  window.setTimeout(() => {
    void mergeWorkbenchPaths([
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

function applyRegionRatios(root) {
  updateHandles(root);
  const regions = visibleRegions(root);
  const availableWidth = availableRegionWidth(root);
  if (availableWidth <= 0) return;
  const total =
    regions.reduce(
      (sum, region) => sum + Number(region.dataset.workbenchRatio || 0),
      0,
    ) || 1;
  for (const region of regions) {
    const ratio = Number(region.dataset.workbenchRatio || 0) / total;
    setRegionWidth(region, ratio * availableWidth);
  }
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

function regionVisibleFromRoot(root, region) {
  const attr = region.getAttribute("data-workbench-visible");
  if (attr === "true") return true;
  if (attr === "false") return false;
  return isVisible(region);
}

function currentSidebarTab(root) {
  const sidebar = root.querySelector(
    "#workbench-shared-sidebar, #thoughts-shared-sidebar",
  );
  return (
    sidebar?.dataset.workbenchSidebarTab ||
    root.dataset.workbenchSidebarTab ||
    ""
  );
}

function currentRightRailTab(root) {
  const rail = root.querySelector("#doc-right-rail");
  return (
    rail?.dataset.workbenchRightRailTab ||
    root.dataset.workbenchRightRailTab ||
    ""
  );
}

function currentConfig(root) {
  const regions = allRegions(root).map((region) => ({
    id: region.dataset.workbenchRegion,
    slot: region.dataset.workbenchSlot,
    kind: region.dataset.workbenchKind,
    ratio: Number(region.dataset.workbenchRatio || 0),
    visible: regionVisibleFromRoot(root, region),
  }));

  return {
    version: 1,
    page: root.dataset.workbenchPage,
    view: root.dataset.workbenchView,
    regions,
    mobile: { activeRegionID: root.dataset.workbenchMobileActive || "" },
    tabs: {
      sidebarTab: currentSidebarTab(root),
      rightRailTab: currentRightRailTab(root),
    },
  };
}

function saveConfig(root) {
  fetch("/api/layout-preferences", {
    method: "POST",
    keepalive: true,
    headers: {
      "Content-Type": "application/json",
      Accept: "text/event-stream",
    },
    body: JSON.stringify({ config: currentConfig(root) }),
  }).catch(() => {});
}

function scheduleSave(root) {
  window.clearTimeout(pendingSaves.get(root));
  pendingSaves.set(
    root,
    window.setTimeout(() => saveConfig(root), 150),
  );
}

function saveBeforeNavigation(event) {
  const link = event.target.closest?.("a[href]");
  if (
    !link ||
    link.target ||
    event.metaKey ||
    event.ctrlKey ||
    event.shiftKey ||
    event.altKey
  ) {
    return;
  }
  const root =
    link.closest("#workbench-root") ||
    document.querySelector("#workbench-root");
  if (!root) return;
  saveConfig(root);
}

function startResize(event) {
  if (event.button !== 0) return;
  const handle = event.currentTarget;
  const root = handle.closest("#workbench-root");
  if (!root) return;
  const before = regionBySignal(root, handle.dataset.workbenchBefore);
  const after = regionBySignal(root, handle.dataset.workbenchAfter);
  if (!before || !after || !isVisible(before) || !isVisible(after)) return;

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
  const beforeMin = regionMinWidth(before);
  const afterMin = regionMinWidth(after);
  const navigationGroup = resizeGroupForHandle(root, before, after);
  const navigationStart = navigationGroup
    ? regionWidth(navigationGroup.navigation)
    : 0;
  const contentStarts = navigationGroup
    ? navigationGroup.content.map(regionWidth)
    : [];
  const contentStartTotal = contentStarts.reduce(
    (sum, width) => sum + width,
    0,
  );
  const contentShares = contentStarts.map((width) =>
    contentStartTotal > 0
      ? width / contentStartTotal
      : 1 / contentStarts.length,
  );
  const contentMin = navigationGroup
    ? navigationGroup.content.reduce(
        (sum, region) => sum + regionMinWidth(region),
        0,
      )
    : 0;
  const startX = event.clientX;

  const onMove = (moveEvent) => {
    const dx = moveEvent.clientX - startX;
    if (navigationGroup) {
      const direction = navigationGroup.navigation === before ? 1 : -1;
      const rawNavigation = navigationStart + dx * direction;
      const minNavigation = regionMinWidth(navigationGroup.navigation);
      if (rawNavigation < minNavigation) {
        collapseRegion(root, navigationGroup.navigation);
        onUp();
        return;
      }
      const maxNavigation = Math.max(
        minNavigation,
        availableWidth - contentMin,
      );
      const nextNavigation = clamp(rawNavigation, minNavigation, maxNavigation);
      const nextContentTotal = availableWidth - nextNavigation;

      navigationGroup.navigation.dataset.workbenchRatio = (
        nextNavigation / availableWidth
      ).toFixed(4);
      setRegionWidth(navigationGroup.navigation, nextNavigation);
      navigationGroup.content.forEach((region, index) => {
        const width = nextContentTotal * contentShares[index];
        region.dataset.workbenchRatio = (width / availableWidth).toFixed(4);
        setRegionWidth(region, width);
      });
      return;
    }

    const rawBefore = beforeStart + dx;
    const rawAfter = pairWidth - rawBefore;
    if (rawBefore < beforeMin && canAutoCollapse(before)) {
      collapseRegion(root, before);
      onUp();
      return;
    }
    if (rawAfter < afterMin && canAutoCollapse(after)) {
      collapseRegion(root, after);
      onUp();
      return;
    }
    const minBefore = beforeMin;
    const maxBefore = Math.max(minBefore, pairWidth - afterMin);
    const nextBefore = clamp(rawBefore, minBefore, maxBefore);
    const nextAfter = pairWidth - nextBefore;
    const beforeRatio = nextBefore / availableWidth;
    const afterRatio = nextAfter / availableWidth;

    before.dataset.workbenchRatio = beforeRatio.toFixed(4);
    after.dataset.workbenchRatio = afterRatio.toFixed(4);
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
    saveConfig(root);
  };

  handle.addEventListener("pointermove", onMove);
  handle.addEventListener("pointerup", onUp);
  handle.addEventListener("pointercancel", onUp);
}

function initWorkbench(root) {
  applyRegionRatios(root);
  if (roots.has(root)) return;
  roots.add(root);
  for (const handle of root.querySelectorAll(
    "[data-workbench-resize-handle]",
  )) {
    handle.addEventListener("pointerdown", startResize);
  }
  root.addEventListener("workbench-state-changed", () => scheduleSave(root));
  root.addEventListener("click", (event) => {
    if (!event.target.closest?.("[data-workbench-save-on-click]")) return;
    requestAnimationFrame(() => scheduleSave(root));
  });
  if (!navigationSaveInstalled) {
    document.addEventListener("click", saveBeforeNavigation, { capture: true });
    navigationSaveInstalled = true;
  }
}

function init() {
  for (const root of document.querySelectorAll("#workbench-root")) {
    initWorkbench(root);
  }
}

init();
document.addEventListener("datastar-patch-elements", init);
new MutationObserver(init).observe(document.documentElement, {
  childList: true,
  subtree: true,
  attributes: true,
  attributeFilter: ["class", "style", "data-workbench-focused"],
});
