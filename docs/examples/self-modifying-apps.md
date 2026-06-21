# Self-modifying app pattern

Vamos self-modifying examples separate a normal application shell from an AI-edited generated bundle.

## Pattern

1. **Shell is normal Vamos UI.** Build the page with Go, templ, Datastar CQRS, short POSTs, and SSE state streams.
2. **AI edits a generated bundle workspace.** The mutable workspace is app data, not live Vamos source.
3. **Runner builds and runs the bundle.** `pkg/agents/generatedgo` compiles and runs one-shot Go with cwd, env, timeout, output, manifest, and artifact guardrails.
4. **Successful runs create immutable snapshots.** Snapshots copy generated source and artifacts under a Thoughts-relative directory.
5. **Generated artifacts use Thoughts renderers.** `app.html` renders through the sandboxed HTML iframe renderer; `results.csv` renders through the CSV table renderer.
6. **History is AI memory.** Snapshot summaries help the agent interpret prompts like “go back to the version where partners rotated more.” Normal users should not need run IDs or filesystem paths.

## Generated bundle contract

A v1 bundle is a one-shot Go program. It reads local seed data, writes outputs to `VAMOS_GENERATED_OUTPUT_DIR`, and exits.

Required outputs:

- `app.html`
- `results.csv`
- `manifest.json`

Required manifest fields:

```json
{
  "schema_version": 1,
  "build_id": "2026-06-20_17-13-32_abcd1234",
  "mode": "one_shot",
  "prompt_summary": "prioritize new partner pairings",
  "artifacts": {
    "html": "app.html",
    "csv": "results.csv"
  }
}
```

## Guardrails

The runner is a local developer/demo guardrail, not a production multi-tenant sandbox. It provides fixed Go build/run commands, compile/run timeouts, minimal environment, output-root validation, artifact allowlisting, stdout/stderr caps, manifest validation, and source/artifact hashes.

## Future seam

The manifest reserves future mode `sse_process` for long-running generated Datastar/SSE applets. V1 examples should not start or proxy generated servers.
