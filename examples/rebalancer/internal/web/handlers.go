package web

import (
	"context"
	"database/sql"
	"log"
	"net/http"
	"path/filepath"
	"strconv"
	"sync"

	"example.com/vamos-rebalancer/internal/classify"
	"example.com/vamos-rebalancer/internal/db/dbgen"
	"example.com/vamos-rebalancer/internal/ingest"
	"example.com/vamos-rebalancer/internal/rebalance"
	"github.com/a-h/templ"
	"github.com/labstack/echo/v4"
	"github.com/labstack/echo/v4/middleware"
)

type App struct {
	db        *sql.DB
	queries   *dbgen.Queries
	filesRoot string
	notifier  *notifier
}

type Config struct {
	DB        *sql.DB
	FilesRoot string
}

func New(cfg Config) (*App, error) {
	return &App{
		db:        cfg.DB,
		queries:   dbgen.New(cfg.DB),
		filesRoot: cfg.FilesRoot,
		notifier:  newNotifier(),
	}, nil
}

func (a *App) Close() error {
	if a.db != nil {
		return a.db.Close()
	}
	return nil
}

func (a *App) Routes() *echo.Echo {
	e := echo.New()
	e.HideBanner = true
	e.Use(middleware.Logger())
	e.Use(middleware.Recover())

	e.GET("/healthz", a.handleHealthz)
	e.GET("/", a.handleIndex)
	e.GET("/events", a.handleEvents)
	e.POST("/ingest", a.handleIngest)
	e.POST("/positions", a.handlePositions)
	e.POST("/rebalance", a.handleRebalance)

	return e
}

func (a *App) handleHealthz(c echo.Context) error {
	return c.String(http.StatusOK, "ok")
}

func (a *App) handleIndex(c echo.Context) error {
	ctx := c.Request().Context()
	data, err := a.buildPageData(ctx, "value", "value", nil)
	if err != nil {
		return err
	}
	return render(c, Layout(data))
}

func (a *App) handleEvents(c echo.Context) error {
	c.Response().Header().Set("Content-Type", "text/event-stream")
	c.Response().Header().Set("Cache-Control", "no-cache")
	c.Response().Header().Set("Connection", "keep-alive")

	ch := a.notifier.subscribe()
	defer a.notifier.unsubscribe(ch)

	for {
		select {
		case <-c.Request().Context().Done():
			return nil
		case event := <-ch:
			_ = event
			// Could send updates here
		}
	}
}

func (a *App) handleIngest(c echo.Context) error {
	ctx := c.Request().Context()

	positionsPath := filepath.Join(a.filesRoot, "..", "data", "acme_positions.csv")
	instrumentsPath := filepath.Join(a.filesRoot, "..", "data", "asset_map.csv")

	if err := ingest.ImportToDatabase(ctx, a.queries, positionsPath, instrumentsPath); err != nil {
		log.Printf("ingest error: %v", err)
		return c.String(http.StatusInternalServerError, "Import failed")
	}

	// Initialize default targets
	targets := []struct {
		class  string
		weight float64
	}{
		{"US Equity", 40},
		{"Intl Equity", 20},
		{"Thematic Equity", 5},
		{"Gold", 5},
		{"Short Duration", 10},
		{"Cash", 20},
	}

	for _, t := range targets {
		_, err := a.queries.UpsertTarget(ctx, dbgen.UpsertTargetParams{
			AssetClass: t.class,
			Weight:     t.weight,
		})
		if err != nil {
			log.Printf("upsert target error: %v", err)
		}
	}

	data, err := a.buildPageData(ctx, "value", "value", nil)
	if err != nil {
		return err
	}

	c.Response().Header().Set("Content-Type", "text/vnd.datastar.fragment+html")
	if err := render(c, Content(data)); err != nil {
		return err
	}

	return nil
}

