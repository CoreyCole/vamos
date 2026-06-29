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

function applyRegionRatios(root) {
  updateHandles(root);
  const regions = visibleRegions(root);
  const availableWidth = availableRegionWidth(root);
  if (availableWidth <= 0) return;

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

  let fixedWidth = 0;
  for (const region of regions) {
    if (region === primary) continue;
    const storedWidth = Number(region.dataset.workbenchWidthPx || 0);
    const ratioWidth =
      Number(region.dataset.workbenchRatio || 0) * availableWidth;
    const width = clamp(
      storedWidth > 0 ? storedWidth : ratioWidth,
      0,
      availableWidth,
    );
    region.dataset.workbenchWidthPx = width.toFixed(2);
    fixedWidth += width;
    setRegionWidth(region, width);
  }
  setRegionWidth(primary, Math.max(0, availableWidth - fixedWidth));
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

function visibleRegionSpecs(root) {
  return allRegions(root).map((region) => ({
    id: region.dataset.workbenchRegion,
    slot: region.dataset.workbenchSlot,
    kind: region.dataset.workbenchKind,
    ratio: Number(region.dataset.workbenchRatio || 0),
    visible: isVisible(region),
  }));
}

function currentConfig(root) {
  const viewportClass = currentViewportClass(root);
  return {
    version: 1,
    page: root.dataset.workbenchPage,
    view: root.dataset.workbenchView,
    viewportClass,
    regions: visibleRegionSpecs(root),
    mobile: { activeRegionID: activeRegionID(root) },
  };
}

function saveConfig(root) {
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
      const nextAfter = clamp(afterStart - dx, 0, availableWidth);
      after.dataset.workbenchRatio = (nextAfter / availableWidth).toFixed(4);
      after.dataset.workbenchWidthPx = nextAfter.toFixed(2);
      applyRegionRatios(root);
      return;
    }

    if (!beforeIsPrimary && afterIsPrimary) {
      const nextBefore = clamp(beforeStart + dx, 0, availableWidth);
      before.dataset.workbenchRatio = (nextBefore / availableWidth).toFixed(4);
      before.dataset.workbenchWidthPx = nextBefore.toFixed(2);
      applyRegionRatios(root);
      return;
    }

    const rawBefore = beforeStart + dx;
    const nextBefore = clamp(rawBefore, 0, pairWidth);
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
