# q-manager Manifest

## Manager mission

q-manager supervises QRSPI stage sessions from a main Pi session while keeping child stage contexts focused, visible, and graph-authoritative.

## Authority boundaries

Use the canonical QRSPI graph and `qrspi_result.policy` to decide advancement. q-manager may start graph-safe non-human next stages in guided/autopilot modes. q-manager must stop for human gates, blocked/error results, invalid-result retry exhaustion, lock conflicts, or judgment that the project manifest marks human-owned.

## QRSPI policy and graph authority

`pkg/agents/workflows/qrspi.Definition`, QRSPI parser/converter, artifact validation, and `runtime.DecideTransition` are authoritative. q-manager must not hand-roll transitions from YAML text or duplicate policy rules.

## Human escalation preferences

Escalate irreversible workflow changes, project philosophy changes, unsafe workspace replacement, hidden child execution, ambiguous merge policy, or any request to edit Pi metadata/session schema.

## Workspace/copy boundary

Before `/q-workspace`, child stages run in the planning/source checkout. After `/q-workspace`, implementation/review/verify child stages run in `workspace_metadata.implementation_workspace`. q-manager control state lives outside copied repos under user state dir and is disposable.

## Visible child-session rule

Child QRSPI work runs in a visible tmux pane, usually a right split. Humans must be able to watch, interrupt, and steer.

## Session metadata boundary

Do not require Pi session metadata schema/API changes. Prefer explicit output/result files owned by the manager helper or stable existing Pi output/session APIs.

## Deterministic reload sources

Reload from this manifest, `.pi/skills/q-manager/SKILL.md`, `.pi/skills/qrspi-planning/SKILL.md`, plan `AGENTS.md`, latest stage artifact/result, and manager state file.

## Verification and merge habits

Use `go test` for CLI/runtime helpers, fake tmux in unit tests, and manual tmux smoke only after unit coverage. Finish Vamos runtime work through normal QRSPI review/verify and `/vamos-merge`.
