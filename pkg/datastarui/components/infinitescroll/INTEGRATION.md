# InfiniteScroll (Pattern A)

Intersection-based progressive loading via `data-on:intersect` sentinels.

## Hard locks

- **Host never remorphed** — patch Items / Loading (sentinel) ids only
- **Chunk patches: View Transitions OFF** (`datastar.WithoutViewTransitions()`)
- **Items = stable history only** — live / working / latest stay **outside** Items as siblings
- Loading = same-id DOM replace on the sentinel id; first paint is Sentinel only

## AgentChat MorphMap

AgentChat Host is `#agent-chat-scroll-region`. Items reuse `#agent-chat-stable-transcript`.
Above-edge sentinel id: `agent-chat-scroll-sentinel-above`.

Chat fixture DOMIDs render with a `msg-` prefix (e.g. `#msg-ai470-density-fixture-reasoning`).
Bare `ai470-*` ids in fixture seed structs are message DOMIDs before the transcript wrapper prefixes `msg-`.

## Chat composition sketch

```
@infinitescroll.Host({ID: "agent-chat-scroll-region", ...}) {
  @Sentinel above (PatchAboveExpr)   // optional
  @Items({ID: "agent-chat-stable-transcript"}) { stable msgs }
  #agent-chat-live-transcript        // OUTSIDE Items
  #chat-latest                       // OUTSIDE Items
}
```
