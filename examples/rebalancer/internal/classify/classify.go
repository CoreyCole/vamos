package classify

import (
	"strings"
)

// AssetClass represents one of the investment categories
type AssetClass string

const (
	USEquity        AssetClass = "US Equity"
	IntlEquity      AssetClass = "Intl Equity"
	ThematicEquity  AssetClass = "Thematic Equity"
	Gold            AssetClass = "Gold"
	ShortDuration   AssetClass = "Short Duration"
	Cash            AssetClass = "Cash"
	Unclassified    AssetClass = "Unclassified"
)

// Instrument represents classification data for a security
type Instrument struct {
	Symbol           string
	Name             string
	AssetClass       AssetClass
	LiquidityRank    int
	AllowsFractional bool
	BuyProxy         string
}

// InstrumentMap maps symbols to their classification
type InstrumentMap map[string]Instrument

// Override represents a user-provided classification override
type Override struct {
	Symbol        string
	AssetClass    AssetClass
	LiquidityRank *int
}

var seedInstruments = InstrumentMap{
	"FNILX": {Symbol: "FNILX", Name: "Fidelity ZERO Large Cap Index", AssetClass: USEquity, LiquidityRank: 3, AllowsFractional: true},
	"FZILX": {Symbol: "FZILX", Name: "Fidelity ZERO International Index", AssetClass: IntlEquity, LiquidityRank: 3, AllowsFractional: true},
	"VGK":   {Symbol: "VGK", Name: "Vanguard FTSE Europe ETF", AssetClass: IntlEquity, LiquidityRank: 3, AllowsFractional: false},
	"IAU":   {Symbol: "IAU", Name: "iShares Gold Trust", AssetClass: Gold, LiquidityRank: 3, AllowsFractional: false},
	"NUKZ":  {Symbol: "NUKZ", Name: "NukeEnergyFund ETF", AssetClass: ThematicEquity, LiquidityRank: 4, AllowsFractional: false},
	"SHLD":  {Symbol: "SHLD", Name: "ShieldAI Growth Fund", AssetClass: ThematicEquity, LiquidityRank: 4, AllowsFractional: false},
	"BIL":   {Symbol: "BIL", Name: "SPDR Bloomberg 1-3 Month T-Bill", AssetClass: ShortDuration, LiquidityRank: 2, AllowsFractional: false},
	"SPAXX": {Symbol: "SPAXX", Name: "Fidelity Government Money Market", AssetClass: Cash, LiquidityRank: 1, AllowsFractional: true},
	"FZFXX": {Symbol: "FZFXX", Name: "Fidelity Treasury Money Market", AssetClass: Cash, LiquidityRank: 1, AllowsFractional: true},
	"FRGXX": {Symbol: "FRGXX", Name: "Fidelity Government Cash Reserves", AssetClass: Cash, LiquidityRank: 1, AllowsFractional: true},
	"FCASH": {Symbol: "FCASH", Name: "Fidelity Cash Account", AssetClass: Cash, LiquidityRank: 0, AllowsFractional: true},
}

// Resolve determines the asset class and liquidity for a symbol using the classification hierarchy
func Resolve(symbol, name string, overrides map[string]Override) Instrument {
	// 1) Check user overrides first
	if override, exists := overrides[symbol]; exists {
		inst := Instrument{
			Symbol:           symbol,
			Name:             name,
			AssetClass:       override.AssetClass,
			LiquidityRank:    4,
			AllowsFractional: false,
		}
		if override.LiquidityRank != nil {
			inst.LiquidityRank = *override.LiquidityRank
		}
		return inst
	}

	// 2) Check export heuristics
	upperName := strings.ToUpper(name)
	if strings.Contains(upperName, "MONEY MARKET") {
		return Instrument{Symbol: symbol, Name: name, AssetClass: Cash, LiquidityRank: 1, AllowsFractional: true}
	}
	if strings.HasPrefix(symbol, "**") {
		return Instrument{Symbol: symbol, Name: name, AssetClass: Cash, LiquidityRank: 1, AllowsFractional: true}
	}

	// 3) Check seed instrument map
	if seed, exists := seedInstruments[symbol]; exists {
		return seed
	}

	// 4) Vendor lookup would go here (skipped for now)

	// 5) Default to Unclassified
	return Instrument{
		Symbol:           symbol,
		Name:             name,
		AssetClass:       Unclassified,
		LiquidityRank:    4,
		AllowsFractional: false,
	}
}

// IsCashLike returns true if the liquidity rank indicates this is settled cash or money market
func (i Instrument) IsCashLike() bool {
	return i.LiquidityRank <= 1
}
