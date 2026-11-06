-- name: UpsertAccount :one
INSERT INTO accounts (name)
VALUES (?)
ON CONFLICT(name) DO UPDATE SET name = excluded.name
RETURNING *;

-- name: GetAccount :one
SELECT * FROM accounts WHERE name = ?;

-- name: ListAccounts :many
SELECT * FROM accounts ORDER BY name;

-- name: UpsertInstrument :one
INSERT INTO instruments (symbol, name, asset_class, liquidity_rank, allows_fractional, buy_proxy)
VALUES (?, ?, ?, ?, ?, ?)
ON CONFLICT(symbol) DO UPDATE SET
    name = excluded.name,
    asset_class = excluded.asset_class,
    liquidity_rank = excluded.liquidity_rank,
    allows_fractional = excluded.allows_fractional,
    buy_proxy = excluded.buy_proxy
RETURNING *;

-- name: GetInstrument :one
SELECT * FROM instruments WHERE symbol = ?;

-- name: ListInstruments :many
SELECT * FROM instruments ORDER BY symbol;

-- name: UpsertPosition :one
INSERT INTO positions (account_id, symbol, quantity, price, market_value)
VALUES (?, ?, ?, ?, ?)
ON CONFLICT(account_id, symbol) DO UPDATE SET
    quantity = excluded.quantity,
    price = excluded.price,
    market_value = excluded.market_value
RETURNING *;

-- name: ListPositions :many
SELECT 
    p.id,
    p.account_id,
    p.symbol,
    p.quantity,
    p.price,
    p.market_value,
    p.created_at,
    a.name as account_name,
    i.name as instrument_name,
    i.asset_class,
    i.liquidity_rank,
    i.allows_fractional
FROM positions p
JOIN accounts a ON p.account_id = a.id
JOIN instruments i ON p.symbol = i.symbol
ORDER BY p.market_value DESC;

-- name: ListPositionsSortedByAccount :many
SELECT 
    p.id,
    p.account_id,
    p.symbol,
    p.quantity,
    p.price,
    p.market_value,
    p.created_at,
    a.name as account_name,
    i.name as instrument_name,
    i.asset_class,
    i.liquidity_rank,
    i.allows_fractional
FROM positions p
JOIN accounts a ON p.account_id = a.id
JOIN instruments i ON p.symbol = i.symbol
ORDER BY a.name, p.market_value DESC;

-- name: ListPositionsSortedByClass :many
SELECT 
    p.id,
    p.account_id,
    p.symbol,
    p.quantity,
    p.price,
    p.market_value,
    p.created_at,
    a.name as account_name,
    i.name as instrument_name,
    i.asset_class,
    i.liquidity_rank,
    i.allows_fractional
FROM positions p
JOIN accounts a ON p.account_id = a.id
JOIN instruments i ON p.symbol = i.symbol
ORDER BY i.asset_class, p.market_value DESC;

-- name: ListPositionsSortedBySymbol :many
SELECT 
    p.id,
    p.account_id,
    p.symbol,
    p.quantity,
    p.price,
    p.market_value,
    p.created_at,
    a.name as account_name,
    i.name as instrument_name,
    i.asset_class,
    i.liquidity_rank,
    i.allows_fractional
FROM positions p
JOIN accounts a ON p.account_id = a.id
JOIN instruments i ON p.symbol = i.symbol
ORDER BY p.symbol;

-- name: DeleteAllPositions :exec
DELETE FROM positions;

-- name: UpsertTarget :one
INSERT INTO targets (asset_class, weight)
VALUES (?, ?)
ON CONFLICT(asset_class) DO UPDATE SET
    weight = excluded.weight,
    updated_at = CURRENT_TIMESTAMP
RETURNING *;

-- name: GetTarget :one
SELECT * FROM targets WHERE asset_class = ?;

-- name: ListTargets :many
SELECT * FROM targets ORDER BY asset_class;

-- name: DeleteAllProposedTrades :exec
DELETE FROM proposed_trades;

-- name: InsertProposedTrade :one
INSERT INTO proposed_trades (account_id, symbol, side, shares, dollars, rounded_shares, reason)
VALUES (?, ?, ?, ?, ?, ?, ?)
RETURNING *;

-- name: ListProposedTrades :many
SELECT 
    t.id,
    t.account_id,
    t.symbol,
    t.side,
    t.shares,
    t.dollars,
    t.rounded_shares,
    t.reason,
    t.created_at,
    a.name as account_name,
    i.name as instrument_name
FROM proposed_trades t
JOIN accounts a ON t.account_id = a.id
JOIN instruments i ON t.symbol = i.symbol
ORDER BY t.account_id, t.side DESC, ABS(t.dollars) DESC;

-- name: GetTotalMarketValue :one
SELECT COALESCE(SUM(market_value), 0) as total FROM positions;

-- name: GetClassBreakdown :many
SELECT 
    i.asset_class,
    COALESCE(SUM(p.market_value), 0) as total_value
FROM instruments i
LEFT JOIN positions p ON i.symbol = p.symbol
GROUP BY i.asset_class
ORDER BY i.asset_class;

-- name: UpsertInstrumentOverride :one
INSERT INTO instrument_overrides (symbol, asset_class, liquidity_rank)
VALUES (?, ?, ?)
ON CONFLICT(symbol) DO UPDATE SET
    asset_class = excluded.asset_class,
    liquidity_rank = excluded.liquidity_rank
RETURNING *;

-- name: GetInstrumentOverride :one
SELECT * FROM instrument_overrides WHERE symbol = ?;
