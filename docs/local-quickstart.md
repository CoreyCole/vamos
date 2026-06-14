# Local quickstart

## Prerequisites

- Go toolchain used by this repository.
- `just`.
- A Google account you can use for local login.
- Optional: licensed Datastar Pro bundle. If absent, Vamos uses public Datastar plus compatibility polyfills.

## 1. Create Google OAuth credentials

Create a Google OAuth 2.0 Web application client. Add this authorized redirect URI:

```text
http://localhost:4200/auth/google/callback
```

Download the client secret JSON to a local path outside git.

## 2. Create local config

Copy the examples:

```bash
cp config.example.yml config.local.yml
cp .env.example .env
```

Set these values in `config.local.yml` or environment variables:

```yaml
auth:
  google_credentials_file: /absolute/path/to/google-client-secret.json
  whitelisted_emails:
    - developer@example.com
  allowed_domains: []
```

Use one `whitelisted_emails` entry for the first local run. Leave `allowed_domains` empty until configuring a team deployment.

## 3. Point Vamos at your host data

Set `runtime.thoughts_repo` and `runtime.thoughts_root` to a host-owned directory. For a local trial, the host directory can be a small private repo or scratch directory that contains `thoughts/`.

## 4. Build and run

```bash
export VAMOS_CONFIG=$PWD/config.local.yml
just build --no-restart
go run ./cmd/server
```

Open `http://localhost:4200` and sign in with the whitelisted email.

## 5. Datastar Pro optional path

No Pro asset is required for the quickstart. Missing `static/js/datastar-pro-v1.js` may produce a warning; the browser falls back to public Datastar and `/js/vamos-datastar-polyfills.js`.

If you have a licensed Pro bundle:

```bash
export VAMOS_DATASTAR_PRO_ASSET=/absolute/path/to/datastar-pro-v1.js
just build --no-restart
```

## Troubleshooting

- OAuth redirect mismatch: confirm the callback URI is exactly `http://localhost:4200/auth/google/callback`.
- Login denied: confirm the signed-in Google account appears in `whitelisted_emails`.
- Blank reactive UI with no Pro asset: inspect browser console for public CDN load errors.
