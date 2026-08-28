# Vamos Pi resources

This directory is for Vamos-specific Pi resources:

- `skills/` — skills that only make sense for Vamos runtime development
- `prompts/` — Vamos-specific prompt templates

Project-local skills:

- `hermes-vamos-chat-delegation` — Hermes background delegation contract for `vamos chat start` / `steer`
- `q-hermes-manager` — Hermes-managed isolated Pi workers using `vamos hermes pi start|result|done`

There are no project-local extensions. Hermes owns worker lifecycle; Pi skills write durable artifacts and record concise CLI completions.

Put broadly useful, cross-repository skills in `.agents/` instead. In local
Chestnut development, `.agents` is a symlink to a shared agent configuration
checkout; `.pi` is intentionally project-local to Vamos.

Do not commit Pi runtime state here. Keep sessions, package installs, auth, and
other generated files ignored. QRSPI review artifacts are plan-owned files under
`thoughts/.../reviews/.../context/`; `.pi-subagents/` is disposable pi-subagents
telemetry/debug output, never a canonical QRSPI artifact path.
