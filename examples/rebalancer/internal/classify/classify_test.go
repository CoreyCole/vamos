package classify

import "testing"

func TestResolve(t *testing.T) {
	tests := []struct {
		name      string
		symbol    string
		instName  string
		overrides map[string]Override
		wantClass AssetClass
		wantRank  int
	}{
		{
			name:      "override takes precedence",
			symbol:    "FNILX",
			instName:  "Fidelity ZERO Large Cap Index",
			overrides: map[string]Override{"FNILX": {Symbol: "FNILX", AssetClass: Cash, LiquidityRank: ptr(0)}},
			wantClass: Cash,
			wantRank:  0,
		},
		{
			name:      "money market heuristic",
			symbol:    "CUSTOM",
			instName:  "Custom Money Market Fund",
			overrides: nil,
			wantClass: Cash,
			wantRank:  1,
		},
		{
			name:      "seed instrument",
			symbol:    "FNILX",
			instName:  "Fidelity ZERO Large Cap Index",
			overrides: nil,
			wantClass: USEquity,
			wantRank:  3,
		},
		{
			name:      "unclassified fallback",
			symbol:    "UNKNOWN",
			instName:  "Unknown Ticker",
			overrides: nil,
			wantClass: Unclassified,
			wantRank:  4,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := Resolve(tt.symbol, tt.instName, tt.overrides)
			if got.AssetClass != tt.wantClass {
				t.Errorf("AssetClass = %v, want %v", got.AssetClass, tt.wantClass)
			}
			if got.LiquidityRank != tt.wantRank {
				t.Errorf("LiquidityRank = %v, want %v", got.LiquidityRank, tt.wantRank)
			}
		})
	}
}

func TestIsCashLike(t *testing.T) {
	tests := []struct {
		name string
		rank int
		want bool
	}{
		{"settled cash", 0, true},
		{"money market", 1, true},
		{"short duration", 2, false},
		{"broad index", 3, false},
		{"satellite", 4, false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			inst := Instrument{LiquidityRank: tt.rank}
			if got := inst.IsCashLike(); got != tt.want {
				t.Errorf("IsCashLike() = %v, want %v", got, tt.want)
			}
		})
	}
}

func ptr(i int) *int {
	return &i
}
