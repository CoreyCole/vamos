# Daily Wordle Applet

Go + templ + Datastar Wordle clone with SQLite persistence and hot reload.

## Behavior

- One deterministic answer per local calendar date.
- The user's timezone determines the daily boundary.
- Same vendored word file is used for answer selection and valid guesses.
- Username-only login resumes the same user's daily board on another device.

## Run

```bash
just build
just status
```

Open <http://127.0.0.1:8081/>.

`just build` starts:

- stable dev proxy: `0.0.0.0:${PORT:-8081}`
- Air backend: `127.0.0.1:${BACKEND_PORT:-18081}`

Logs live in `.run/proxy.log` and `.run/air.log`. Durable applet files live under `VAMOS_APP_FILES_ROOT`, defaulting to `./files`.

## Word list

Vendored source:

```text
https://gist.githubusercontent.com/puls/9fa72925b4527c636bf1de575006fb9a/raw/e18d17f9e21f0701b11754a483d88d6eee34733c/words.txt
```

Refresh manually:

```bash
curl -fsSL 'https://gist.githubusercontent.com/puls/9fa72925b4527c636bf1de575006fb9a/raw/e18d17f9e21f0701b11754a483d88d6eee34733c/words.txt' -o internal/words/words.txt
```