func (a *App) handlePositions(c echo.Context) error {
	ctx := c.Request().Context()

	listShows := c.FormValue("listShows")
	if listShows == "" {
		listShows = "value"
	}

	sortBy := c.FormValue("sortBy")
	if sortBy == "" {
		sortBy = "value"
	}

	data, err := a.buildPageData(ctx, listShows, sortBy, nil)
	if err != nil {
		return err
	}

	c.Response().Header().Set("Content-Type", "text/vnd.datastar.fragment+html")
	return render(c, Positions(data))
}

func (a *App) handleRebalance(c echo.Context) error {
	ctx := c.Request().Context()

	// Parse target weights from form
	targetsMap := map[string]float64{
		"US Equity":       parseFloat(c.FormValue("wUs"), 40),
		"Intl Equity":     parseFloat(c.FormValue("wIntl"), 20),
		"Thematic Equity": parseFloat(c.FormValue("wThematic"), 5),
		"Gold":            parseFloat(c.FormValue("wGold"), 5),
		"Short Duration":  parseFloat(c.FormValue("wShort"), 10),
		"Cash":            parseFloat(c.FormValue("wCash"), 20),
	}

	// Update targets in DB
	for class, weight := range targetsMap {
		_, err := a.queries.UpsertTarget(ctx, dbgen.UpsertTargetParams{
			AssetClass: class,
			Weight:     weight,
		})
		if err != nil {
			log.Printf("upsert target error: %v", err)
		}
	}

	data, err := a.buildPageData(ctx, "value", "value", targetsMap)
	if err != nil {
		return err
	}

	c.Response().Header().Set("Content-Type", "text/vnd.datastar.fragment+html")
	if err := render(c, Targets(data)); err != nil {
		return err
	}
	if err := render(c, Summary(data)); err != nil {
		return err
	}
	if err := render(c, Trades(data)); err != nil {
		return err
	}

	return nil
}

