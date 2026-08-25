(() => {
  const bridgeMode = document.currentScript?.dataset.commentuiMode || "parent";
  const protocolVersion = 1;
  const handshakeType = "vamos:frame-comment-handshake";
  const selectionType = "vamos:frame-quote-selection";
  const selectionClearType = "vamos:frame-quote-selection-clear";
  const maxQuoteLength = 8192;
  const maxCoordinate = 10_000_000;

  if (bridgeMode === "child") {
    installChildBridge();
  } else {
    installParentBridge();
  }

  function installChildBridge() {
    if (window.parent === window) {
      return;
    }

    let bridgeID = "";
    window.addEventListener("message", (event) => {
      const message = event.data;
      if (
        event.source !== window.parent ||
        !message ||
        message.version !== protocolVersion ||
        typeof message.bridge_id !== "string" ||
        message.bridge_id.length > 256
      ) {
        return;
      }
      if (message.type === handshakeType) {
        bridgeID = message.bridge_id;
        return;
      }
      if (
        message.type === selectionClearType &&
        bridgeID &&
        message.bridge_id === bridgeID
      ) {
        window.getSelection()?.removeAllRanges();
      }
    });

    const publishSelection = scheduledSelectionReader(() => {
      if (!bridgeID) {
        return;
      }
      const selection = window.getSelection();
      const quote = selection ? selection.toString().trim() : "";
      if (
        !quote ||
        quote.length > maxQuoteLength ||
        !selection ||
        selection.rangeCount === 0
      ) {
        postChildSelection(bridgeID, "", null);
        return;
      }
      postChildSelection(
        bridgeID,
        quote,
        rectanglePayload(selection.getRangeAt(0).getBoundingClientRect()),
      );
    });
    for (const eventName of ["selectionchange", "mouseup", "touchend"]) {
      document.addEventListener(eventName, publishSelection, true);
    }
  }

  function scheduledSelectionReader(readSelection) {
    let scheduled = false;
    return () => {
      if (scheduled) {
        return;
      }
      scheduled = true;
      window.requestAnimationFrame(() => {
        scheduled = false;
        readSelection();
      });
    };
  }

  function postChildSelection(bridgeID, quote, rect) {
    window.parent.postMessage(
      {
        type: selectionType,
        version: protocolVersion,
        bridge_id: bridgeID,
        quote,
        rect,
      },
      "*",
    );
  }

  function installParentBridge() {
    const boundDocuments = new WeakMap();
    const observedFrames = new WeakSet();

    const bindFrame = (frame) => {
      if (!(frame instanceof HTMLIFrameElement)) {
        return;
      }
      const bridgeID = frame.dataset.commentuiBridgeId;
      const transport = frame.dataset.commentuiTransport;
      if (!bridgeID || !transport) {
        return;
      }

      if (!observedFrames.has(frame)) {
        observedFrames.add(frame);
        frame.addEventListener("load", () => bindFrame(frame));
      }

      if (transport === "opaque-postmessage") {
        frame.contentWindow?.postMessage(
          {
            type: handshakeType,
            version: protocolVersion,
            bridge_id: bridgeID,
          },
          "*",
        );
        return;
      }
      if (transport !== "same-origin-dom") {
        return;
      }

      try {
        const childDocument = frame.contentDocument;
        if (!childDocument || boundDocuments.get(frame) === childDocument) {
          return;
        }
        boundDocuments.set(frame, childDocument);
        const publishSelection = scheduledSelectionReader(() =>
          applyDirectSelection(frame),
        );
        for (const eventName of ["selectionchange", "mouseup", "touchend"]) {
          childDocument.addEventListener(eventName, publishSelection, true);
        }
      } catch {
        clearSelection(frame);
      }
    };

    const discoverFrames = (root) => {
      if (
        root instanceof HTMLIFrameElement &&
        root.matches("[data-commentui-frame]")
      ) {
        bindFrame(root);
      }
      if (root instanceof Document || root instanceof Element) {
        root
          .querySelectorAll("iframe[data-commentui-frame]")
          .forEach(bindFrame);
      }
    };

    window.addEventListener("message", (event) => {
      const frame = Array.from(
        document.querySelectorAll("iframe[data-commentui-frame]"),
      ).find((candidate) => candidate.contentWindow === event.source);
      if (!frame || frame.dataset.commentuiTransport !== "opaque-postmessage") {
        return;
      }
      const message = event.data;
      if (
        !message ||
        message.type !== selectionType ||
        message.version !== protocolVersion ||
        message.bridge_id !== frame.dataset.commentuiBridgeId
      ) {
        return;
      }
      if (message.quote === "" && message.rect === null) {
        clearSelection(frame);
        return;
      }
      if (!validQuote(message.quote) || !validRectangle(message.rect)) {
        return;
      }
      applySelection(frame, message.quote, message.rect);
    });

    document.addEventListener(
      "submit",
      (event) => {
        const form = event.target;
        if (
          !(form instanceof HTMLFormElement) ||
          !form.matches("form.commentui-selection-trigger")
        ) {
          return;
        }
        window.getSelection()?.removeAllRanges();
        const container = form.closest("[data-commentui-frame-bridge]");
        const frame = container?.querySelector("iframe[data-commentui-frame]");
        if (frame instanceof HTMLIFrameElement) {
          clearChildSelection(frame);
        }
      },
      true,
    );

    discoverFrames(document);
    new MutationObserver((records) => {
      for (const record of records) {
        record.addedNodes.forEach(discoverFrames);
      }
    }).observe(document.documentElement, { childList: true, subtree: true });
  }

  function clearChildSelection(frame) {
    if (frame.dataset.commentuiTransport === "same-origin-dom") {
      try {
        frame.contentWindow?.getSelection()?.removeAllRanges();
        return;
      } catch {
        return;
      }
    }
    if (frame.dataset.commentuiTransport === "opaque-postmessage") {
      frame.contentWindow?.postMessage(
        {
          type: selectionClearType,
          version: protocolVersion,
          bridge_id: frame.dataset.commentuiBridgeId,
        },
        "*",
      );
    }
  }

  function applyDirectSelection(frame) {
    try {
      const selection = frame.contentWindow?.getSelection();
      const quote = selection ? selection.toString().trim() : "";
      if (!quote || !selection || selection.rangeCount === 0) {
        clearSelection(frame);
        return;
      }
      if (quote.length > maxQuoteLength) {
        clearSelection(frame);
        return;
      }
      const rect = rectanglePayload(
        selection.getRangeAt(0).getBoundingClientRect(),
      );
      if (!validRectangle(rect)) {
        clearSelection(frame);
        return;
      }
      applySelection(frame, quote, rect);
    } catch {
      clearSelection(frame);
    }
  }

  function applySelection(frame, quote, rect) {
    const container = frame.closest("[data-commentui-frame-bridge]");
    if (
      !container ||
      container.dataset.commentuiFrameBridge !== frame.dataset.commentuiBridgeId
    ) {
      return;
    }
    const frameRect = frame.getBoundingClientRect();
    const containerRect = container.getBoundingClientRect();
    const top =
      frameRect.top + rect.top - containerRect.top + container.scrollTop;
    const left =
      frameRect.left + rect.left - containerRect.left + container.scrollLeft;
    if (!Number.isFinite(top) || !Number.isFinite(left)) {
      return;
    }

    setSelectionField(container, "top", boundedCoordinate(top));
    setSelectionField(
      container,
      "bottom",
      boundedCoordinate(top + rect.height),
    );
    setSelectionField(container, "left", boundedCoordinate(left));
    setSelectionField(container, "section-id", "document");
    setSelectionField(
      container,
      "heading-hint",
      container.dataset.commentuiHeadingHint || "Document",
    );
    setSelectionField(container, "text", quote);
  }

  function clearSelection(frame) {
    const container = frame.closest("[data-commentui-frame-bridge]");
    if (!container) {
      return;
    }
    setSelectionField(container, "text", "");
    setSelectionField(container, "section-id", "");
    setSelectionField(container, "heading-hint", "");
  }

  function setSelectionField(container, field, value) {
    const input = container.querySelector(
      `[data-commentui-selection-field="${field}"]`,
    );
    if (!input) {
      return;
    }
    input.value = String(value);
    input.dispatchEvent(new Event("input", { bubbles: true }));
  }

  function rectanglePayload(rect) {
    return {
      top: rect.top,
      left: rect.left,
      width: rect.width,
      height: rect.height,
    };
  }

  function validQuote(quote) {
    return (
      typeof quote === "string" &&
      quote.trim() !== "" &&
      quote.length <= maxQuoteLength
    );
  }

  function validRectangle(rect) {
    if (!rect || typeof rect !== "object") {
      return false;
    }
    return (
      [rect.top, rect.left, rect.width, rect.height].every(
        (value) =>
          typeof value === "number" &&
          Number.isFinite(value) &&
          Math.abs(value) <= maxCoordinate,
      ) &&
      rect.width >= 0 &&
      rect.height >= 0
    );
  }

  function boundedCoordinate(value) {
    return Math.max(-maxCoordinate, Math.min(maxCoordinate, value));
  }
})();
