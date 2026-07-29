---
name: project-plan-summary
description: Generates a single-file collapsible `index.html` summary for a Vamos/QRSPI nested project plan. Use when summarizing project milestones, ticket estimates, Linear project structure, or creating a team-facing project roadmap from `thoughts/...` project-planning artifacts.
---

# Project Plan Summary

Generate a team-facing single-page `index.html` for a Vamos/QRSPI nested project plan.

Output path: `<project-plan-dir>/index.html`.

## Scope

Use only for QRSPI nested project directories with milestone and ticket dirs. The page summarizes:

- project BLUF
- milestone roadmap in planned order
- milestone BLUF, end-state user stories, and optional deferred/non-goals
- ticket estimate tables per milestone
- remaining, completed, and milestone total days

Do not turn this into an archive browser. Link to detailed artifacts instead.

## Step 1: Load sources

Read only the needed project-planning sources:

1. project plan `AGENTS.md`
1. project status/routing artifact named by `AGENTS.md`
1. project provider log named by `AGENTS.md`
1. milestone `AGENTS.md` files
1. milestone `milestone-plan/design.md` when present
1. ticket `ticket.md` files under `milestones/*/tickets/pro-*`
1. thoughts root `AGENTS.md` for public link policy

Use Linear for current issue metadata: title, URL, milestone, status, native estimate. Treat `In Review`, `On Stage`, and `Done` as done for progress/remaining calculations; all other non-canceled statuses count as remaining.

## Step 2: Validate Linear vs thoughts inventory

Build two inventories:

- Linear project issues grouped by Linear milestone
- local `milestones/*/tickets/pro-*` dirs plus provider-log routing entries

Stop and ask the lead engineer for any discrepancy:

| Discrepancy | Action |
|---|---|
| Linear issue missing ticket dir | Ask whether to create routing dir or exclude |
| Ticket dir missing from Linear project/milestone | Ask whether to update Linear or docs |
| Milestone mismatch | Ask lead to choose Linear or docs truth |
| Duplicate ticket dirs for same issue | Stop until fixed |
| Material title mismatch | Ask whether to update docs or Linear |

Linear is authoritative for current title, URL, status, and estimate. Thoughts docs are authoritative for milestone order and planning context.

## Step 3: Resolve estimates

Use Linear native t-shirt estimates. The Linear API/CLI stores them as numeric values for `issueEstimationType: "tShirt"`.

| Linear estimate | Label | Days |
|---:|---|---:|
| 1 | XS | 0.25 |
| 2 | S | 0.5 |
| 3 | M | 1 |
| 5 | L | 3 |
| 8 | XL | 5 |

Rules:

- Valid Linear native estimate values are `1`, `2`, `3`, `5`, and `8` for `XS`, `S`, `M`, `L`, and `XL`.
- `null`, `0`, missing, `No estimate`, and `-` are missing.
- Non-integer or values outside `1`, `2`, `3`, `5`, and `8` are invalid; stop and ask the lead.
- Never guess estimates.
- Collect missing estimates one milestone at a time.
- After the lead answers for a milestone, update Linear immediately before moving on.

Prompt shape:

```text
Milestone: Eligibility & Qualification Overrides
1. PRO-9666 feat(...) — missing
2. PRO-9667 feat(...) — missing
3. PRO-9668 test(...) — missing

Reply with sizes:
1. M
2. L
3. S
```

Update with Linear CLI numeric mapping, e.g. `linear-cli i update PRO-9666 -e 3` for `M`, `-e 5` for `L`, and `-e 8` for `XL`.

## Step 4: Resolve milestone summaries

For each milestone, produce:

- BLUF: one sentence
- End-state user stories: 2-4 bullets
- Not in scope / deferred: optional; omit if empty

Ground summaries in existing docs:

1. Prefer reviewed `milestone-plan/design.md`.
1. Else use milestone `AGENTS.md` goal/scope plus ticket bodies.
1. Else start a `/grill-me` style interview with the lead engineer.

Do not invent product outcomes. If context is missing or too vague, interview the lead one question at a time until BLUF, user stories, and non-goals are clear.

