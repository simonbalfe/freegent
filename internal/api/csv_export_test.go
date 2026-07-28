package api

import (
	"bytes"
	"encoding/csv"
	"encoding/json"
	"testing"
)

func TestCSVExportAddsOneAnswerColumn(t *testing.T) {
	job := DashboardJob{
		ID:   "job-1",
		Name: "prospects.csv",
		Request: APIRequest{
			Schema: json.RawMessage(`{"company":"string","fit":"high|medium|low","details":{"type":"object"}}`),
		},
		Rows: []DashboardRow{
			{
				Index:  0,
				Input:  map[string]any{"company": "Figma", "domain": "figma.com"},
				Status: "completed",
				Result: APIResult{
					Result: map[string]any{
						"company": "Figma Inc.",
						"fit":     "high",
						"details": map[string]any{"segment": "design"},
					},
					Sources:    []string{"https://figma.com/"},
					Tokens:     TokenUsage{Input: 100, Output: 20},
					DurationMS: 750,
					Model:      "test-model",
				},
			},
		},
	}
	inputColumns, answerColumn := csvExportColumns(job)
	var output bytes.Buffer
	writer := csv.NewWriter(&output)
	if err := writer.Write(csvExportHeaders(inputColumns, answerColumn)); err != nil {
		t.Fatal(err)
	}
	if err := writer.Write(csvExportRecord(job.Rows[0], inputColumns)); err != nil {
		t.Fatal(err)
	}
	writer.Flush()
	records, err := csv.NewReader(&output).ReadAll()
	if err != nil {
		t.Fatal(err)
	}
	headers := records[0]
	values := records[1]
	index := map[string]int{}
	for position, header := range headers {
		index[header] = position
	}
	if values[index["company"]] != "Figma" {
		t.Fatalf("expected original company column, got %#v", records)
	}
	if len(headers) != 3 || headers[2] != "answer" {
		t.Fatalf("expected exactly one appended answer column, got %#v", headers)
	}
	if values[index["answer"]] != `{"company":"Figma Inc.","details":{"segment":"design"},"fit":"high"}` {
		t.Fatalf("unexpected answer value: %#v", records)
	}
	if csvExportFilename(job) != "prospects-results.csv" {
		t.Fatalf("unexpected filename %q", csvExportFilename(job))
	}
}
