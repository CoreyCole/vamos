package rebalance

import (
	"testing"

	"example.com/vamos-rebalancer/internal/classify"
)

func TestRebalance_HouseholdLevel(t *testing.T) {
	instruments := map[string]classify.Instrument{
		"FNILX": {Symbol: "FNILX", AssetClass: classify.USEquity, LiquidityRank: 3, AllowsFractional: true},
		"SPAXX": {Symbol: "SPAXX", AssetClass: classify.Cash, LiquidityRank: 1, AllowsFractional: true},
	}

	positions := []Position{
		{AccountID: 1, AccountName: "IRA", Symbol: "FNILX", Quantity: 1000, Price: 20, MarketValue: 20000, AssetClass: classify.USEquity, LiquidityRank: 3, AllowsFractional: true},
		{AccountID: 2, AccountName: "Taxable", Symbol: "FNILX", Quantity: 2000, Price: 20, MarketValue: 40000, AssetClass: classify.USEquity, LiquidityRank: 3, AllowsFractional: true},
		{AccountID: 2, AccountName: "Taxable", Symbol: "SPAXX", Quantity: 10000, Price: 1, MarketValue: 10000, AssetClass: classify.Cash, LiquidityRank: 1, AllowsFractional: true},
	}

	targets := []Target{
		{AssetClass: classify.USEquity, Weight: 80},
		{AssetClass: classify.Cash, Weight: 20},
	}

	result := Rebalance(positions, targets, instruments)

	// Total value = 70,000, target 80% US = 56,000, current 60,000 = overweight by 4,000
	// Should generate sells
	if len(result.Trades) == 0 {
		t.Error("Expected trades to be generated")
	}

	foundSell := false
	for _, trade := range result.Trades {
		if trade.Side == "SELL" && trade.Symbol == "FNILX" {
			foundSell = true
		}
	}

	if !foundSell {
		t.Error("Expected SELL trade for overweight US Equity")
	}
}

func TestRebalance_CashIsolation(t *testing.T) {
	instruments := map[string]classify.Instrument{
		"FNILX": {Symbol: "FNILX", AssetClass: classify.USEquity, LiquidityRank: 3, AllowsFractional: true},
		"SPAXX": {Symbol: "SPAXX", AssetClass: classify.Cash, LiquidityRank: 1, AllowsFractional: true},
	}

	positions := []Position{
		{AccountID: 1, AccountName: "IRA", Symbol: "SPAXX", Quantity: 1000, Price: 1, MarketValue: 1000, AssetClass: classify.Cash, LiquidityRank: 1, AllowsFractional: true},
		{AccountID: 2, AccountName: "Taxable", Symbol: "FNILX", Quantity: 4000, Price: 20, MarketValue: 80000, AssetClass: classify.USEquity, LiquidityRank: 3, AllowsFractional: true},
	}

	targets := []Target{
		{AssetClass: classify.USEquity, Weight: 90},
		{AssetClass: classify.Cash, Weight: 10},
	}

	result := Rebalance(positions, targets, instruments)

	// IRA has cash but is isolated - cannot move to Taxable to buy US Equity
	// Should generate blocker about insufficient cash
	hasBlocker := false
	for _, b := range result.Blockers {
		if len(b.Message) > 0 {
			hasBlocker = true
		}
	}

	if !hasBlocker {
		t.Error("Expected blocker due to cash isolation")
	}
}

func TestRebalance_WholeShares(t *testing.T) {
	instruments := map[string]classify.Instrument{
		"VGK":   {Symbol: "VGK", AssetClass: classify.IntlEquity, LiquidityRank: 3, AllowsFractional: false},
		"SPAXX": {Symbol: "SPAXX", AssetClass: classify.Cash, LiquidityRank: 1, AllowsFractional: true},
	}

	positions := []Position{
		{AccountID: 1, AccountName: "IRA", Symbol: "SPAXX", Quantity: 50, Price: 1, MarketValue: 50, AssetClass: classify.Cash, LiquidityRank: 1, AllowsFractional: true},
	}

	targets := []Target{
		{AssetClass: classify.IntlEquity, Weight: 50},
		{AssetClass: classify.Cash, Weight: 50},
	}

	result := Rebalance(positions, targets, instruments)

	// VGK costs ~$50, only $25 should be invested (50% of $50), cannot buy even 1 share
	// With only $50 total and needing half in cash, only $25 available for intl equity
	// Since we can't buy fractional VGK and need ~$68.50 for 1 share, should generate blocker
	hasBlocker := false
	for _, b := range result.Blockers {
		if len(b.Message) > 0 {
			hasBlocker = true
		}
	}

	if !hasBlocker {
		t.Errorf("Expected blocker for insufficient funds to buy whole share, got %d blockers: %v", len(result.Blockers), result.Blockers)
	}
}
