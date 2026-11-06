package rebalance

import (
	"fmt"
	"math"
	"sort"

	"example.com/vamos-rebalancer/internal/classify"
)

// Position represents a holding in an account
type Position struct {
	AccountID    int64
	AccountName  string
	Symbol       string
	Quantity     float64
	Price        float64
	MarketValue  float64
	AssetClass   classify.AssetClass
	LiquidityRank int
	AllowsFractional bool
}

// Target represents desired asset allocation
type Target struct {
	AssetClass classify.AssetClass
	Weight     float64 // percentage 0-100
}

// Trade represents a proposed buy or sell
type Trade struct {
	AccountID      int64
	AccountName    string
	Symbol         string
	Side           string // "BUY" or "SELL"
	Shares         float64
	Dollars        float64
	RoundedShares  int64
	Reason         string
}

// Blocker represents a constraint that prevents perfect rebalancing
type Blocker struct {
	Message string
}

// Note provides informational messages about the rebalancing process
type Note struct {
	Message string
}

// Result contains the output of the rebalancing algorithm
type Result struct {
	Trades   []Trade
	Blockers []Blocker
	Notes    []Note
}

// Rebalance computes trades needed to align household positions with target allocation
func Rebalance(positions []Position, targets []Target, instruments map[string]classify.Instrument) Result {
	result := Result{}

	// Calculate total household value
	totalValue := 0.0
	for _, p := range positions {
		totalValue += p.MarketValue
	}

	if totalValue == 0 {
		result.Blockers = append(result.Blockers, Blocker{Message: "No positions to rebalance"})
		return result
	}

	// Build target map and validate
	targetMap := make(map[classify.AssetClass]float64)
	totalTargetWeight := 0.0
	for _, t := range targets {
		if t.AssetClass != classify.Unclassified {
			targetMap[t.AssetClass] = t.Weight
			totalTargetWeight += t.Weight
		}
	}

	if math.Abs(totalTargetWeight-100.0) > 0.01 {
		result.Blockers = append(result.Blockers, Blocker{
			Message: fmt.Sprintf("Target weights sum to %.2f%%, must equal 100%%", totalTargetWeight),
		})
		return result
	}

	// Calculate current allocation by class
	currentAllocation := make(map[classify.AssetClass]float64)
	for _, p := range positions {
		if p.AssetClass != classify.Unclassified {
			currentAllocation[p.AssetClass] += p.MarketValue
		}
	}

	// Calculate drift for each class
	classDrift := make(map[classify.AssetClass]float64)
	for class, weight := range targetMap {
		targetValue := totalValue * weight / 100.0
		currentValue := currentAllocation[class]
		classDrift[class] = currentValue - targetValue
	}

	// Group positions by account
	accountPositions := make(map[int64][]Position)
	for _, p := range positions {
		accountPositions[p.AccountID] = append(accountPositions[p.AccountID], p)
	}

	// Process each overweight class - sell positions
	for class, drift := range classDrift {
		if drift <= 1.0 { // Skip if not meaningfully overweight
			continue
		}

		// Find all positions in this overweight class, sorted by liquidity (lowest rank first = least liquid)
		var classPositions []Position
		for _, p := range positions {
			if p.AssetClass == class {
				classPositions = append(classPositions, p)
			}
		}

		sort.Slice(classPositions, func(i, j int) bool {
			if classPositions[i].LiquidityRank != classPositions[j].LiquidityRank {
				return classPositions[i].LiquidityRank < classPositions[j].LiquidityRank
			}
			return classPositions[i].MarketValue > classPositions[j].MarketValue
		})

		remainingToSell := drift
		for _, p := range classPositions {
			if remainingToSell <= 1.0 {
				break
			}

			// Check account cash availability first
			accountCash := getAccountCash(accountPositions[p.AccountID])
			if accountCash > 10.0 {
				// Prefer spending cash before selling
				continue
			}

			sellValue := math.Min(p.MarketValue*0.9, remainingToSell) // Don't sell entire position unless needed
			sellShares := sellValue / p.Price
			roundedShares := int64(math.Floor(sellShares))

			if roundedShares > 0 {
				actualDollars := float64(roundedShares) * p.Price
				result.Trades = append(result.Trades, Trade{
					AccountID:     p.AccountID,
					AccountName:   p.AccountName,
					Symbol:        p.Symbol,
					Side:          "SELL",
					Shares:        sellShares,
					Dollars:       actualDollars,
					RoundedShares: roundedShares,
					Reason:        fmt.Sprintf("Overweight %s", class),
				})
				remainingToSell -= actualDollars
			}
		}
	}

	// Process underweight classes - buy positions
	for class, drift := range classDrift {
		if drift >= -1.0 { // Skip if not meaningfully underweight
			continue
		}

		needToBuy := -drift

		// Find accounts with available cash
		for accountID, accountPos := range accountPositions {
			accountCash := getAccountCash(accountPos)
			if accountCash < 10.0 {
				continue
			}

			// Determine buy symbol
			buySymbol := findBuyProxy(accountPos, class, instruments)
			if buySymbol == "" {
				result.Blockers = append(result.Blockers, Blocker{
					Message: fmt.Sprintf("No buy proxy found for %s in account %s", class, accountPos[0].AccountName),
				})
				continue
			}

			inst := instruments[buySymbol]
			buyValue := math.Min(accountCash, needToBuy)
			
			// Rough price estimation (use $50 default if unknown)
			price := 50.0
			for _, p := range positions {
				if p.Symbol == buySymbol {
					price = p.Price
					break
				}
			}

			buyShares := buyValue / price
			roundedShares := int64(math.Floor(buyShares))

			if !inst.AllowsFractional && roundedShares < 1 {
				result.Blockers = append(result.Blockers, Blocker{
					Message: fmt.Sprintf("Cannot buy fractional shares of %s, need $%.2f more", buySymbol, price-buyValue),
				})
				continue
			}

			if inst.AllowsFractional {
				roundedShares = int64(math.Round(buyShares))
			}

			if roundedShares > 0 || (inst.AllowsFractional && buyShares > 0.001) {
				actualDollars := float64(roundedShares) * price
				if inst.AllowsFractional {
					actualDollars = buyShares * price
				}

				result.Trades = append(result.Trades, Trade{
					AccountID:     accountID,
					AccountName:   accountPos[0].AccountName,
					Symbol:        buySymbol,
					Side:          "BUY",
					Shares:        buyShares,
					Dollars:       actualDollars,
					RoundedShares: roundedShares,
					Reason:        fmt.Sprintf("Underweight %s", class),
				})
				needToBuy -= actualDollars
			}
		}

		if needToBuy > 10.0 {
			result.Blockers = append(result.Blockers, Blocker{
				Message: fmt.Sprintf("Insufficient cash across accounts to fully rebalance %s (need $%.2f more)", class, needToBuy),
			})
		}
	}

	return result
}

// getAccountCash sums up cash-like positions in an account
func getAccountCash(positions []Position) float64 {
	cash := 0.0
	for _, p := range positions {
		if p.LiquidityRank <= 1 { // Cash-like
			cash += p.MarketValue
		}
	}
	return cash
}

// findBuyProxy determines which symbol to buy for a given asset class in an account
func findBuyProxy(accountPositions []Position, class classify.AssetClass, instruments map[string]classify.Instrument) string {
	// First, check if account already holds something in this class
	for _, p := range accountPositions {
		if p.AssetClass == class && p.MarketValue > 0 {
			return p.Symbol
		}
	}

	// Use default buy proxies
	proxyMap := map[classify.AssetClass]string{
		classify.USEquity:       "FNILX",
		classify.IntlEquity:     "FZILX",
		classify.ThematicEquity: "FNILX", // Avoid NUKZ/SHLD unless already held
		classify.Gold:           "IAU",
		classify.ShortDuration:  "BIL",
		classify.Cash:           "SPAXX",
	}

	return proxyMap[class]
}
