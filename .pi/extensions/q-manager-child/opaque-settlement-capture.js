/**
 * Projects the persisted Pi assistant message without rendering or normalizing it.
 * Pi assistant content also contains thinking and tool-call parts; neither is evidence.
 */
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

function openerAt(raw, start, end) {
  let cursor = start;
  while (raw[cursor] === "`") cursor++;
  const runLength = cursor - start;
  if (runLength < 3) return undefined;
  while (raw[cursor] === " " || raw[cursor] === "\t") cursor++;
  const languageStart = cursor;
  while (/[A-Za-z]/.test(raw[cursor] ?? "")) cursor++;
  const language = raw.slice(languageStart, cursor);
  if (!/^(yaml|yml)$/i.test(language)) return undefined;
  while (raw[cursor] === " " || raw[cursor] === "\t" || raw[cursor] === "\r")
    cursor++;
  if (cursor !== end && raw[cursor] !== "\n") return undefined;
  return { runLength, language };
}

function closerAt(raw, start, end, runLength) {
  let cursor = start;
  while (raw[cursor] === "`") cursor++;
  if (cursor - start !== runLength) return false;
  while (raw[cursor] === " " || raw[cursor] === "\t" || raw[cursor] === "\r")
    cursor++;
  return cursor === end || raw[cursor] === "\n";
}

/**
 * Lexically copies qualifying YAML/YML fences. This deliberately does not
 * decode YAML or assign meaning to its contents.
 */
export function captureOpaqueYamlFences(rawResponse) {
  const blocks = [];
  let lineStart = 0;
  while (lineStart < rawResponse.length) {
    const openerEnd = lineEnd(rawResponse, lineStart);
    const opener = openerAt(rawResponse, lineStart, openerEnd);
    if (!opener) {
      lineStart = openerEnd;
      continue;
    }

    let closerStart = openerEnd;
    let matched = false;
    while (closerStart < rawResponse.length) {
      const closerEnd = lineEnd(rawResponse, closerStart);
      if (closerAt(rawResponse, closerStart, closerEnd, opener.runLength)) {
        blocks.push({
          language: opener.language,
          raw: rawResponse.slice(lineStart, closerEnd),
        });
        lineStart = closerEnd;
        matched = true;
        break;
      }
      closerStart = closerEnd;
    }
    if (!matched) lineStart = openerEnd;
  }
  return blocks;
}

/**
 * The extension's future envelope serializer consumes this neutral evidence
 * object directly. It intentionally has no completion or routing semantics.
 */
export function captureOpaqueSettlementEvidence(content) {
  const rawResponse = projectPersistedAssistantText(content);
  return {
    rawResponse,
    fencedYamlBlocks: captureOpaqueYamlFences(rawResponse),
  };
}
