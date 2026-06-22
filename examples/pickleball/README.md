# Self-modifying pickleball example

This example demonstrates Vamos as a framework for mobile-friendly Go + templ + Datastar applets that a non-technical person can change through chat.

The product pattern is:

1. A stable Vamos workbench shows **Files** and **Chat**.
1. The user asks for app behavior in plain language.
1. Vamos uses Temporal + Pi/Agent Chat to edit a hidden applet iteration.
1. Vamos builds, health-checks, and promotes a safe iteration behind the scenes.
1. The current app stays available if a change fails.

User-visible files live in `files/`. The committed starter applet lives in `files/apps/current/`. Generated iterations live in `files/apps/iterations/` and are hidden from normal users.

## Try the starter applet

From this checkout:

```bash
cd examples/pickleball/files/apps/current
go test ./...
PORT=8080 VAMOS_APP_FILES_ROOT="$OLDPWD/examples/pickleball/files" go run .
```

Open <http://127.0.0.1:8080/>. The app reads `players.csv`, writes `matchups.csv`, and updates `tournament.html` inside `examples/pickleball/files/`.

## Example prompts

- `Prioritize new partner pairings over skill balance.`
- `Make the schedule easier to read on a phone.`
- `Add a note explaining why each matchup was chosen.`

## Boundary

The Vamos shell owns chat, safety checks, build, health check, promotion, recovery, and friendly explanations. The applet owns pickleball rules, files, and presentation. Deterministic edits are only for tests/fixtures; the product path uses Temporal + Pi/Agent Chat.
