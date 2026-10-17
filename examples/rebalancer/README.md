# Portfolio Rebalancer

A household stock-portfolio rebalancer demo applet built with Go, SQLite, sqlc, and Datastar SSE.

## Quick Start

```bash
go run ./cmd/server
```

Then open http://localhost:8082 in your browser.

## Features

- Multi-account portfolio tracking (IRA, Roth IRA, Joint, Taxable)
- Six asset classes: US Equity, Intl Equity, Thematic Equity, Gold, Short Duration, Cash
- Configurable household-level target allocation
- Smart rebalancing with cash isolation and liquidity preferences
- List controls: sort by value/account/class/symbol, show value/quantity/price

## How It Works

1. Click "Load Positions" to import the fixture data (~$548k across 4 accounts)
2. Adjust target allocation sliders (must sum to 100%)
3. View proposed buy/sell trades and any constraints
4. Sort positions by account, class, or symbol
5. Switch display between market value, quantity, or last price

## Architecture

```
CSV → ingest → SQLite → sqlc → domain (classify, rebalance) → HTTP + Datastar SSE
```

- **No SQL in handlers**: All database access through sqlc
- **No SSE in SQL**: Business logic separate from data access
- **Rebalancer is pure**: `Rebalance()` operates only on passed data

## Testing

```bash
go test ./...
```

Tests cover:
- Asset classification (overrides, heuristics, seed mappings)
- Rebalancing logic (household drift, cash isolation, whole-share constraints)

## Building from source

Requires:
- Go 1.25+
- sqlc
- templ

```bash
sqlc generate
templ generate
go build -o server ./cmd/server
./server
```

Or use the justfile (requires just):

```bash
just build
```
