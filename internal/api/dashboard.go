package api

import (
	"database/sql"
	"encoding/csv"
	"encoding/json"
	"errors"
	"fmt"
	"html/template"
	"io"
	"mime"
	"net/http"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/simonbalfe/freegent/internal/agent"
)

var dashboardTemplates = template.Must(template.New("dashboard").Funcs(template.FuncMap{
	"active":       dashboardActive,
	"eventLabel":   dashboardEventLabel,
	"eventMessage": dashboardEventMessage,
	"eventTime":    dashboardEventTime,
	"fields":       dashboardFields,
	"jobLabel":     dashboardJobLabel,
	"jobTime":      dashboardJobTime,
	"pretty":       dashboardPretty,
	"rowNumber":    func(index int) int { return index + 1 },
	"stepLabel":    dashboardStepLabel,
	"tokens":       dashboardTokens,
}).Parse(dashboardTemplateSource))

const dashboardTemplateSource = `{{define "dashboard"}}
<!doctype html>
<html lang="en">
<head>
  <meta charset="utf-8">
  <meta name="viewport" content="width=device-width, initial-scale=1">
  <title>Freegent</title>
  <script src="https://unpkg.com/htmx.org@2.0.4"></script>
  <style>body{font:16px system-ui;max-width:960px;margin:40px auto;padding:0 20px;color:#17202a;background:#fafafa}h1{margin-bottom:8px}h2{margin-top:32px}.card{background:#fff;border:1px solid #e2e4e8;border-radius:10px;padding:24px;margin:20px 0;box-shadow:0 2px 8px #17202a0a}form.card label{display:block;margin:16px 0 7px;font-weight:600}form.card label:first-child{margin-top:0}textarea,input{width:100%;box-sizing:border-box;margin:0 0 4px;padding:11px 12px;border:1px solid #98a2b3;border-radius:6px;font:14px ui-monospace,monospace;line-height:1.45;background:#fff}textarea{min-height:108px;resize:vertical}button{margin-top:12px;padding:11px 18px;border:0;border-radius:6px;background:#175cd3;color:#fff;font-weight:600;cursor:pointer}.muted{color:#667085}.ok{color:#18794e}.error{color:#b42318}table{width:100%;border-collapse:collapse}td,th{text-align:left;border-bottom:1px solid #eee;padding:8px}</style>
</head>
<body>
  <h1>Freegent</h1>
  <p class="muted"><strong>Open source Freegent</strong> for researching any spreadsheet.</p>
  <p class="muted">Upload a CSV or paste JSON rows, then watch every row complete. Job history survives API restarts.</p>
  <form method="post" action="/dashboard/jobs" enctype="multipart/form-data" class="card">
    <label>Job name</label>
    <input name="name" placeholder="Optional, defaults to the CSV filename">
    <label>Instructions</label>
    <textarea name="instructions">Use current reliable evidence. Follow the requested task exactly and do not guess unsupported facts.</textarea>
    <label>Template</label>
    <input name="template" value="Research {{"{{subject}}"}} using {{"{{url}}"}} when supplied. Return a concise factual answer.">
    <label>Output schema</label>
    <textarea name="schema">{"answer":"string"}</textarea>
    <label>CSV file</label>
    <input type="file" name="csv" accept=".csv,text/csv">
    <p class="muted">The first row must contain headers matching template fields such as {{"{{subject}}"}} and {{"{{url}}"}}. A CSV takes precedence over JSON rows.</p>
    <label>Rows (optional JSON array)</label>
    <textarea name="rows">[
  {"subject":"Figma","url":"https://figma.com"},
  {"subject":"Dario Amodei","url":""},
  {"subject":"EU AI Act","url":""}
]</textarea>
    <button type="submit">Start research</button>
  </form>
  <h2>Recent jobs</h2>
  {{if eq (len .) 0}}
    <p class="muted">No jobs yet.</p>
  {{else}}
    {{range .}}
      <div class="card"><a href="/dashboard/jobs/{{.ID}}">{{jobLabel .}}</a> · {{.Status}} · {{.Completed}}/{{.Total}} rows · <span class="muted">{{jobTime .CreatedAt}}</span></div>
    {{end}}
  {{end}}
</body>
</html>
{{end}}

{{define "job"}}
<!doctype html>
<html lang="en">
<head>
  <meta charset="utf-8">
  <meta name="viewport" content="width=device-width, initial-scale=1">
  <title>Freegent job</title>
  <script src="https://unpkg.com/htmx.org@2.0.4"></script>
  <script>
    let openTraces = []
    document.addEventListener("htmx:beforeSwap", event => {
      if (event.detail.target.id === "job-status") openTraces = [...event.detail.target.querySelectorAll(".trace[open]")].map(trace => trace.id)
    })
    document.addEventListener("htmx:afterSwap", event => {
      if (event.detail.target.id === "job-status") openTraces.forEach(id => document.getElementById(id)?.setAttribute("open", ""))
    })
  </script>
  <style>body{font:16px system-ui;max-width:960px;margin:40px auto;padding:0 20px;color:#17202a;background:#fafafa}.card{background:#fff;border:1px solid #e2e4e8;border-radius:12px;padding:20px;margin:16px 0;box-shadow:0 2px 8px #17202a0a}.muted{color:#667085}.error{color:#b42318}.summary{display:grid;grid-template-columns:repeat(3,1fr);gap:12px;margin:20px 0}.metric{background:#f5f7fa;border-radius:8px;padding:12px}.metric strong{display:block;font-size:20px}.flow{border:1px solid #e2e4e8;border-radius:8px;max-height:280px;overflow:auto}.event{display:grid;grid-template-columns:150px 1fr;gap:12px;padding:10px 12px;border-bottom:1px solid #eaecf0}.event:last-child{border-bottom:0}.event span{color:#667085;font:13px ui-monospace,monospace}.row-card{border:1px solid #e2e4e8;border-radius:10px;padding:18px;margin:12px 0}.row-head{display:flex;gap:16px;align-items:center;flex-wrap:wrap}.status{color:#18794e;font-weight:600}.tokens{color:#667085;font-size:14px}.answer{display:grid;gap:10px;margin:12px 0}.answer-row{background:#f8fafc;border-radius:7px;padding:11px 12px}.answer-row dt,.step-input-row dt{font-weight:600}.answer-row dd,.step-input-row dd{margin:4px 0 0;white-space:pre-wrap}.trace{border-top:1px solid #eaecf0;padding-top:12px}.trace ol{border-left:2px solid #d0d5dd;list-style:none;margin-left:8px;padding-left:20px}.step{margin:16px 0;position:relative}.step:before{background:#175cd3;border:3px solid #fff;border-radius:50%;content:"";height:8px;left:-26px;position:absolute;top:5px;width:8px}.step-title{font-weight:600}.step-input{display:grid;gap:6px;margin-top:8px}.step-input-row{font-size:14px}.sources{font-size:14px;padding-left:20px}details{margin-top:12px}summary{cursor:pointer;font-weight:600}a{color:#175cd3}@media(max-width:640px){.summary{grid-template-columns:1fr}.event{grid-template-columns:1fr;gap:3px}}</style>
</head>
<body>
  <p><a href="/dashboard">← New job</a></p>
  <h1>Job {{.ID}}</h1>
  <div id="job-status">{{template "job-status" .}}</div>
</body>
</html>
{{end}}

{{define "job-status"}}
<div class="card"{{if active .}} hx-get="/dashboard/jobs/{{.ID}}/status" hx-trigger="every 1s" hx-target="#job-status" hx-swap="innerHTML"{{end}}>
  <div class="row-head"><h2>{{.Status}}</h2><span class="muted">{{.LatestEvent}}</span></div>
  <div class="summary">
    <div class="metric"><span class="muted">Rows</span><strong>{{.Completed}}/{{.Total}}</strong></div>
    <div class="metric"><span class="muted">Input tokens</span><strong>{{(tokens .).Input}}</strong></div>
    <div class="metric"><span class="muted">Output tokens</span><strong>{{(tokens .).Output}}</strong></div>
  </div>
  <h3>Live activity</h3>
  <div class="flow">
    {{range .Events}}
      <div class="event"><span>{{eventTime .At}} · {{eventLabel .}}</span> <strong>{{eventMessage .Message}}</strong></div>
    {{end}}
  </div>
  <h3>Row runs</h3>
  {{range .Rows}}
    <article class="row-card">
      <div class="row-head"><strong>Row {{rowNumber .Index}}</strong><span class="status">{{.Status}}</span><span class="tokens">{{.Result.Tokens.Input}} input · {{.Result.Tokens.Output}} output tokens</span></div>
      {{if .Result.Result}}
        <dl class="answer">
          {{range fields .Result.Result}}
            <div class="answer-row"><dt>{{.Key}}</dt><dd>{{.Value}}</dd></div>
          {{end}}
        </dl>
      {{end}}
      {{if .Result.Error}}
        <p class="error"><strong>Error:</strong> {{.Result.Error}}</p>
      {{end}}
      <details class="trace" id="trace-{{.Index}}"><summary>Agent trace · {{len .Result.AgentLog}} steps</summary>
        {{if eq (len .Result.AgentLog) 0}}
          <p class="muted">No completed steps yet. Watch the live activity above.</p>
        {{else}}
          <ol>
            {{range .Result.AgentLog}}
              {{template "step" .}}
            {{end}}
          </ol>
        {{end}}
        {{if .Result.Sources}}
          {{template "sources" .Result.Sources}}
        {{end}}
      </details>
    </article>
  {{end}}
  {{if not (active .)}}
    <p><a href="/jobs/{{.ID}}/results.csv">Download enriched CSV</a> · <a href="/jobs/{{.ID}}">View JSON status</a></p>
  {{end}}
</div>
{{end}}

{{define "step"}}
<li class="step">
  <div class="step-title">{{stepLabel .}}</div>
  {{if .Input}}
    <dl class="step-input">
      {{range fields .Input}}
        <div class="step-input-row"><dt>{{.Key}}</dt><dd>{{.Value}}</dd></div>
      {{end}}
    </dl>
  {{end}}
</li>
{{end}}

{{define "sources"}}
<p><strong>Evidence sources</strong></p>
<ul class="sources">
  {{range .}}
    <li><a href="{{.}}" target="_blank" rel="noreferrer">{{.}}</a></li>
  {{end}}
</ul>
{{end}}`

