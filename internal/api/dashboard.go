package api

import (
	"context"
	"database/sql"
	"encoding/csv"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"mime"
	"net/http"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/a-h/templ"
	"github.com/simonbalfe/freegent/internal/agent"
)

func handleJob(writer http.ResponseWriter, request *http.Request, store JobBackend) {
	input, rows, err := decodeJobRequest(writer, request)
	if err != nil {
		writeJSON(writer, http.StatusBadRequest, map[string]string{"error": err.Error()})
		return
	}
	id, err := store.Start(request.Context(), input, rows)
	if err != nil {
		writeJSON(writer, http.StatusInternalServerError, map[string]string{"error": err.Error()})
		return
	}
	writeJSON(writer, http.StatusAccepted, map[string]string{"jobId": id, "status": "queued"})
}

func handleJobsJSON(writer http.ResponseWriter, request *http.Request, store JobBackend) {
	jobs, err := store.List(50)
	if err != nil {
		writeJSON(writer, http.StatusInternalServerError, map[string]string{"error": err.Error()})
		return
	}
	writeJSON(writer, http.StatusOK, map[string]any{"jobs": jobs})
}

func handleJobJSON(writer http.ResponseWriter, request *http.Request, store JobBackend) {
	var job DashboardJob
	var err error
	if request.URL.Query().Get("summary") == "1" {
		job, err = store.GetSummary(request.PathValue("id"))
	} else if rawLimit := request.URL.Query().Get("limit"); rawLimit != "" {
		limit, parseError := strconv.Atoi(rawLimit)
		if parseError != nil || limit < 1 {
			writeJSON(writer, http.StatusBadRequest, map[string]string{"error": "limit must be a positive integer"})
			return
		}
		offset, parseError := strconv.Atoi(request.URL.Query().Get("offset"))
		if request.URL.Query().Get("offset") == "" {
			offset = 0
			parseError = nil
		}
		if parseError != nil || offset < 0 {
			writeJSON(writer, http.StatusBadRequest, map[string]string{"error": "offset must be a non-negative integer"})
			return
		}
		job, err = store.GetPage(request.PathValue("id"), limit, offset)
	} else {
		job, err = store.Get(request.PathValue("id"))
	}
	if errors.Is(err, sql.ErrNoRows) {
		writeJSON(writer, http.StatusNotFound, map[string]string{"error": "job not found"})
		return
	}
	if err != nil {
		writeJSON(writer, http.StatusInternalServerError, map[string]string{"error": err.Error()})
		return
	}
	writeJSON(writer, http.StatusOK, job)
}

func handleDashboard(writer http.ResponseWriter, request *http.Request, store JobBackend) {
	jobs, err := store.List(50)
	if err != nil {
		http.Error(writer, err.Error(), http.StatusInternalServerError)
		return
	}
	renderDashboard(writer, DashboardPage(jobs))
}

func handleDashboardJob(writer http.ResponseWriter, request *http.Request, store JobBackend) {
	request.Body = http.MaxBytesReader(writer, request.Body, 32<<20)
	input, rows, err := decodeDashboardForm(request)
	if err != nil {
		http.Error(writer, err.Error(), http.StatusBadRequest)
		return
	}
	id, err := store.Start(request.Context(), input, rows)
	if err != nil {
		http.Error(writer, err.Error(), http.StatusInternalServerError)
		return
	}
	http.Redirect(writer, request, "/dashboard/jobs/"+id, http.StatusSeeOther)
}

func handleDashboardJobPage(writer http.ResponseWriter, request *http.Request, store JobBackend) {
	job, err := store.GetPage(request.PathValue("id"), 200, 0)
	if errors.Is(err, sql.ErrNoRows) {
		http.NotFound(writer, request)
		return
	}
	if err != nil {
		http.Error(writer, err.Error(), http.StatusInternalServerError)
		return
	}
	renderDashboard(writer, DashboardJobPage(job))
}

