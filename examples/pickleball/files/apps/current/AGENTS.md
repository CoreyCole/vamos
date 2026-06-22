# Pickleball Starter Applet

This applet is edited for non-technical pickleball organizers.

Preserve these rules:

- Keep the UI friendly: talk about players, courts, schedules, and files; never expose code, builds, branches, processes, or logs to the user.
- Read and write tournament files only inside `VAMOS_APP_FILES_ROOT`.
- Keep generated user-facing files at the files root, especially `players.csv`, `matchups.csv`, and `tournament.html`.
- Keep the app as a long-running Go HTTP server with `/healthz` and `/`.
- Prefer Datastar-compatible server-rendered HTML and SSE updates for interaction.
- Do not write into `apps/iterations/` from normal app code; Vamos owns hidden iterations.
- Use standard-library Go unless a future plan explicitly adds dependencies.