func handleJob(writer http.ResponseWriter, request *http.Request, store *PostgresStore) {
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

func handleJobsJSON(writer http.ResponseWriter, request *http.Request, store *PostgresStore) {
	jobs, err := store.List(50)
	if err != nil {
		writeJSON(writer, http.StatusInternalServerError, map[string]string{"error": err.Error()})
		return
	}
	writeJSON(writer, http.StatusOK, map[string]any{"jobs": jobs})
}

func handleJobJSON(writer http.ResponseWriter, request *http.Request, store *PostgresStore) {
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

func handleDashboard(writer http.ResponseWriter, request *http.Request, store *PostgresStore) {
	jobs, err := store.List(50)
	if err != nil {
		http.Error(writer, err.Error(), http.StatusInternalServerError)
		return
	}
	renderDashboard(writer, "dashboard", jobs)
}

func handleDashboardJob(writer http.ResponseWriter, request *http.Request, store *PostgresStore) {
	request.Body = http.MaxBytesReader(writer, request.Body, 32<<20)
	input, rows, err := decodeMultipartJobForm(request)
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

func handleDashboardJobPage(writer http.ResponseWriter, request *http.Request, store *PostgresStore) {
	job, err := store.GetPage(request.PathValue("id"), 200, 0)
	if errors.Is(err, sql.ErrNoRows) {
		http.NotFound(writer, request)
		return
	}
	if err != nil {
		http.Error(writer, err.Error(), http.StatusInternalServerError)
		return
	}
	renderDashboard(writer, "job", job)
}

func handleDashboardJobStatus(writer http.ResponseWriter, request *http.Request, store *PostgresStore) {
	job, err := store.GetPage(request.PathValue("id"), 200, 0)
	if errors.Is(err, sql.ErrNoRows) {
		http.NotFound(writer, request)
		return
	}
	if err != nil {
		http.Error(writer, err.Error(), http.StatusInternalServerError)
		return
	}
	renderDashboard(writer, "job-status", job)
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

func renderDashboard(writer http.ResponseWriter, name string, value any) {
	writer.Header().Set("Content-Type", "text/html; charset=utf-8")
	if err := dashboardTemplates.ExecuteTemplate(writer, name, value); err != nil {
		http.Error(writer, err.Error(), http.StatusInternalServerError)
	}
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