func handleDashboardJobStatus(writer http.ResponseWriter, request *http.Request, store JobBackend) {
	job, err := store.GetPage(request.PathValue("id"), 200, 0)
	if errors.Is(err, sql.ErrNoRows) {
		http.NotFound(writer, request)
		return
	}
	if err != nil {
		http.Error(writer, err.Error(), http.StatusInternalServerError)
		return
	}
	renderDashboard(writer, DashboardJobStatus(job))
}

func decodeJobRequest(writer http.ResponseWriter, request *http.Request) (APIRequest, []map[string]any, error) {
	mediaType := ""
	if contentType := strings.TrimSpace(request.Header.Get("Content-Type")); contentType != "" {
		parsedMediaType, _, err := mime.ParseMediaType(contentType)
		if err != nil {
			return APIRequest{}, nil, fmt.Errorf("invalid content type: %w", err)
		}
		mediaType = parsedMediaType
	}
	if mediaType == "multipart/form-data" {
		request.Body = http.MaxBytesReader(writer, request.Body, 32<<20)
		return decodeMultipartJobForm(request)
	}
	if mediaType != "" && mediaType != "application/json" {
		return APIRequest{}, nil, errors.New("content type must be application/json or multipart/form-data")
	}
	defer request.Body.Close()
	decoder := json.NewDecoder(http.MaxBytesReader(writer, request.Body, 32<<20))
	decoder.DisallowUnknownFields()
	var input APIRequest
	if err := decoder.Decode(&input); err != nil {
		return APIRequest{}, nil, err
	}
	rows := input.Rows
	if len(rows) == 0 {
		rows = []map[string]any{input.Input}
	}
	if err := validateAPIRequest(input); err != nil {
		return APIRequest{}, nil, err
	}
	return input, rows, nil
}

func decodeDashboardForm(request *http.Request) (APIRequest, []map[string]any, error) {
	return decodeMultipartJobForm(request)
}

func decodeMultipartJobForm(request *http.Request) (APIRequest, []map[string]any, error) {
	if err := request.ParseMultipartForm(32 << 20); err != nil {
		return APIRequest{}, nil, err
	}
	rows := []map[string]any{}
	name := strings.TrimSpace(request.FormValue("name"))
	file, header, fileError := request.FormFile("csv")
	if fileError == nil {
		defer file.Close()
		parsed, err := parseCSVRows(file)
		if err != nil {
			return APIRequest{}, nil, err
		}
		rows = parsed
		if name == "" {
			name = filepath.Base(header.Filename)
		}
	} else if !errors.Is(fileError, http.ErrMissingFile) {
		return APIRequest{}, nil, fileError
	} else if raw := strings.TrimSpace(request.FormValue("rows")); raw != "" {
		if err := json.Unmarshal([]byte(raw), &rows); err != nil {
			return APIRequest{}, nil, fmt.Errorf("rows must be a JSON array: %w", err)
		}
	}
	if len(rows) == 0 {
		return APIRequest{}, nil, errors.New("upload a CSV or provide at least one JSON row")
	}
	input := APIRequest{
		Name:            name,
		Instructions:    request.FormValue("instructions"),
		Template:        request.FormValue("template"),
		Schema:          json.RawMessage(request.FormValue("schema")),
		Rows:            rows,
		Model:           request.FormValue("model"),
		MaxSteps:        5,
		MaxOutputTokens: 1500,
	}
	if err := validateAPIRequest(input); err != nil {
		return APIRequest{}, nil, err
	}
	return input, rows, nil
}

