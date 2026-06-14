# Host configuration

A Vamos host owns organization-specific config and policy. Start from `config.example.yml`, keep secrets outside git, and pass the selected config with `VAMOS_CONFIG`.

## `app`

Human-facing labels for the server and account. These names appear in UI and should match the host team or deployment, not reusable runtime internals.

## `runtime`

Host-owned artifact and state paths:

- `thoughts_repo`: repository or directory that owns durable thoughts artifacts.
- `thoughts_root`: root directory served by Vamos for plans, research, ADRs, and handoffs.
- `state_dir`: host-local runtime state.
- `database_path`: projection/cache database path.

Back up the thoughts root and host config. Treat the database as rebuildable when workflows are driven from durable thoughts artifacts.

## `web`

HTTP listen and public browser-facing settings:

- `listen_address`: local bind address for the Vamos process.
- `public_base_url`: canonical URL users visit.
- `cors_allowed_origins`: origins allowed to call the host.

Reverse proxy, TLS, DNS, and workspace domains are host responsibilities.

## `auth`

Google OAuth and access policy are host-owned:

- `google_credentials_file`: path to the Google OAuth web client JSON.
- `whitelisted_emails`: explicit individual email allowlist, best for first local runs or small teams.
- `allowed_domains`: team/domain allowlist for deployments.

Use one whitelisted email for the first local quickstart. Add allowed domains only when the host is ready to grant team access.

## `projects`

Project definitions tell Vamos where code lives:

- `default_repo` and `default_checkout`: initial project/checkout selection.
- `github_url`: project remote.
- `default_branch`: trunk branch for freshness checks.
- `baseline_checkout`: clean/latest checkout used for history reads or workspace seeds.
- `checkouts`: working and baseline checkout roots plus cleanliness/freshness policy.

Keep absolute paths in host config. Durable thoughts artifacts should still prefer thoughts-relative references.

## `workspaces`

Workspace mode controls how Vamos opens or creates implementation environments:

- `standalone`: one local checkout, simple local development.
- `manager`: copied implementation checkouts, metadata under `.vamos/`, and configured release/checkpoint lanes.

Workspace domains, checkout parent directories, lane names, and module markers are host-owned.

## `deploy`

Deployment config points to host-owned service names and restart/rebuild hooks. Host executors own private commands. Reusable Vamos must not hardcode service names, domains, or organization-specific deploy policy.

## Datastar assets

A licensed Datastar Pro bundle is optional. If `VAMOS_DATASTAR_PRO_ASSET` is unset and no local Pro bundle exists, the browser uses public Datastar plus `/js/vamos-datastar-polyfills.js` for the small Pro contracts Vamos uses.

## Generic host repo layout

```text
company-agents/
  config/company-agents.yml
  thoughts/
  deploy/
  README.md
```
