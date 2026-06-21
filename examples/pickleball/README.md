# Self-modifying pickleball example

This example demonstrates Vamos as a framework for mobile-friendly Go + templ + Datastar apps that can modify a generated bundle through AI prompts.

The reusable pattern is:

1. A normal Vamos shell owns prompt UI, workflow state, preview links, sharing, and failure display.
2. An AI edits the generated bundle workspace, not the live Vamos source tree.
3. `pkg/agents/generatedgo` compiles and runs the bundle once with bounded time, env, output, and artifact rules.
4. Successful runs publish immutable snapshots under Thoughts.
5. Generated `app.html` and `results.csv` render through the Thoughts HTML iframe and CSV table renderers.

The seed bundle in `seed-bundle/` is deliberately standard-library-only so agents and humans can inspect or rewrite it quickly.

## Try the seed bundle

From this directory's parent checkout:

```bash
rm -rf /tmp/vamos-pickleball-seed
mkdir -p /tmp/vamos-pickleball-seed
(cd examples/pickleball/seed-bundle && VAMOS_GENERATED_OUTPUT_DIR=/tmp/vamos-pickleball-seed go run .)
ls /tmp/vamos-pickleball-seed/app.html /tmp/vamos-pickleball-seed/results.csv /tmp/vamos-pickleball-seed/manifest.json
```

## Example prompts

- `Prioritize new partner pairings over skill balance.`
- `Make the preview more colorful and mobile-friendly.`
- `Add a CSV column explaining skill totals.`

## Boundary

The shell owns orchestration and user experience. The generated bundle owns matchup algorithm, CSV shape, and presentation HTML. V1 is one-shot only; long-running generated Datastar/SSE processes are reserved for a future runner mode.
