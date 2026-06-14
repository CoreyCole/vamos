export function install(datastar) {
  installClipboard(datastar);
  installReplaceURL(datastar);
}

function installClipboard(datastar) {
  if (hasActionAPI(datastar)) {
    datastar.action({
      name: "clipboard",
      apply: ({ error }, text, base64 = false) => {
        const value = base64 ? atob(String(text ?? "")) : String(text ?? "");
        if (!navigator.clipboard?.writeText) {
          throw (
            error?.("ClipboardNotAvailable") ??
            new Error("ClipboardNotAvailable")
          );
        }
        return navigator.clipboard.writeText(value);
      },
    });
    return;
  }

  installClipboardDOMFallback();
}

function installReplaceURL(datastar) {
  if (hasAttributeAPI(datastar)) {
    datastar.attribute({
      name: "replace-url",
      requirement: { key: "denied", value: "must" },
      returnsValue: true,
      apply({ rx }) {
        return datastar.effect(() => {
          replaceCurrentURL(rx());
        });
      },
    });
    return;
  }

  installReplaceURLDOMFallback();
}

function hasActionAPI(datastar) {
  return datastar && typeof datastar.action === "function";
}

function hasAttributeAPI(datastar) {
  return (
    datastar &&
    typeof datastar.attribute === "function" &&
    typeof datastar.effect === "function"
  );
}

export function resolveSameOriginURL(raw, base = window.location.href) {
  const value = unwrapDatastarString(raw);
  if (!value) return null;
  try {
    const url = new URL(value, base);
    if (url.origin !== window.location.origin) return null;
    return url;
  } catch {
    return null;
  }
}

function replaceCurrentURL(raw) {
  const url = resolveSameOriginURL(raw);
  if (!url) return;
  window.history.replaceState({}, "", url.toString());
}

function unwrapDatastarString(raw) {
  const value = String(raw ?? "").trim();
  if (
    (value.startsWith("'") && value.endsWith("'")) ||
    (value.startsWith('"') && value.endsWith('"'))
  ) {
    return value.slice(1, -1);
  }
  return value;
}

function installReplaceURLDOMFallback() {
  const apply = (root = document) => {
    root.querySelectorAll?.("[data-replace-url]").forEach((element) => {
      replaceCurrentURL(element.getAttribute("data-replace-url"));
    });
  };

  if (document.readyState === "loading") {
    document.addEventListener("DOMContentLoaded", () => apply(), {
      once: true,
    });
  } else {
    apply();
  }

  new MutationObserver((mutations) => {
    for (const mutation of mutations) {
      if (
        mutation.type === "attributes" &&
        mutation.target instanceof Element
      ) {
        applyElementReplaceURL(mutation.target);
      }
      for (const node of mutation.addedNodes) {
        if (node instanceof Element) {
          applyElementReplaceURL(node);
          apply(node);
        }
      }
    }
  }).observe(document.documentElement, {
    childList: true,
    subtree: true,
    attributes: true,
    attributeFilter: ["data-replace-url"],
  });
}

function applyElementReplaceURL(element) {
  if (element.hasAttribute?.("data-replace-url")) {
    replaceCurrentURL(element.getAttribute("data-replace-url"));
  }
}

function installClipboardDOMFallback() {
  document.addEventListener(
    "click",
    (event) => {
      const button = event.target?.closest?.('[data-on\\:click*="@clipboard"]');
      if (!button || !navigator.clipboard?.writeText) return;
      const expression = button.getAttribute("data-on:click") || "";
      const hash = expression.match(/#msg-[A-Za-z0-9_-]+/)?.[0] || "";
      if (!hash) return;
      event.preventDefault();
      event.stopImmediatePropagation();
      navigator.clipboard.writeText(window.location.href.split("#")[0] + hash);
    },
    true,
  );
}
