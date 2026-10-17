---
vamos_artifact: applet
applet:
  id: rebalancer
  title: Portfolio Rebalancer
  kind: datastar
  files_root: files
  app_dir: .
  route: /examples/rebalancer
  app_route: /examples/rebalancer/app/
  start_command: [just, build]
  health_path: /healthz
  port: 8082
  root_aliases:
    - pattern: /events
      methods: [GET]
    - pattern: /positions
      methods: [POST]
    - pattern: /rebalance
      methods: [POST]
    - pattern: /ingest
      methods: [POST]
---

# Portfolio Rebalancer Applet

A household stock-portfolio rebalancer demo applet using Go, SQLite, sqlc, and Datastar SSE.

## Architecture

Strict separation of concerns:
- CSV → ingest → SQLite → sqlc → domain (classify, rebalance) → HTTP + Datastar SSE
- No SQL in handlers
- No SSE in SQL
- Rebalancer is a pure function

## Features

- Multi-account position tracking (IRA, Roth IRA, Joint, Taxable)
- Six asset classes: US Equity, Intl Equity, Thematic Equity, Gold, Short Duration, Cash
- Configurable household-level target allocation
- Rebalancing with respect to:
  - Cash isolation (no cross-account cash movement)
  - Liquidity preferences (sell least liquid first when overweight)
  - Whole-share constraints for non-fractional instruments
  - Buy proxy selection (existing holdings or defaults)

## Usage

Run `just build` after source changes to regenerate code, test, compile, restart, and healthcheck.

Check `just status` and `.run/server.log` for runtime state.

Use `VAMOS_APP_FILES_ROOT` for durable applet files (default: `./files`).

## Files

- `cmd/server/main.go` - Entry point
- `internal/db/` - Database initialization
- `internal/ingest/` - CSV loading
- `internal/classify/` - Asset class resolution
- `internal/rebalance/` - Pure rebalancing logic
- `internal/web/` - HTTP handlers and templates
- `query/queries.sql` - sqlc queries
- `schema.sql` - Database schema
- `data/` - Fixture CSV files

## Notes

- Do not edit generated `*_templ.go` or `internal/db/dbgen/*` files directly
- Positions are loaded from `data/acme_positions.csv` (~$533k across four accounts)
- Instrument mappings seed from `data/asset_map.csv`
- Target weights must sum to 100%; the app shows blockers for any constraints