func parseCSVRows(reader io.Reader) ([]map[string]any, error) {
	records, err := csv.NewReader(reader).ReadAll()
	if err != nil {
		return nil, fmt.Errorf("invalid CSV: %w", err)
	}
	if len(records) < 2 {
		return nil, errors.New("CSV must contain a header and at least one data row")
	}
	headers := make([]string, len(records[0]))
	seen := map[string]bool{}
	for index, raw := range records[0] {
		header := strings.TrimSpace(raw)
		if header == "" {
			return nil, fmt.Errorf("CSV column %d has an empty header", index+1)
		}
		if seen[header] {
			return nil, fmt.Errorf("CSV header %q is duplicated", header)
		}
		seen[header] = true
		headers[index] = header
	}
	rows := make([]map[string]any, 0, len(records)-1)
	for _, record := range records[1:] {
		row := make(map[string]any, len(headers))
		for index, header := range headers {
			value := ""
			if index < len(record) {
				value = record[index]
			}
			row[header] = value
		}
		rows = append(rows, row)
	}
	return rows, nil
}

func validateAPIRequest(input APIRequest) error {
	if input.Instructions == "" || input.Template == "" || !json.Valid(input.Schema) {
		return fmt.Errorf("instructions, template, and a valid schema are required")
	}
	_, err := agent.CompileOutputSchema(input.Schema)
	return err
}

func renderDashboard(writer http.ResponseWriter, component templ.Component) {
	writer.Header().Set("Content-Type", "text/html; charset=utf-8")
	if err := component.Render(context.Background(), writer); err != nil {
		http.Error(writer, err.Error(), http.StatusInternalServerError)
	}
}

func dashboardInput(value map[string]any) string {
	return dashboardPretty(value)
}

type DashboardField struct {
	Key   string
	Value string
}

func dashboardFields(value map[string]any) []DashboardField {
	keys := make([]string, 0, len(value))
	for key := range value {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	fields := make([]DashboardField, 0, len(keys))
	for _, key := range keys {
		fields = append(fields, DashboardField{Key: key, Value: dashboardPretty(value[key])})
	}
	return fields
}

func dashboardPretty(value any) string {
	if text, ok := value.(string); ok {
		return text
	}
	data, _ := json.MarshalIndent(value, "", "  ")
	return string(data)
}

func dashboardTokens(job DashboardJob) TokenUsage {
	total := TokenUsage{}
	for _, row := range job.Rows {
		total.Add(row.Result.Tokens)
	}
	return total
}

func dashboardStepLabel(step Step) string {
	if step.Name == "" {
		return step.Kind
	}
	return strings.Title(step.Kind) + " · " + step.Name
}

func dashboardEventLabel(event DashboardEvent) string {
	if event.Row == 0 {
		return "run"
	}
	return fmt.Sprintf("row %d", event.Row)
}

func dashboardEventTime(value time.Time) string {
	return value.Local().Format("15:04:05")
}

func dashboardJobTime(value time.Time) string {
	return value.Local().Format("2 Jan 2006 15:04")
}

func dashboardJobLabel(job DashboardJob) string {
	if job.Name != "" {
		return job.Name
	}
	return job.ID
}

func dashboardEventMessage(message string) string {
	if strings.HasPrefix(message, "run start") {
		return "Agent started research"
	}
	if strings.Contains(message, "model requested tool=") {
		start := strings.Index(message, "tool=") + len("tool=")
		end := strings.Index(message[start:], " ")
		if end < 0 {
			end = len(message) - start
		}
		return "Agent selected " + message[start:start+end]
	}
	if strings.Contains(message, "tool=") && strings.Contains(message, "completed") {
		start := strings.Index(message, "tool=") + len("tool=")
		end := strings.Index(message[start:], " ")
		if end < 0 {
			end = len(message) - start
		}
		return message[start:start+end] + " completed"
	}
	if strings.Contains(message, "schema-valid final answer") {
		return "Answer passed schema validation"
	}
	if strings.HasPrefix(message, "finalizer start") {
		return "Agent is finalizing the answer"
	}
	return message
}

func dashboardActive(job DashboardJob) bool {
	return job.Status == "queued" || job.Status == "running"
}
