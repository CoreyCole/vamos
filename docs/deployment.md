# Deployment

Use this checklist when a host team deploys Vamos beyond a local quickstart.

## Public URL

Choose the public URL users will visit and set `web.public_base_url` to that value. Configure the reverse proxy and TLS certificate before sending users to the host.

## OAuth callback

Create or update the Google OAuth web client with this callback:

```text
<public_base_url>/auth/google/callback
```

Store the client secret JSON in a host-owned secret location and point `auth.google_credentials_file` at it.

## Access policy

Use `auth.whitelisted_emails` for explicit individual access. Use `auth.allowed_domains` for team or organization deployments. Keep the policy in host config so reusable Vamos code does not encode organization access rules.

## CORS and workspace origins

Set `web.cors_allowed_origins` to the public host and any workspace origins that must call back to the server. If manager workspaces are enabled, configure wildcard workspace DNS and TLS in the host layer.

## Secrets

Put OAuth credentials, webhook secrets, internal tokens, and deploy credentials in environment variables or a secret manager. Do not commit them to reusable Vamos code or public examples.

## Reverse proxy

Configure TLS termination, request size limits, and buffering rules that support server-sent events. Avoid proxy buffering for event streams used by reactive UI and agent workflow updates.

## Workspace manager

If using manager mode, configure:

- checkout parent directory
- metadata directory name
- workspace domain
- release/checkpoint lanes
- module marker and package subdirectory
- baseline checkout cleanliness/freshness policy

These values are deployment policy, not Vamos defaults.

## Service management

Run Vamos under the host's service manager. Configure rebuild/restart hooks and service names in the host layer. Keep host commands private to the deployment.

## Backups and recovery

Back up the thoughts root and host config. Treat the database as a rebuildable projection/cache when workflows are driven from durable thoughts artifacts. Include service config and OAuth setup in recovery docs.
