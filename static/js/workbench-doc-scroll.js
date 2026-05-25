const initializedRoots = new WeakSet();

function closestScrollRoot(trigger) {
	return trigger.closest('#workbench-root') || document;
}

function nearestSharedSidebar(trigger) {
	return trigger.closest('aside[id]') || closestScrollRoot(trigger);
}

function visibleEnough(element) {
	if (!element) return false;
	const rect = element.getBoundingClientRect();
	return rect.width > 0 && rect.height > 0;
}

function scrollNearestVertical(container, target) {
	if (
		!container ||
		!target ||
		!visibleEnough(container) ||
		!visibleEnough(target)
	)
		return;

	const containerRect = container.getBoundingClientRect();
	const targetRect = target.getBoundingClientRect();
	const before = targetRect.top - containerRect.top;
	const after = targetRect.bottom - containerRect.bottom;

	if (before < 0) {
		container.scrollBy({ top: before, behavior: 'smooth' });
		return;
	}
	if (after > 0) {
		container.scrollBy({ top: after, behavior: 'smooth' });
	}
}

export function scrollCurrentDocInContainer(root, containerName) {
	if (!root || !containerName) return;
	const container = root.querySelector(
		`[data-doc-scroll-container="${CSS.escape(containerName)}"]`,
	);
	const target = container?.querySelector(
		'[data-current-doc-tree-item="true"]',
	);
	scrollNearestVertical(container, target);
}

export function scheduleCurrentDocRevealScroll(trigger) {
	if (!trigger) return;
	const containerName = trigger.dataset.docScrollOnReveal;
	const root = nearestSharedSidebar(trigger);
	scheduleAfterWorkbenchLayout(() =>
		scrollCurrentDocInContainer(root, containerName),
	);
}

function handleReveal(event) {
	const trigger = event.target?.closest?.('[data-doc-scroll-on-reveal]');
	if (!trigger) return;
	scheduleCurrentDocRevealScroll(trigger);
}

function cleanSectionHash(hashOrID) {
	const raw = String(hashOrID || '').trim();
	if (!raw) return '';
	const withoutHash = raw.startsWith('#') ? raw.slice(1) : raw;
	try {
		return decodeURIComponent(withoutHash);
	} catch (_) {
		return withoutHash;
	}
}

function sectionTargetFromLink(link) {
	if (!link) return '';
	const explicit = link.dataset.workbenchSectionTarget;
	if (explicit) return explicit;
	const href = link.getAttribute('href') || '';
	return cleanSectionHash(href);
}

function workbenchRootFor(element) {
	return (
		element?.closest?.('#workbench-root') ||
		document.getElementById('workbench-root') ||
		document
	);
}

function findSectionTarget(id, root = document) {
	if (!id) return null;
	return root.getElementById?.(id) || document.getElementById(id);
}

function findSectionContainer(id, root = document) {
	const containerID = id || 'thoughts-markdown-scroll-region';
	return (
		root.getElementById?.(containerID) || document.getElementById(containerID)
	);
}

function activeRegionSignalKey(root, regionKey) {
	const key =
		regionKey || root?.dataset?.workbenchMobileActive || 'docWorkbenchCenter';
	return key || 'docWorkbenchCenter';
}

function workbenchRegionForSignal(root, key) {
	return root.querySelector(`[data-workbench-signal="${CSS.escape(key)}"]`);
}

function activateWorkbenchRegion(root, regionKey = 'docWorkbenchCenter') {
	if (!root) return;
	const key = activeRegionSignalKey(root, regionKey);
	const activeRegion = workbenchRegionForSignal(root, key);
	if (!activeRegion) return;
	activeRegion.classList.remove('max-md:!hidden');
	activeRegion.classList.add('max-md:!flex');
	for (const region of root.querySelectorAll('[data-workbench-signal]')) {
		if (region === activeRegion) continue;
		region.classList.remove('max-md:!flex');
		region.classList.add('max-md:!hidden');
	}
}

