package claygent

import (
	"bytes"
	"context"
	"encoding/csv"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io"
	"net/http"
	"os"
	"sort"
	"strconv"
	"strings"
)

const cliHelp = `freegent — research rows through the Freegent API

Quick start:
  freegent --instructions "Research this company." \
    --template "Research {{company}} at {{domain}}." \
    --schema '{"summary":"string","source":"string"}' \
    --row 'company=Figma,domain=figma.com' --pretty

Research a CSV:
  freegent --rows companies.csv \
    --instructions "Research each company for GTM outbound." \
    --template "Research {{company}} at {{domain}}." \
    --schema '{"product":"string","targetCustomer":"string"}' --json

Required:
  --instructions  What the agent should find
  --template      Prompt template using row fields, such as {{company}}
  --schema        JSON fields to return

Rows:
  --row k=v,k=v   One row (repeatable)
  --input k=v     One row field
  --rows file.csv CSV batch

Useful options:
  --concurrency n Run rows in parallel (default: 5)
  --api-url url   API address (default: http://localhost:8080)
  --pretty        Human-readable results
  --json          Full JSON results
  --out file      Save JSON results to a file
  --action file   Load a reusable job configuration

Run the local API:
  freegent serve --port 8080`

type repeatedInputs map[string]string

func (i *repeatedInputs) String() string {
	return ""
}

func (i *repeatedInputs) Set(value string) error {
	key, fieldValue, found := strings.Cut(value, "=")
	if !found || strings.TrimSpace(key) == "" {
		return fmt.Errorf("invalid --input %q; use key=value", value)
	}
	if *i == nil {
		*i = repeatedInputs{}
	}
	(*i)[strings.TrimSpace(key)] = fieldValue
	return nil
}

func runCLI(args []string) {
	flags := flag.NewFlagSet("openclaygent-go", flag.ContinueOnError)
	flags.SetOutput(io.Discard)
	var inputs repeatedInputs
	flags.Var(&inputs, "input", "row field key=value; repeatable")
	actionPath := flags.String("action", "", "action JSON file")
	instructions := flags.String("instructions", "", "research instructions")
	template := flags.String("template", "", "task template")
	task := flags.String("task", "", "alias for --template")
	schemaValue := flags.String("schema", "", "short form or JSON Schema")
	rowValue := flags.String("row", "", "comma-separated row fields")
	rowsPath := flags.String("rows", "", "CSV file")
	requireField := flags.String("require", "", "skip rows missing this field")
	model := flags.String("model", "google/gemini-3.1-flash-lite", "OpenRouter model")
	maxSteps := flags.Int("max-steps", 5, "maximum model/tool turns")
	maxOutputTokens := flags.Int("max-output-tokens", 1500, "maximum answer tokens")
	concurrency := flags.Int("concurrency", 5, "parallel rows")
	apiURL := flags.String("api-url", "", "OpenClaygent Go API")
	outPath := flags.String("out", "", "write full results JSON")
	jsonOutput := flags.Bool("json", false, "print full results")
	pretty := flags.Bool("pretty", false, "human-readable output")
	verbose := flags.Bool("verbose", false, "write agent events to stderr")
	demo := flags.Bool("demo", false, "offline deterministic demo")
	help := flags.Bool("help", false, "show help")
	if err := flags.Parse(args); err != nil {
		failCLI(err)
	}
	if *help || len(args) == 0 {
		fmt.Println(cliHelp)
		return
	}
	if *demo {
		runDemoCLI(*verbose)
		return
	}

	request, rows, err := buildCLIRequest(*actionPath, *instructions, firstNonEmpty(*template, *task), *schemaValue, *rowValue, inputs, *rowsPath)
	if err != nil {
		failCLI(err)
	}
	request.Model = *model
	request.MaxSteps = *maxSteps
	request.MaxOutputTokens = *maxOutputTokens
	request.Concurrency = *concurrency
	request.Require = *requireField
	request.Verbose = *verbose

	resolvedAPIURL := firstNonEmpty(*apiURL, os.Getenv("OPENCLAYGENT_API_URL"), "http://localhost:8080")
	results, err := runRemoteCLI(context.Background(), resolvedAPIURL, request)
	if err != nil {
		failCLI(err)
	}
	renderCLIResults(rows, results, *jsonOutput, *pretty)
	if *outPath != "" {
		data, err := json.MarshalIndent(results, "", "  ")
		if err != nil {
			failCLI(err)
		}
		if err := os.WriteFile(*outPath, data, 0644); err != nil {
			failCLI(err)
		}
		fmt.Fprintf(os.Stderr, "wrote %s\n", *outPath)
	}
}

func buildCLIRequest(actionPath, instructions, template, schemaValue, rowValue string, inputs repeatedInputs, rowsPath string) (APIRequest, []map[string]any, error) {
	request := APIRequest{}
	if actionPath != "" {
		data, err := os.ReadFile(actionPath)
		if err != nil {
			return request, nil, err
		}
		var action struct {
			Name         string          `json:"name"`
			Instructions string          `json:"instructions"`
			Template     string          `json:"template"`
			Schema       json.RawMessage `json:"schema"`
		}
		if err := json.Unmarshal(data, &action); err != nil {
			return request, nil, err
		}
		request.Name, request.Instructions, request.Template, request.Schema = action.Name, action.Instructions, action.Template, action.Schema
	} else {
		if instructions == "" {
			instructions = "Research the requested fields using current web evidence."
		}
		if template == "" || schemaValue == "" {
			return request, nil, errors.New("need --template and --schema, or --action <file>")
		}
		request.Instructions = instructions
		request.Template = template
		request.Schema = json.RawMessage(schemaValue)
	}
	if _, err := compileOutputSchema(request.Schema); err != nil {
		return request, nil, err
	}

	rows := []map[string]any{}
	if rowsPath != "" {
		loaded, err := readCSVRows(rowsPath)
		if err != nil {
			return request, nil, err
		}
		rows = loaded
	} else {
		row := map[string]any{}
		for key, value := range inputs {
			row[key] = value
		}
		if rowValue != "" {
			parsed, err := parseRow(rowValue)
			if err != nil {
				return request, nil, err
			}
			for key, value := range parsed {
				row[key] = value
			}
		}
		rows = []map[string]any{row}
	}
	request.Rows = rows
	return request, rows, nil
}

