# Generated bundle editing rules

This directory is the starter generated app bundle for the Vamos pickleball example.

AI edits should preserve these rules:

- Write generated outputs only under `VAMOS_GENERATED_OUTPUT_DIR`.
- Always produce `app.html`, `results.csv`, and `manifest.json`.
- Keep `manifest.json` schema version `1`, mode `one_shot`, and artifacts `app.html` / `results.csv`.
- Do not start a server or long-running process in v1.
- Do not use network access.
- Prefer standard-library Go unless a future plan explicitly adds dependencies.
- Keep the app useful inside a sandboxed iframe with no parent DOM assumptions.
- Treat `players.csv` as editable seed data for generated matchup behavior.

The bundle may change matchup logic, CSV columns, copy, colors, or HTML layout when the user asks the app to modify itself.
