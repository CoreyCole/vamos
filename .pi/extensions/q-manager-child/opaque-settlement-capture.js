import {
  Alias,
  Document,
  Pair,
  Scalar,
  YAMLMap,
  YAMLSeq,
  parseDocument,
} from "yaml";

/** Projects only persisted text, without rendering or normalization. */
export function projectPersistedAssistantText(content) {
  return content
    .filter((part) => part?.type === "text" && typeof part.text === "string")
    .map((part) => part.text)
    .join("");
}

function lineEnd(raw, start) {
  const newline = raw.indexOf("\n", start);
  return newline < 0 ? raw.length : newline + 1;
}

/** Returns bodies from exactly lexically-qualified yaml/yml fences. */
export function captureOpaqueYamlFences(raw) {
  const blocks = [];
  let start = 0;
  while (start < raw.length) {
    const end = lineEnd(raw, start);
    const opener = raw
      .slice(start, end)
      .match(/^(?<ticks>`{3,})(?<lang>yaml|yml)[ \t\r]*(?:\n|$)/i);
    if (!opener) {
      start = end;
      continue;
    }
    let cursor = end;
    while (cursor < raw.length) {
      const closeEnd = lineEnd(raw, cursor);
      if (
        new RegExp(`^${opener.groups.ticks}[ \\t\\r]*(?:\\n|$)`).test(
          raw.slice(cursor, closeEnd),
        )
      ) {
        blocks.push(raw.slice(end, cursor));
        start = closeEnd;
        cursor = -1;
        break;
      }
      cursor = closeEnd;
    }
    if (cursor !== -1) start = end;
  }
  return blocks;
}

function scalarString(node) {
  return node instanceof Scalar && typeof node.value === "string"
    ? node.value
    : undefined;
}

function rejected(node, seen = new Set()) {
  if (!node || seen.has(node)) return false;
  seen.add(node);
  if (node instanceof Alias) return true;
  if (node instanceof YAMLMap) {
    const keys = new Set();
    for (const pair of node.items) {
      if (!(pair instanceof Pair)) return true;
      const key = scalarString(pair.key);
      if (
        key === undefined ||
        key === "<<" ||
        keys.has(key) ||
        RESERVED.has(key)
      )
        return true;
      keys.add(key);
      if (rejected(pair.key, seen) || rejected(pair.value, seen)) return true;
    }
  } else if (node instanceof YAMLSeq) {
    for (const item of node.items) if (rejected(item, seen)) return true;
  }
  return false;
}

function codePointCompare(left, right) {
  const a = Array.from(left);
  const b = Array.from(right);
  for (let index = 0; index < Math.min(a.length, b.length); index++) {
    const difference = a[index].codePointAt(0) - b[index].codePointAt(0);
    if (difference) return difference;
  }
  return a.length - b.length;
}

function cloneAndSort(node) {
  if (node instanceof YAMLMap) {
    const map = new YAMLMap();
    map.items = [...node.items]
      .sort((a, b) =>
        codePointCompare(scalarString(a.key), scalarString(b.key)),
      )
      .map(
        (pair) => new Pair(cloneAndSort(pair.key), cloneAndSort(pair.value)),
      );
    return map;
  }
  if (node instanceof YAMLSeq) {
    const seq = new YAMLSeq();
    seq.items = node.items.map(cloneAndSort);
    return seq;
  }
  if (node instanceof Scalar) return new Scalar(node.value);
  throw new Error("unexpected accepted YAML node");
}

const RESERVED = new Set([
  "version",
  "manager_thread_id",
  "pi_session_id",
  "message_id",
  "raw_response",
]);

function literal(value) {
  const scalar = new Scalar(value);
  scalar.type = "BLOCK_LITERAL";
  // yaml's default is strip for no LF and clip for one LF. Keep is explicit.
  scalar.chompKeep = /\n\n$/.test(value);
  return scalar;
}

/**
 * Builds immutable evidence. A child mapping is copied only when it is one
 * unambiguous YAML document; otherwise the record is deliberately system-only.
 */
export function buildSettlementEvidence(identity, rawResponse) {
  const fences = captureOpaqueYamlFences(rawResponse);
  let child;
  if (fences.length === 1) {
    const body = fences[0];
    const hasDocumentSyntax = /^(?:%|---[ \t]*$|\.\.\.[ \t]*$)/m.test(body);
    const document = hasDocumentSyntax
      ? undefined
      : parseDocument(body, {
          uniqueKeys: true,
          merge: false,
          prettyErrors: false,
        });
    if (
      document &&
      document.errors.length === 0 &&
      document.warnings.length === 0 &&
      document.contents instanceof YAMLMap &&
      !rejected(document.contents)
    )
      child = cloneAndSort(document.contents);
  }

  const document = new Document();
  const map = child ?? new YAMLMap();
  map.items.push(
    new Pair(new Scalar("version"), new Scalar(1)),
    new Pair(
      new Scalar("manager_thread_id"),
      new Scalar(identity.managerThreadID),
    ),
    new Pair(new Scalar("pi_session_id"), new Scalar(identity.piSessionID)),
    new Pair(new Scalar("message_id"), new Scalar(identity.messageID)),
    new Pair(new Scalar("raw_response"), literal(rawResponse)),
  );
  document.contents = map;
  return Buffer.from(
    document.toString({
      indent: 2,
      lineWidth: 0,
      defaultStringType: "QUOTE_DOUBLE",
      defaultKeyType: "PLAIN",
    }),
  );
}

export function captureOpaqueSettlementEvidence(content, identity) {
  const rawResponse = projectPersistedAssistantText(content);
  return {
    rawResponse,
    bytes: identity
      ? buildSettlementEvidence(identity, rawResponse)
      : undefined,
  };
}