func readCSVRows(path string) ([]map[string]any, error) {
	file, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer file.Close()
	reader := csv.NewReader(file)
	records, err := reader.ReadAll()
	if err != nil {
		return nil, err
	}
	if len(records) == 0 {
		return []map[string]any{}, nil
	}
	headers := records[0]
	rows := make([]map[string]any, 0, len(records)-1)
	for _, record := range records[1:] {
		row := map[string]any{}
		for index, header := range headers {
			value := ""
			if index < len(record) {
				value = record[index]
			}
			row[strings.TrimSpace(header)] = value
		}
		rows = append(rows, row)
	}
	return rows, nil
}

func parseRow(value string) (Row, error) {
	row := Row{}
	if strings.TrimSpace(value) == "" {
		return row, nil
	}
	for _, pair := range strings.Split(value, ",") {
		key, fieldValue, found := strings.Cut(pair, "=")
		if !found || strings.TrimSpace(key) == "" {
			return nil, fmt.Errorf("invalid row field %q; use key=value,key=value", pair)
		}
		row[strings.TrimSpace(key)] = strings.TrimSpace(fieldValue)
	}
	return row, nil
}

func runRemoteCLI(ctx context.Context, baseURL string, request APIRequest) ([]APIResult, error) {
	endpoint := strings.TrimRight(baseURL, "/") + "/run"
	encoded, err := json.Marshal(request)
	if err != nil {
		return nil, err
	}
	httpRequest, err := http.NewRequestWithContext(ctx, http.MethodPost, endpoint, bytes.NewReader(encoded))
	if err != nil {
		return nil, err
	}
	httpRequest.Header.Set("Content-Type", "application/json")
	response, err := (&http.Client{}).Do(httpRequest)
	if err != nil {
		return nil, fmt.Errorf("cannot reach OpenClaygent Go API at %s: %w", endpoint, err)
	}
	defer response.Body.Close()
	data, err := io.ReadAll(io.LimitReader(response.Body, 20<<20))
	if err != nil {
		return nil, err
	}
	if response.StatusCode < 200 || response.StatusCode >= 300 {
		return nil, fmt.Errorf("API %s: %s", response.Status, string(data))
	}
	var payload struct {
		Results []APIResult `json:"results"`
	}
	if err := json.Unmarshal(data, &payload); err != nil {
		return nil, err
	}
	return payload.Results, nil
}

func renderCLIResults(rows []map[string]any, results []APIResult, fullJSON, pretty bool) {
	if fullJSON {
		value := any(results)
		if len(results) == 1 {
			value = results[0]
		}
		printJSON(value)
		return
	}
	if pretty {
		for index, result := range results {
			label := "row " + strconv.Itoa(index+1)
			if index < len(rows) {
				for _, value := range rows[index] {
					label = fmt.Sprint(value)
					break
				}
			}
			fmt.Printf("\n%s  %.1fs · %d in / %d out tok · %d sources\n", label, float64(result.DurationMS)/1000, result.Tokens.Input, result.Tokens.Output, len(result.Sources))
			if result.Skipped {
				fmt.Println("  skipped")
				continue
			}
			if result.Error != "" {
				fmt.Println("  error:", result.Error)
				continue
			}
			keys := make([]string, 0, len(result.Result))
			for key := range result.Result {
				keys = append(keys, key)
			}
			sort.Strings(keys)
			for _, key := range keys {
				fmt.Printf("  %s  %v\n", key, result.Result[key])
			}
			if result.Reasoning != "" {
				fmt.Println("  »", result.Reasoning)
			}
		}
		return
	}
	answers := make([]map[string]any, len(results))
	for index, result := range results {
		answers[index] = map[string]any{"result": result.Result, "reasoning": result.Reasoning, "sources": result.Sources}
	}
	if len(answers) == 1 {
		printJSON(answers[0])
	} else {
		printJSON(answers)
	}
}

func runDemoCLI(verbose bool) {
	schema, err := compileOutputSchema(json.RawMessage(`{"company":"string","summary":"string","confidence":"low|medium|high"}`))
	if err != nil {
		failCLI(err)
	}
	action := Action{
		Instructions:          researchInstructions("Research the company.", string(schema.Canonical)),
		FinalizerInstructions: finalizerInstructions("Research the company."),
		Template:              "Research {{company}}.",
		Schema:                schema.Canonical,
		Validator:             schema,
	}
	result, err := (Agent{
		Model:    DemoModel{},
		Tools:    map[string]Tool{"web_search": DemoSearchTool{}},
		MaxSteps: 3,
		Verbose:  verbose,
	}).Run(context.Background(), action, Row{"company": "Acme"})
	if err != nil {
		failCLI(err)
	}
	printJSON(result)
}

func printJSON(value any) {
	encoded, err := json.MarshalIndent(value, "", "  ")
	if err != nil {
		failCLI(err)
	}
	fmt.Println(string(encoded))
}

func failCLI(err error) {
	fmt.Fprintln(os.Stderr, err)
	os.Exit(1)
}
