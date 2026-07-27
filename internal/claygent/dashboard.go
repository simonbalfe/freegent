package claygent

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"

	"github.com/a-h/templ"
)

func (s *JobStore) List() []DashboardJob {
	s.mu.RLock()
	jobs := make([]DashboardJob, 0, len(s.jobs))
	for _, job := range s.jobs {
		copy := *job
		copy.Rows = append([]DashboardRow(nil), job.Rows...)
		jobs = append(jobs, copy)
	}
	s.mu.RUnlock()
	return jobs
}

func handleJob(writer http.ResponseWriter, request *http.Request, store *JobStore) {
	input, rows, err := decodeJobRequest(writer, request)
	if err != nil {
		writeJSON(writer, http.StatusBadRequest, map[string]string{"error": err.Error()})
		return
	}
	id := store.Start(input, rows)
	writeJSON(writer, http.StatusAccepted, map[string]string{"jobId": id, "status": "queued"})
}

func handleJobJSON(writer http.ResponseWriter, request *http.Request, store *JobStore) {
	job, ok := store.Get(request.PathValue("id"))
	if !ok {
		writeJSON(writer, http.StatusNotFound, map[string]string{"error": "job not found"})
		return
	}
	writeJSON(writer, http.StatusOK, job)
}

func handleDashboard(writer http.ResponseWriter, request *http.Request, store *JobStore) {
	renderDashboard(writer, DashboardPage(store.List()))
}

func handleDashboardJob(writer http.ResponseWriter, request *http.Request, store *JobStore) {
	input, rows, err := decodeDashboardForm(request)
	if err != nil {
		http.Error(writer, err.Error(), http.StatusBadRequest)
		return
	}
	id := store.Start(input, rows)
	http.Redirect(writer, request, "/dashboard/jobs/"+id, http.StatusSeeOther)
}

func handleDashboardJobPage(writer http.ResponseWriter, request *http.Request, store *JobStore) {
	job, ok := store.Get(request.PathValue("id"))
	if !ok {
		http.NotFound(writer, request)
		return
	}
	renderDashboard(writer, DashboardJobPage(job))
}

func handleDashboardJobStatus(writer http.ResponseWriter, request *http.Request, store *JobStore) {
	job, ok := store.Get(request.PathValue("id"))
	if !ok {
		http.NotFound(writer, request)
		return
	}
	renderDashboard(writer, DashboardJobStatus(job))
}

func decodeJobRequest(writer http.ResponseWriter, request *http.Request) (APIRequest, []map[string]any, error) {
	defer request.Body.Close()
	decoder := json.NewDecoder(http.MaxBytesReader(writer, request.Body, 2<<20))
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
	if err := request.ParseForm(); err != nil {
		return APIRequest{}, nil, err
	}
	rows := []map[string]any{}
	if raw := strings.TrimSpace(request.FormValue("rows")); raw != "" {
		if err := json.Unmarshal([]byte(raw), &rows); err != nil {
			return APIRequest{}, nil, fmt.Errorf("rows must be a JSON array: %w", err)
		}
	}
	if len(rows) == 0 {
		rows = []map[string]any{{}}
	}
	input := APIRequest{
		Instructions:    request.FormValue("instructions"),
		Template:        request.FormValue("template"),
		Schema:          json.RawMessage(request.FormValue("schema")),
		Rows:            rows,
		Model:           request.FormValue("model"),
		MaxSteps:        5,
		MaxOutputTokens: 1500,
		Concurrency:     5,
	}
	if err := validateAPIRequest(input); err != nil {
		return APIRequest{}, nil, err
	}
	return input, rows, nil
}

func validateAPIRequest(input APIRequest) error {
	if input.Instructions == "" || input.Template == "" || !json.Valid(input.Schema) {
		return fmt.Errorf("instructions, template, and a valid schema are required")
	}
	_, err := compileOutputSchema(input.Schema)
	return err
}

func renderDashboard(writer http.ResponseWriter, component templ.Component) {
	writer.Header().Set("Content-Type", "text/html; charset=utf-8")
	if err := component.Render(context.Background(), writer); err != nil {
		http.Error(writer, err.Error(), http.StatusInternalServerError)
	}
}

func dashboardInput(value map[string]any) string {
	data, _ := json.Marshal(value)
	return string(data)
}

func dashboardActive(job DashboardJob) bool {
	return job.Status == "queued" || job.Status == "running"
}
