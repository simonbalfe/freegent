package claygent

import (
	"context"
	"fmt"
	"sync/atomic"
	"testing"
	"time"
)

func TestRunBatchProcessesOneHundredRowsWithBoundedConcurrency(t *testing.T) {
	rows := make([]map[string]any, 100)
	for index := range rows {
		rows[index] = map[string]any{"row": index}
	}

	var active int32
	var maximum int32
	started := time.Now()
	results := runBatchWith(context.Background(), APIRequest{Concurrency: 5}, rows, func(ctx context.Context, request APIRequest, row map[string]any) APIResult {
		current := atomic.AddInt32(&active, 1)
		for {
			observed := atomic.LoadInt32(&maximum)
			if current <= observed || atomic.CompareAndSwapInt32(&maximum, observed, current) {
				break
			}
		}
		time.Sleep(10 * time.Millisecond)
		atomic.AddInt32(&active, -1)
		return APIResult{
			RunID:  fmt.Sprintf("row-%d", row["row"]),
			Result: map[string]any{"row": row["row"]},
		}
	})

	if len(results) != 100 {
		t.Fatalf("expected 100 results, got %d", len(results))
	}
	if maximum != 5 {
		t.Fatalf("expected five concurrent rows, got %d", maximum)
	}
	for index, result := range results {
		expected := fmt.Sprintf("row-%d", index)
		if result.RunID != expected {
			t.Fatalf("result %d lost input order: expected %q, got %q", index, expected, result.RunID)
		}
	}
	t.Logf("processed 100 rows with maximum concurrency %d in %s", maximum, time.Since(started).Round(time.Millisecond))
}
