# q-manager guide for the pickleball app

The pickleball app is for a non-technical tournament organizer. They should describe desired app behavior in plain language: matchup rules, player data, schedule layout, printed copy, or file output.

q-manager handles the technical work:

- turns the plain-language request into a safe implementation plan;
- asks Pi/Agent Chat to edit a hidden applet iteration;
- runs the required checks, build, and app health check;
- promotes only a working change to the current app;
- keeps the last good app available when a change fails;
- explains success or failure without code, branches, logs, paths, or process details.

The user-visible document space is `examples/pickleball/files/`. Root files such as `players.csv`, `matchups.csv`, and `tournament.html` are the durable app data users can browse, edit, download, or share.

The committed starter applet lives in `examples/pickleball/files/apps/current/`. Generated attempts live under `examples/pickleball/files/apps/iterations/`; that directory is hidden from normal users and ignored by git.

Product prompt edits use Vamos Temporal plus Pi/Agent Chat. Deterministic patching exists only for tests and local fixtures; it must not be presented as the real product path.

Normal UI rule: show Files/app and Chat. Hide planning, implementation, builds, promotion, iterations, workspaces, run IDs, manifests, stack traces, filesystem paths, and recovery mechanics.
