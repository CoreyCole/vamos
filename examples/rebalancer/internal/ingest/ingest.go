package ingest

import (
	"context"
	"database/sql"
	"encoding/csv"
	"fmt"
	"os"
	"strconv"
	"strings"

	"example.com/vamos-rebalancer/internal/classify"
	"example.com/vamos-rebalancer/internal/db/dbgen"
)

// PositionRecord represents a row from the positions CSV
type PositionRecord struct {
	Account     string
	Symbol      string
	Quantity    float64
	Price       float64
	MarketValue float64
}

// InstrumentRecord represents a row from the asset map CSV
type InstrumentRecord struct {
	Symbol           string
	Name             string
	AssetClass       string
	LiquidityRank    int
	AllowsFractional bool
	BuyProxy         string
}

// LoadPositions reads positions from a CSV file
func LoadPositions(path string) ([]PositionRecord, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, fmt.Errorf("open positions file: %w", err)
	}
	defer f.Close()

	reader := csv.NewReader(f)
	rows, err := reader.ReadAll()
	if err != nil {
		return nil, fmt.Errorf("read positions csv: %w", err)
	}

	if len(rows) < 2 {
		return nil, fmt.Errorf("positions file has no data rows")
	}

	var records []PositionRecord
	for i, row := range rows {
		if i == 0 {
			continue // Skip header
		}

		if len(row) < 5 {
			return nil, fmt.Errorf("row %d: expected 5 columns, got %d", i+1, len(row))
		}

		qty, err := strconv.ParseFloat(row[2], 64)
		if err != nil {
			return nil, fmt.Errorf("row %d: invalid quantity: %w", i+1, err)
		}

		price, err := strconv.ParseFloat(row[3], 64)
		if err != nil {
			return nil, fmt.Errorf("row %d: invalid price: %w", i+1, err)
		}

		mv, err := strconv.ParseFloat(row[4], 64)
		if err != nil {
			return nil, fmt.Errorf("row %d: invalid market value: %w", i+1, err)
		}

		records = append(records, PositionRecord{
			Account:     strings.TrimSpace(row[0]),
			Symbol:      strings.TrimSpace(row[1]),
			Quantity:    qty,
			Price:       price,
			MarketValue: mv,
		})
	}

	return records, nil
}

// LoadInstruments reads instrument mappings from a CSV file
func LoadInstruments(path string) ([]InstrumentRecord, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, fmt.Errorf("open instruments file: %w", err)
	}
	defer f.Close()

	reader := csv.NewReader(f)
	rows, err := reader.ReadAll()
	if err != nil {
		return nil, fmt.Errorf("read instruments csv: %w", err)
	}

	if len(rows) < 2 {
		return nil, fmt.Errorf("instruments file has no data rows")
	}

	var records []InstrumentRecord
	for i, row := range rows {
		if i == 0 {
			continue // Skip header
		}

		if len(row) < 6 {
			return nil, fmt.Errorf("row %d: expected 6 columns, got %d", i+1, len(row))
		}

		rank, err := strconv.Atoi(strings.TrimSpace(row[3]))
		if err != nil {
			return nil, fmt.Errorf("row %d: invalid liquidity rank: %w", i+1, err)
		}

		fractional := strings.TrimSpace(row[4]) == "1"

		records = append(records, InstrumentRecord{
			Symbol:           strings.TrimSpace(row[0]),
			Name:             strings.TrimSpace(row[1]),
			AssetClass:       strings.TrimSpace(row[2]),
			LiquidityRank:    rank,
			AllowsFractional: fractional,
			BuyProxy:         strings.TrimSpace(row[5]),
		})
	}

	return records, nil
}

// ImportToDatabase loads CSV data into the database
func ImportToDatabase(ctx context.Context, queries *dbgen.Queries, positionsPath, instrumentsPath string) error {
	// Load instrument map first
	instRecords, err := LoadInstruments(instrumentsPath)
	if err != nil {
		return fmt.Errorf("load instruments: %w", err)
	}

	for _, rec := range instRecords {
		buyProxy := sql.NullString{}
		if rec.BuyProxy != "" {
			buyProxy = sql.NullString{String: rec.BuyProxy, Valid: true}
		}

		_, err := queries.UpsertInstrument(ctx, dbgen.UpsertInstrumentParams{
			Symbol:           rec.Symbol,
			Name:             rec.Name,
			AssetClass:       rec.AssetClass,
			LiquidityRank:    int64(rec.LiquidityRank),
			AllowsFractional: rec.AllowsFractional,
			BuyProxy:         buyProxy,
		})
		if err != nil {
			return fmt.Errorf("upsert instrument %s: %w", rec.Symbol, err)
		}
	}

	// Load positions
	posRecords, err := LoadPositions(positionsPath)
	if err != nil {
		return fmt.Errorf("load positions: %w", err)
	}

	// Clear old positions
	if err := queries.DeleteAllPositions(ctx); err != nil {
		return fmt.Errorf("delete old positions: %w", err)
	}

	// Upsert accounts and positions
	accountIDs := make(map[string]int64)
	for _, rec := range posRecords {
		if _, exists := accountIDs[rec.Account]; !exists {
			acct, err := queries.UpsertAccount(ctx, rec.Account)
			if err != nil {
				return fmt.Errorf("upsert account %s: %w", rec.Account, err)
			}
			accountIDs[rec.Account] = acct.ID
		}

		// Ensure instrument exists (use classify.Resolve if not in map)
		inst, err := queries.GetInstrument(ctx, rec.Symbol)
		if err != nil {
			// Classify and insert
			resolved := classify.Resolve(rec.Symbol, rec.Symbol, nil)
			buyProxy := sql.NullString{}
			if resolved.BuyProxy != "" {
				buyProxy = sql.NullString{String: resolved.BuyProxy, Valid: true}
			}

			_, err := queries.UpsertInstrument(ctx, dbgen.UpsertInstrumentParams{
				Symbol:           resolved.Symbol,
				Name:             resolved.Name,
				AssetClass:       string(resolved.AssetClass),
				LiquidityRank:    int64(resolved.LiquidityRank),
				AllowsFractional: resolved.AllowsFractional,
				BuyProxy:         buyProxy,
			})
			if err != nil {
				return fmt.Errorf("upsert missing instrument %s: %w", rec.Symbol, err)
			}
		} else {
			_ = inst
		}

		_, err = queries.UpsertPosition(ctx, dbgen.UpsertPositionParams{
			AccountID:   accountIDs[rec.Account],
			Symbol:      rec.Symbol,
			Quantity:    rec.Quantity,
			Price:       rec.Price,
			MarketValue: rec.MarketValue,
		})
		if err != nil {
			return fmt.Errorf("upsert position %s/%s: %w", rec.Account, rec.Symbol, err)
		}
	}

	return nil
}
