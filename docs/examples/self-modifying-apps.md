# Self-modifying app pattern

Vamos self-modifying examples separate a stable Vamos shell from an AI-edited applet. The flagship pickleball example is a non-technical Files/app + Chat workbench, not a developer-facing generated artifact demo.

## Flagship applet pattern

1. **Shell stays stable.** Build the page with Go, templ, Datastar CQRS, short POSTs, and SSE state streams. The normal surface shows Files/app and Chat.
1. **User asks in plain language.** The end user describes desired behavior, copy, layout, tournament rules, or file outputs. They do not manage code, builds, branches, runs, or promotion.
1. **q-manager hides the technical work.** It decomposes the request, asks Pi/Agent Chat to edit a hidden iteration, runs safety checks, builds, health-checks, promotes, and recovers.
1. **Files root is app data.** User-visible files live under the app's `files/` root. For pickleball this is `examples/pickleball/files/`.
1. **Generated iterations are hidden.** The committed starter applet lives in `files/apps/current/`; generated attempts live under `files/apps/iterations/` and are not shown in the normal Files browser.
1. **Last-good app remains available.** A failed edit keeps the current app running and reports a friendly unchanged message.

## Pickleball product path

The pickleball app uses a long-running Go + Datastar/DatastarUI applet as the current target. Prompt edits should go through Vamos Temporal plus Pi/Agent Chat and return a non-technical summary for the user. Diagnostic refs, logs, run IDs, and changed-file details are for operators and agents, not the normal user surface.

Deterministic patching is allowed only for tests and local fixtures. Fixture mode must not be described as product AI.

## One-shot generated bundles

Older or simpler examples can still use the static one-shot pattern: AI edits a generated bundle workspace, `pkg/agents/generatedgo` compiles and runs a Go program once, and successful runs create immutable artifacts such as `app.html`, `results.csv`, and `manifest.json`.

That pattern is useful for constrained demos and renderer tests, but it is not the final pickleball product target.

## Guardrails

Current generated-code guardrails are local developer/demo guardrails, not a production multi-tenant sandbox. They include fixed Go commands, compile/run timeouts, minimal environment, output-root validation, artifact allowlisting, stdout/stderr caps, manifest validation, source/artifact hashes, hidden diagnostics, and app-specific health checks.

## User-facing rule

Normal users should see Files/app + Chat only. Hide workspaces, builds, plans, workflow state, promotion, iterations, schemas, processes, manifests, run IDs, filesystem paths, stack traces, and raw logs.
