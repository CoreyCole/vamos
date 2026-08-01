# q-manager manifest

## Manager mission

Hermes supervises isolated QRSPI Pi workers while durable plan artifacts remain authoritative. Hermes owns conversation state, the live background process handle, steering, and every decision to stop or continue work. Child settlement text is opaque evidence, never lifecycle authority.

## Managed launcher

A managed launch is selected only when the launcher inherits a runtime-issued `HERMES_SESSION_ID` from the current Hermes session:

```bash
vamos hermes pi start --plan <absolute-plan-dir> [--previous-session <id>] "<bounded task>"
```

Run it in a Hermes background PTY and retain that exact process handle. The Go parent owns the child lifecycle and notification transport. The child receives the opaque Hermes identity and one write-only handoff descriptor; it receives no endpoint, credential, configuration path, or synthetic route identity.

The extension first publishes immutable evidence at:

```text
<plan>/.vamos/sessions/pi/<pi-session>/settlements/<message-id>.yaml
```

It then writes only the protocol version, launch nonce, Pi session ID, and message ID to the inherited descriptor. The parent descriptor-loads that exact file, preserves `raw_response` byte-for-byte, performs protocol-v1 capability negotiation, and requests exact-session admission. The Hermes ingress formatter applies the fixed manager-facing wrapper exactly once.

## Truthful boundary vocabulary

Report these boundaries separately:

1. child process exit;
1. immutable settlement publication;
1. notifier attempt and retry classification;
1. exact-session admission (`accepted_idle` or `accepted_queued`);
1. manager transcript append and model execution;
1. manager response submission to the original process handle;
1. receipt by the same live Pi child.

Capability success proves only protocol compatibility. Admission proves only that Hermes accepted the turn for the exact current session generation. Neither proves manager execution or reverse delivery. Timeouts after a possible write remain at-least-once uncertainty, so retry can duplicate a manager turn.

## Exact manual recovery

Retry one explicitly named immutable settlement:

```bash
vamos hermes pi notify \
  --plan <absolute-plan-dir> \
  --pi-session <pi-session-id> \
  --message-id <message-id>
```

Use `--format json` for the structured aggregate and per-event result. The command opens only the supplied plan's exact Pi session directory and exact message file, then uses the same parent notifier factory, configuration, capability preflight, retry policy, and canonical enqueue construction as managed start and continue. It does not discover a plan, inspect `manual-resume.json`, find a latest session, or follow a stale identity.

Recovery output reports publication, admission, code, detail, retryability, and uncertainty. It explicitly does not claim manager execution or reverse-child receipt. Review the immutable artifact and manager transcript before deciding whether another explicit action is warranted.

## Authority and exclusions

The managed path does not interpret `outcome`, `next`, `complete`, or fenced YAML in child text. It does not register a callback, create a schedule, scan for evidence, select a successor, automatically continue work, or invoke `pi done`. Managed start and continue do not route through `manager_thread_id`.

The old `/vamos/manager-wake` synthetic-thread endpoint is compatibility behavior for old launchers only. It is not translated into protocol v1 and must not be used as evidence of exact-session notification.

## Source-tree proof boundary

Automated tests and recovery are not the end-to-end proof. Before promoting a modified Hermes runtime, run the separately approved source-tree TUI proof and establish the complete same-session, same-generation, same-process-handle, same-Pi-child round trip. Do not infer that proof from health, capability, publication, HTTP response, or admission alone.
