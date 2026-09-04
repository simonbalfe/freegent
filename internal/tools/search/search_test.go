package search

import (
	"context"
	"errors"
	"slices"
	"testing"
)

func TestRunSearchLadder(t *testing.T) {
	tests := []struct {
		name         string
		providers    []searchProvider
		wantText     string
		wantProvider string
		wantOutcomes []string
	}{
		{
			name: "falls through errors",
			providers: []searchProvider{
				{name: "first", key: "key", run: func(context.Context, string, string, int) ([]SearchHit, error) { return nil, errors.New("unavailable") }},
				{name: "second", key: "key", run: func(context.Context, string, string, int) ([]SearchHit, error) {
					return []SearchHit{{Title: "Example", URL: "https://example.com"}}, nil
				}},
			},
			wantText:     `[{"title":"Example","url":"https://example.com","content":""}]`,
			wantProvider: "second",
			wantOutcomes: []string{"error", "ok"},
		},
		{
			name: "keeps an empty result after later errors",
			providers: []searchProvider{
				{name: "first", key: "key", run: func(context.Context, string, string, int) ([]SearchHit, error) { return nil, nil }},
				{name: "second", key: "key", run: func(context.Context, string, string, int) ([]SearchHit, error) { return nil, errors.New("unavailable") }},
			},
			wantText:     "[]",
			wantOutcomes: []string{"empty", "error"},
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			result, err := runSearchLadder(context.Background(), test.providers, "example", 5)
			if err != nil {
				t.Fatal(err)
			}
			outcomes := make([]string, len(result.Attempts))
			for index, attempt := range result.Attempts {
				outcomes[index] = attempt.Outcome
			}
			if result.Text != test.wantText || result.Provider != test.wantProvider || !slices.Equal(outcomes, test.wantOutcomes) {
				t.Fatalf("result = %+v, outcomes = %v", result, outcomes)
			}
		})
	}
}