function scheduleAfterWorkbenchLayout(callback) {
	requestAnimationFrame(() => {
		requestAnimationFrame(callback);
	});
}

function scrollTargetInsideContainer(container, target, options = {}) {
	if (
		!container ||
		!target ||
		!visibleEnough(container) ||
		!visibleEnough(target)
	) {
		return false;
	}
	const behavior = options.behavior || 'smooth';
	const containerRect = container.getBoundingClientRect();
	const targetRect = target.getBoundingClientRect();
	const top = targetRect.top - containerRect.top + container.scrollTop;
	container.scrollTo({ top, behavior });
	return true;
}

function updateSectionURL(id, updateURL) {
	if (!updateURL || !id) return;
	const nextHash = `#${encodeURIComponent(id)}`;
	if (window.location.hash !== nextHash) {
		history.pushState(null, '', nextHash);
	}
}

export function navigateWorkbenchSection(hashOrID, options = {}) {
	const id = cleanSectionHash(hashOrID);
	if (!id) return false;
	const root = workbenchRootFor(options.trigger || document.body);
	const regionKey = options.regionKey || 'docWorkbenchCenter';
	const containerID = options.containerID || 'thoughts-markdown-scroll-region';
	const container = findSectionContainer(containerID, root);
	const target = findSectionTarget(id, root);
	if (!container || !target) return false;
	if (options.activateRegion !== false) {
		activateWorkbenchRegion(root, regionKey);
	}
	updateSectionURL(id, options.updateURL !== false);
	scheduleAfterWorkbenchLayout(() =>
		scrollTargetInsideContainer(container, target, options),
	);
	return true;
}

export function navigateCurrentWorkbenchHash(options = {}) {
	return navigateWorkbenchSection(window.location.hash, {
		updateURL: false,
		...options,
	});
}

function handleWorkbenchSectionClick(event) {
	const link = event.target?.closest?.('[data-workbench-section-link]');
	if (!link) return;
	const id = sectionTargetFromLink(link);
	if (!id) return;
	const handled = navigateWorkbenchSection(id, {
		trigger: link,
		updateURL: true,
		regionKey: link.dataset.workbenchSectionRegion || 'docWorkbenchCenter',
		containerID:
			link.dataset.workbenchSectionContainer ||
			'thoughts-markdown-scroll-region',
	});
	if (handled) {
		event.preventDefault();
	}
}

function sectionTargetFromEventDetail(detail = {}) {
	if (Object.hasOwn(detail, 'hash')) return detail.hash;
	if (Object.hasOwn(detail, 'id')) return detail.id;
	return window.location.hash;
}

function handleWorkbenchSectionNav(event) {
	const detail = event.detail || {};
	navigateWorkbenchSection(sectionTargetFromEventDetail(detail), {
		updateURL: false,
		activateRegion: detail.activateRegion !== false,
		regionKey: detail.regionKey || 'docWorkbenchCenter',
		containerID: detail.containerID || 'thoughts-markdown-scroll-region',
	});
}

function init(root = document) {
	if (initializedRoots.has(root)) return;
	initializedRoots.add(root);
	root.addEventListener('doc-scroll-reveal', handleReveal);
	root.addEventListener('workbench-section-nav', handleWorkbenchSectionNav);
	root.addEventListener('click', handleWorkbenchSectionClick, true);
	root.addEventListener('click', (event) => {
		const trigger = event.target?.closest?.('[data-doc-scroll-on-reveal]');
		if (!trigger) return;
		scheduleCurrentDocRevealScroll(trigger);
	});
}

window.addEventListener('hashchange', () =>
	navigateCurrentWorkbenchHash({ updateURL: false }),
);
init();
scheduleAfterWorkbenchLayout(() =>
	navigateCurrentWorkbenchHash({ updateURL: false }),
);
