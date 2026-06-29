# Wordle Starter Applet

This applet is edited for non-technical Wordle players.

Preserve these rules:

- Keep the UI friendly: talk about words, guesses, letters, and games; never expose code, builds, branches, processes, or logs to the user.
- Keep the hidden answer and all game state on the backend.
- The frontend may submit forms and open one Datastar SSE stream, but it must not score guesses or know the answer.
- Read and write app files only inside `VAMOS_APP_FILES_ROOT`.
- Keep user-facing dictionary files at the files root, especially `valid_words.txt` and `answers.txt`.
- Keep the app as a long-running Go HTTP server with `/healthz` and `/`.
- Prefer Datastar-compatible server-rendered HTML and SSE updates for interaction.
- Do not write into `apps/iterations/` from normal app code; Vamos owns hidden iterations.
- Use standard-library Go plus the existing templ dependency unless a future plan explicitly adds more dependencies.