func (a *App) buildPageData(ctx context.Context, listShows, sortBy string, targetsOverride map[string]float64) (PageData, error) {
	data := PageData{
		ListShows: listShows,
		SortBy:    sortBy,
	}

	// Get targets
	dbTargets, err := a.queries.ListTargets(ctx)
	if err != nil && err != sql.ErrNoRows {
		return data, err
	}

	targetMap := make(map[string]float64)
	for _, t := range dbTargets {
		targetMap[t.AssetClass] = t.Weight
	}

	// Apply override if provided
	if targetsOverride != nil {
		for k, v := range targetsOverride {
			targetMap[k] = v
		}
	}

	data.Targets = TargetsView{
		USEquity:       targetMap["US Equity"],
		IntlEquity:     targetMap["Intl Equity"],
		ThematicEquity: targetMap["Thematic Equity"],
		Gold:           targetMap["Gold"],
		ShortDuration:  targetMap["Short Duration"],
		Cash:           targetMap["Cash"],
	}

	// Get positions - convert all to common structure
	var positions []struct {
		ID               int64
		AccountID        int64
		Symbol           string
		Quantity         float64
		Price            float64
		MarketValue      float64
		CreatedAt        string
		AccountName      string
		InstrumentName   string
		AssetClass       string
		LiquidityRank    int64
		AllowsFractional bool
	}

	switch sortBy {
	case "account":
		rows, err := a.queries.ListPositionsSortedByAccount(ctx)
		if err != nil && err != sql.ErrNoRows {
			return data, err
		}
		for _, r := range rows {
			positions = append(positions, struct {
				ID               int64
				AccountID        int64
				Symbol           string
				Quantity         float64
				Price            float64
				MarketValue      float64
				CreatedAt        string
				AccountName      string
				InstrumentName   string
				AssetClass       string
				LiquidityRank    int64
				AllowsFractional bool
			}{r.ID, r.AccountID, r.Symbol, r.Quantity, r.Price, r.MarketValue, r.CreatedAt, r.AccountName, r.InstrumentName, r.AssetClass, r.LiquidityRank, r.AllowsFractional})
		}
	case "class":
		rows, err := a.queries.ListPositionsSortedByClass(ctx)
		if err != nil && err != sql.ErrNoRows {
			return data, err
		}
		for _, r := range rows {
			positions = append(positions, struct {
				ID               int64
				AccountID        int64
				Symbol           string
				Quantity         float64
				Price            float64
				MarketValue      float64
				CreatedAt        string
				AccountName      string
				InstrumentName   string
				AssetClass       string
				LiquidityRank    int64
				AllowsFractional bool
			}{r.ID, r.AccountID, r.Symbol, r.Quantity, r.Price, r.MarketValue, r.CreatedAt, r.AccountName, r.InstrumentName, r.AssetClass, r.LiquidityRank, r.AllowsFractional})
		}
	case "symbol":
		rows, err := a.queries.ListPositionsSortedBySymbol(ctx)
		if err != nil && err != sql.ErrNoRows {
			return data, err
		}
		for _, r := range rows {
			positions = append(positions, struct {
				ID               int64
				AccountID        int64
				Symbol           string
				Quantity         float64
				Price            float64
				MarketValue      float64
				CreatedAt        string
				AccountName      string
				InstrumentName   string
				AssetClass       string
				LiquidityRank    int64
				AllowsFractional bool
			}{r.ID, r.AccountID, r.Symbol, r.Quantity, r.Price, r.MarketValue, r.CreatedAt, r.AccountName, r.InstrumentName, r.AssetClass, r.LiquidityRank, r.AllowsFractional})
		}
	default:
		rows, err := a.queries.ListPositions(ctx)
		if err != nil && err != sql.ErrNoRows {
			return data, err
		}
		for _, r := range rows {
			positions = append(positions, struct {
				ID               int64
				AccountID        int64
				Symbol           string
				Quantity         float64
				Price            float64
				MarketValue      float64
				CreatedAt        string
				AccountName      string
				InstrumentName   string
				AssetClass       string
				LiquidityRank    int64
				AllowsFractional bool
			}{r.ID, r.AccountID, r.Symbol, r.Quantity, r.Price, r.MarketValue, r.CreatedAt, r.AccountName, r.InstrumentName, r.AssetClass, r.LiquidityRank, r.AllowsFractional})
		}
	}

	for _, p := range positions {
		data.Positions = append(data.Positions, PositionView{
			Account:     p.AccountName,
			Symbol:      p.Symbol,
			Name:        p.InstrumentName,
			AssetClass:  p.AssetClass,
			Quantity:    p.Quantity,
			Price:       p.Price,
			MarketValue: p.MarketValue,
		})
	}

	// Get total value
	totalRaw, err := a.queries.GetTotalMarketValue(ctx)
	if err != nil {
		return data, err
	}
	data.TotalValue = toFloat64(totalRaw)

	// Get class breakdown
	breakdown, err := a.queries.GetClassBreakdown(ctx)
	if err != nil && err != sql.ErrNoRows {
		return data, err
	}

	for _, cb := range breakdown {
		totalValue := toFloat64(cb.TotalValue)
		if totalValue > 0 {
			pct := 0.0
			if data.TotalValue > 0 {
				pct = (totalValue / data.TotalValue) * 100
			}
			data.ClassBreakdown = append(data.ClassBreakdown, ClassBreakdownView{
				Class:      cb.AssetClass,
				CurrentPct: pct,
				TargetPct:  targetMap[cb.AssetClass],
			})
		}
	}

	// Run rebalance
	if len(positions) > 0 && len(dbTargets) > 0 {
		rebalancePositions := make([]rebalance.Position, len(positions))
		for i, p := range positions {
			rebalancePositions[i] = rebalance.Position{
				AccountID:        p.AccountID,
				AccountName:      p.AccountName,
				Symbol:           p.Symbol,
				Quantity:         p.Quantity,
				Price:            p.Price,
				MarketValue:      p.MarketValue,
				AssetClass:       classify.AssetClass(p.AssetClass),
				LiquidityRank:    int(p.LiquidityRank),
				AllowsFractional: p.AllowsFractional,
			}
		}

		targets := []rebalance.Target{
			{AssetClass: classify.USEquity, Weight: data.Targets.USEquity},
			{AssetClass: classify.IntlEquity, Weight: data.Targets.IntlEquity},
			{AssetClass: classify.ThematicEquity, Weight: data.Targets.ThematicEquity},
			{AssetClass: classify.Gold, Weight: data.Targets.Gold},
			{AssetClass: classify.ShortDuration, Weight: data.Targets.ShortDuration},
			{AssetClass: classify.Cash, Weight: data.Targets.Cash},
		}

		// Build instrument map
		instruments, err := a.queries.ListInstruments(ctx)
		if err != nil {
			return data, err
		}

		instMap := make(map[string]classify.Instrument)
		for _, inst := range instruments {
			buyProxy := ""
			if inst.BuyProxy.Valid {
				buyProxy = inst.BuyProxy.String
			}
			instMap[inst.Symbol] = classify.Instrument{
				Symbol:           inst.Symbol,
				Name:             inst.Name,
				AssetClass:       classify.AssetClass(inst.AssetClass),
				LiquidityRank:    int(inst.LiquidityRank),
				AllowsFractional: inst.AllowsFractional,
				BuyProxy:         buyProxy,
			}
		}

		result := rebalance.Rebalance(rebalancePositions, targets, instMap)

		// Clear old trades
		if err := a.queries.DeleteAllProposedTrades(ctx); err != nil {
			log.Printf("delete trades error: %v", err)
		}

		// Save new trades
		for _, t := range result.Trades {
			_, err := a.queries.InsertProposedTrade(ctx, dbgen.InsertProposedTradeParams{
				AccountID:     t.AccountID,
				Symbol:        t.Symbol,
				Side:          t.Side,
				Shares:        t.Shares,
				Dollars:       t.Dollars,
				RoundedShares: t.RoundedShares,
				Reason:        t.Reason,
			})
			if err != nil {
				log.Printf("insert trade error: %v", err)
			}

			data.Trades = append(data.Trades, TradeView{
				Account:       t.AccountName,
				Side:          t.Side,
				Symbol:        t.Symbol,
				RoundedShares: t.RoundedShares,
				Dollars:       t.Dollars,
				Reason:        t.Reason,
			})
		}

		for _, b := range result.Blockers {
			data.Blockers = append(data.Blockers, b.Message)
		}
	}

	return data, nil
}

func toFloat64(v interface{}) float64 {
	switch val := v.(type) {
	case float64:
		return val
	case int64:
		return float64(val)
	case int:
		return float64(val)
	default:
		return 0
	}
}

func parseFloat(s string, defaultVal float64) float64 {
	if s == "" {
		return defaultVal
	}
	v, err := strconv.ParseFloat(s, 64)
	if err != nil {
		return defaultVal
	}
	return v
}

func render(c echo.Context, component templ.Component) error {
	return component.Render(c.Request().Context(), c.Response().Writer)
}

type notifier struct {
	mu   sync.Mutex
	subs map[chan notifierEvent]struct{}
}

type notifierEvent struct {
	Type string
}

func newNotifier() *notifier {
	return &notifier{subs: make(map[chan notifierEvent]struct{})}
}

func (n *notifier) subscribe() chan notifierEvent {
	ch := make(chan notifierEvent, 1)
	n.mu.Lock()
	n.subs[ch] = struct{}{}
	n.mu.Unlock()
	return ch
}

func (n *notifier) unsubscribe(ch chan notifierEvent) {
	n.mu.Lock()
	delete(n.subs, ch)
	close(ch)
	n.mu.Unlock()
}
