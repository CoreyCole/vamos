# Concepts

## Vamos runtime

Vamos is the reusable server runtime. It owns generic behavior for reading configured artifacts, serving workbench UI, running shared Agent Chat/Pi sessions, and coordinating workflow runtimes such as QRSPI. It must not own organization-specific policy, host names, credentials, or deployment commands.

## Host application or repository

A host application or host repository wraps Vamos for a team. The host owns config, secrets, public URLs, reverse proxy rules, service management, linked project paths, and local operating docs. Host code and config can be private while the Vamos runtime stays reusable.

## Thoughts root

The thoughts root is the durable knowledge tree for plans, research, ADRs, reviews, and handoffs. Vamos treats the filesystem artifacts as source of truth. Databases and indexes are projections that should be rebuildable from the thoughts tree.

## Agent Chat and Pi sessions

Agent Chat and Pi sessions are shared organizational context. Plan-owned sessions and artifacts are intended for multiplayer discovery and handoff, not private per-user plan history. User and workspace metadata are useful for provenance and routing, not for hiding plan-owned history.

## QRSPI

QRSPI is the staged workflow for question, research, design, outline, plan, implementation, review, and verification. Each stage writes durable artifacts under `thoughts/` so decisions and implementation evidence survive individual chat sessions.

## Linked projects

Linked projects are repositories configured by the host. A project can have a working checkout for active edits and a clean baseline checkout for history reads, workspace seeding, or release lanes. The host decides branch freshness and cleanliness policy.

## Workspaces

Standalone workspace mode points Vamos at one checkout. Manager workspace mode creates copied implementation workspaces and tracks metadata under `.vamos/`. These copied directories are normal filesystem copies, not git worktrees.

## Configured lanes

Configured lanes are durable host-defined targets such as `stage` and `main`. They represent how a host wants to test, promote, or inspect a project. Vamos provides reusable lane and workflow machinery; the host owns lane names, domains, services, and release commands.
