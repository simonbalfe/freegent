package api

import (
	"math"
	"testing"
)

func TestDashboardJobCostCombinesHistoricalUsage(t *testing.T) {
	job := DashboardJob{Rows: []DashboardRow{{
		Result: APIResult{
			Model:  "google/gemini-3.1-flash-lite",
			Tokens: TokenUsage{Input: 1_000_000, Output: 1_000_000},
			Evidence: []Evidence{
				{Provider: "serper"},
				{
					Provider: "apify:harvestapi~linkedin-company",
					Attempts: []FetchAttempt{{Detail: "run one, dataset two, 2 items"}},
				},
			},
		},
	}}}

	cost := dashboardJobCost(job)
	if math.Abs(cost.ModelUSD-1.75) > 0.000001 {
		t.Fatalf("model cost = %f", cost.ModelUSD)
	}
	if math.Abs(cost.SerperUSD-0.001) > 0.000001 || cost.SerperQueries != 1 {
		t.Fatalf("serper cost = %+v", cost)
	}
	if math.Abs(cost.ApifyUSD-0.00805) > 0.000001 || cost.ApifyRuns != 1 || cost.ApifyItems != 2 {
		t.Fatalf("apify cost = %+v", cost)
	}
	if math.Abs(cost.TotalUSD-1.75905) > 0.000001 || !cost.Estimated {
		t.Fatalf("total cost = %+v", cost)
	}
}

func TestDashboardJobCostPrefersRecordedProviderCosts(t *testing.T) {
	job := DashboardJob{Rows: []DashboardRow{{
		Result: APIResult{
			Model:  "custom/model",
			Tokens: TokenUsage{Input: 100, Output: 20, CostUSD: 0.02, CostKnown: true},
			Evidence: []Evidence{
				{Provider: "serper", CostUSD: 0.0005, CostKnown: true},
				{Provider: "apify:custom~actor", CostUSD: 0.03, CostKnown: true},
			},
		},
	}}}

	cost := dashboardJobCost(job)
	if math.Abs(cost.TotalUSD-0.0505) > 0.000001 || cost.Estimated {
		t.Fatalf("recorded cost = %+v", cost)
	}
	if len(cost.UnpricedModels) != 0 || len(cost.UnpricedActors) != 0 {
		t.Fatalf("recorded providers should not require price tables: %+v", cost)
	}
}
