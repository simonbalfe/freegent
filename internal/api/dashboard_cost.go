package api

import (
	"fmt"
	"regexp"
	"sort"
	"strconv"
	"strings"
)

const (
	geminiFlashLiteInputPerMillion  = 0.25
	geminiFlashLiteOutputPerMillion = 1.50
	serperStarterQueryCostUSD       = 0.001
)

var apifyItemCountPattern = regexp.MustCompile(`,\s*(\d+)\s+items$`)

type DashboardCost struct {
	TotalUSD       float64
	ModelUSD       float64
	SerperUSD      float64
	ApifyUSD       float64
	InputTokens    int
	OutputTokens   int
	SerperQueries  int
	ApifyRuns      int
	ApifyItems     int
	Estimated      bool
	UnpricedModels []string
	UnpricedActors []string
}

type apifyEstimateRate struct {
	StartUSD   float64
	PerItemUSD float64
}

func dashboardJobCost(job DashboardJob) DashboardCost {
	cost := DashboardCost{}
	unpricedModels := map[string]bool{}
	unpricedActors := map[string]bool{}
	for _, row := range job.Rows {
		tokens := row.Result.Tokens
		cost.InputTokens += tokens.Input
		cost.OutputTokens += tokens.Output
		if tokens.CostKnown {
			cost.ModelUSD += tokens.CostUSD
		} else if tokens.Input > 0 || tokens.Output > 0 {
			inputRate, outputRate, ok := dashboardModelRates(row.Result.Model)
			if ok {
				cost.ModelUSD += float64(tokens.Input)*inputRate/1_000_000 + float64(tokens.Output)*outputRate/1_000_000
				cost.Estimated = true
			} else {
				unpricedModels[row.Result.Model] = true
			}
		}
		for _, evidence := range row.Result.Evidence {
			switch {
			case evidence.Provider == "serper":
				cost.SerperQueries++
				if evidence.CostKnown {
					cost.SerperUSD += evidence.CostUSD
				} else {
					cost.SerperUSD += serperStarterQueryCostUSD
					cost.Estimated = true
				}
			case strings.HasPrefix(evidence.Provider, "apify:"):
				cost.ApifyRuns++
				cost.ApifyItems += dashboardApifyItems(evidence)
				if evidence.CostKnown {
					cost.ApifyUSD += evidence.CostUSD
				} else if rate, ok := dashboardApifyRate(evidence.Provider); ok {
					cost.ApifyUSD += rate.StartUSD + float64(dashboardApifyItems(evidence))*rate.PerItemUSD
					cost.Estimated = true
				} else {
					unpricedActors[evidence.Provider] = true
				}
			}
		}
	}
	cost.TotalUSD = cost.ModelUSD + cost.SerperUSD + cost.ApifyUSD
	cost.UnpricedModels = sortedMapKeys(unpricedModels)
	cost.UnpricedActors = sortedMapKeys(unpricedActors)
	return cost
}

func dashboardModelRates(model string) (float64, float64, bool) {
	switch model {
	case "", "google/gemini-3.1-flash-lite", "google/gemini-3.1-flash-lite-20260507":
		return geminiFlashLiteInputPerMillion, geminiFlashLiteOutputPerMillion, true
	default:
		return 0, 0, false
	}
}

func dashboardApifyRate(provider string) (apifyEstimateRate, bool) {
	rates := map[string]apifyEstimateRate{
		"apify:harvestapi~linkedin-profile-scraper":   {PerItemUSD: 0.004},
		"apify:harvestapi~linkedin-profile-posts":     {StartUSD: 0.00005, PerItemUSD: 0.002},
		"apify:harvestapi~linkedin-post-reactions":    {PerItemUSD: 0.002},
		"apify:harvestapi~linkedin-company-employees": {PerItemUSD: 0.003},
		"apify:harvestapi~linkedin-company":           {StartUSD: 0.00005, PerItemUSD: 0.004},
		"apify:parseforge~crunchbase-scraper":         {StartUSD: 0.005, PerItemUSD: 0.03199},
	}
	rate, ok := rates[provider]
	return rate, ok
}

func dashboardApifyItems(evidence Evidence) int {
	for _, attempt := range evidence.Attempts {
		match := apifyItemCountPattern.FindStringSubmatch(attempt.Detail)
		if len(match) != 2 {
			continue
		}
		count, err := strconv.Atoi(match[1])
		if err == nil {
			return count
		}
	}
	return 0
}

func dashboardMoney(value float64) string {
	if value >= 1 {
		return fmt.Sprintf("$%.2f", value)
	}
	return fmt.Sprintf("$%.4f", value)
}

func dashboardCostLabel(cost DashboardCost) string {
	if cost.Estimated || len(cost.UnpricedModels) > 0 || len(cost.UnpricedActors) > 0 {
		return "Estimated cost"
	}
	return "Recorded cost"
}

func dashboardCostNote(cost DashboardCost) string {
	parts := []string{}
	if cost.Estimated {
		parts = append(parts, "Older usage is estimated from the pricing snapshot.")
	}
	if len(cost.UnpricedModels) > 0 {
		parts = append(parts, "Unpriced models: "+strings.Join(cost.UnpricedModels, ", ")+".")
	}
	if len(cost.UnpricedActors) > 0 {
		parts = append(parts, "Unpriced actors: "+strings.Join(cost.UnpricedActors, ", ")+".")
	}
	if len(parts) == 0 {
		return "Provider-reported charges recorded during the run."
	}
	return strings.Join(parts, " ")
}

func dashboardModelCostReason(cost DashboardCost) string {
	return fmt.Sprintf("%d input tokens and %d output tokens. New calls use OpenRouter-reported cost; older Gemini 3.1 Flash Lite calls use $0.25/M input and $1.50/M output.", cost.InputTokens, cost.OutputTokens)
}

func dashboardSerperCostReason(cost DashboardCost) string {
	return fmt.Sprintf("%d successful searches at the configured or Starter rate of $0.001 per query.", cost.SerperQueries)
}

func dashboardApifyCostReason(cost DashboardCost) string {
	return fmt.Sprintf("%d actor runs and %d returned items. New runs use Apify's reported run charge; older runs use the actor pricing snapshot.", cost.ApifyRuns, cost.ApifyItems)
}

func sortedMapKeys(values map[string]bool) []string {
	keys := make([]string, 0, len(values))
	for key := range values {
		if key != "" {
			keys = append(keys, key)
		}
	}
	sort.Strings(keys)
	return keys
}
