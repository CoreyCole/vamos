package web

import (
	"fmt"
	"strconv"
)

type PageData struct {
	Targets        TargetsView
	TotalValue     float64
	ClassBreakdown []ClassBreakdownView
	Positions      []PositionView
	Trades         []TradeView
	Blockers       []string
	ListShows      string
	SortBy         string
}

type TargetsView struct {
	USEquity       float64
	IntlEquity     float64
	ThematicEquity float64
	Gold           float64
	ShortDuration  float64
	Cash           float64
}

type ClassBreakdownView struct {
	Class      string
	CurrentPct float64
	TargetPct  float64
}

type PositionView struct {
	Account     string
	Symbol      string
	Name        string
	AssetClass  string
	Quantity    float64
	Price       float64
	MarketValue float64
}

type TradeView struct {
	Account       string
	Side          string
	Symbol        string
	RoundedShares int64
	Dollars       float64
	Reason        string
}

func formatFloat(f float64) string {
	return strconv.FormatFloat(f, 'f', 2, 64)
}

func formatMoney(f float64) string {
	return fmt.Sprintf("%.2f", f)
}

func formatInt(i int64) string {
	return strconv.FormatInt(i, 10)
}