## Step 5: Render `index.html`

Use one static HTML file. All milestones are collapsed by default. Collapsed headers show planned order, milestone name, ticket count, and remaining days.

When the file is rendered through the Vamos thoughts HTML applet, use native `<details>/<summary>` for collapsible sections and do not load JavaScript. The applet iframe is sandboxed and the CSP allows only self-hosted scripts; CDN Datastar is blocked and Datastar can hit `sessionStorage` errors without `allow-same-origin`. Parent-page Datastar does not hydrate inside the child iframe.

```html
<details class="milestone-card">
  <summary class="milestone-header">
    <span>01 Architecture</span>
    <span>6 tickets · 4 days remaining</span>
  </summary>
  <section class="milestone-body">...</section>
</details>
```

Only use Datastar when the output is served outside the Vamos sandbox or the applet runtime explicitly provides a safe self-hosted Datastar bundle for child documents.

### Page structure

1. Header
   - project name
   - concise project BLUF
   - Linear project link
   - generated timestamp
   - remaining estimated days
   - completed vs total estimated days
   - progress bar based on Linear status-derived done days
1. Estimate legend
   - `XS 0.25d · S 0.5d · M 1d · L 3d · XL 5d`
1. Milestone roadmap
   - one collapsible card per milestone, in planned order
   - include unticketed milestones inline, not in a separate section
1. Footer
   - source docs links
   - `Estimates sourced from Linear`

### Milestone card

Collapsed header examples:

```text
09 Eligibility & Qualification Overrides        3 tickets · 3 days remaining
10 Reporting Snapshots                          Not ticketed · not estimated
```

Expanded body:

```text
BLUF
[one sentence]

End-state user stories
- ...
- ...

Not in scope / deferred
- ...

Tickets
[HTML table]
```

Omit `Not in scope / deferred` when empty. If no tickets exist, omit the table and show `Not ticketed yet`.

### Ticket table

Use real HTML tables styled like Vamos markdown tables:

```html
<div class="table-wrapper">
  <table>
    <thead>
      <tr><th>Ticket</th><th>Status</th><th>Size</th><th>Time (days)</th></tr>
    </thead>
    <tbody>
      <tr>
        <td><a href="https://linear.app/...">PRO-9666 feat(...)</a></td>
        <td><span class="ticket-status status-open">Todo</span></td>
        <td>M</td>
        <td class="num">1</td>
      </tr>
      <tr class="total-row"><td><strong>Total done</strong></td><td></td><td></td><td class="num"><strong>0</strong></td></tr>
      <tr class="total-row remaining-row"><td><strong>Total remaining</strong></td><td></td><td></td><td class="num"><strong>1</strong></td></tr>
      <tr class="total-row"><td><strong>Total estimated</strong></td><td></td><td></td><td class="num"><strong>1</strong></td></tr>
    </tbody>
  </table>
</div>
```

Include equivalent CSS in the standalone file:

```css
.table-wrapper { margin: 1.5rem 0; width: fit-content; overflow: hidden; border-radius: .5rem; border: 1px solid rgba(148,163,184,.35); }
table { border-collapse: collapse; }
thead { background: rgba(148,163,184,.12); }
tbody tr { border-bottom: 1px solid rgba(148,163,184,.2); }
tbody tr:last-child { border-bottom: 0; }
th, td { padding: .75rem 1rem; font-size: .875rem; text-align: left; }
.num { text-align: right; }
.total-row { background: rgba(148,163,184,.08); }
```

## Step 6: Verify

Before done:

1. `rg "cdn.jsdelivr|data-on:|data-attr:|data-show|data-signals" <project-plan-dir>/index.html` returns nothing for Vamos thoughts-viewer output.
1. Open locally or through the thoughts viewer when possible.
1. Confirm all milestone cards start collapsed.
1. Confirm each header total matches ticket rows.
1. Confirm project total excludes unticketed milestones and does not treat them as zero.
1. Run the host docs sync command when available, e.g. `just sync-thoughts`.
