# Vamos Pi resources

This directory is for Vamos-specific Pi resources:

- `skills/` — skills that only make sense for Vamos runtime development
- `prompts/` — Vamos-specific prompt templates
- `extensions/` — Vamos-specific Pi extensions

Project-local skills:

- `hermes-vamos-chat-delegation` — Hermes background delegation contract for `vamos chat start` / `steer`
- `q-hermes-manager` — Hermes-managed QRSPI orchestration with background Pi processes instead of tmux q-manager panes

Project-local extensions:

- `q-manager-parent` — `/q-manager start-next|continue` parent wrapper; samples live Pi context usage and triggers native parent compaction after q-manager delivery is queue-safe.

Put broadly useful, cross-repository skills in `.agents/` instead. In local
Chestnut development, `.agents` is a symlink to a shared agent configuration
checkout; `.pi` is intentionally project-local to Vamos.

Do not commit Pi runtime state here. Keep sessions, package installs, auth, and
other generated files ignored.
