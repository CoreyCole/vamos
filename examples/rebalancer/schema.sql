CREATE TABLE IF NOT EXISTS accounts (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    name TEXT NOT NULL UNIQUE,
    created_at TEXT NOT NULL DEFAULT CURRENT_TIMESTAMP
);

CREATE TABLE IF NOT EXISTS instruments (
    symbol TEXT PRIMARY KEY,
    name TEXT NOT NULL,
    asset_class TEXT NOT NULL CHECK (
        asset_class IN (
            'US Equity',
            'Intl Equity',
            'Thematic Equity',
            'Gold',
            'Short Duration',
            'Cash',
            'Unclassified'
        )
    ),
    liquidity_rank INTEGER NOT NULL DEFAULT 4 CHECK (liquidity_rank BETWEEN 0 AND 4),
    allows_fractional BOOLEAN NOT NULL DEFAULT 0,
    buy_proxy TEXT,
    created_at TEXT NOT NULL DEFAULT CURRENT_TIMESTAMP
);

CREATE TABLE IF NOT EXISTS positions (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    account_id INTEGER NOT NULL REFERENCES accounts(id),
    symbol TEXT NOT NULL REFERENCES instruments(symbol),
    quantity REAL NOT NULL,
    price REAL NOT NULL,
    market_value REAL NOT NULL,
    created_at TEXT NOT NULL DEFAULT CURRENT_TIMESTAMP,
    UNIQUE(account_id, symbol)
);

CREATE TABLE IF NOT EXISTS targets (
    asset_class TEXT PRIMARY KEY,
    weight REAL NOT NULL CHECK (weight >= 0 AND weight <= 100),
    updated_at TEXT NOT NULL DEFAULT CURRENT_TIMESTAMP
);

CREATE TABLE IF NOT EXISTS proposed_trades (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    account_id INTEGER NOT NULL REFERENCES accounts(id),
    symbol TEXT NOT NULL REFERENCES instruments(symbol),
    side TEXT NOT NULL CHECK (side IN ('BUY', 'SELL')),
    shares REAL NOT NULL,
    dollars REAL NOT NULL,
    rounded_shares INTEGER NOT NULL,
    reason TEXT NOT NULL,
    created_at TEXT NOT NULL DEFAULT CURRENT_TIMESTAMP
);

CREATE TABLE IF NOT EXISTS instrument_overrides (
    symbol TEXT PRIMARY KEY,
    asset_class TEXT NOT NULL,
    liquidity_rank INTEGER,
    created_at TEXT NOT NULL DEFAULT CURRENT_TIMESTAMP
);

CREATE INDEX IF NOT EXISTS positions_account_idx ON positions(account_id);
CREATE INDEX IF NOT EXISTS positions_symbol_idx ON positions(symbol);
CREATE INDEX IF NOT EXISTS trades_account_idx ON proposed_trades(account_id);
