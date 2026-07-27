package claygent

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestBuildCLIRequestFromActionAndCSV(t *testing.T) {
	directory := t.TempDir()
	actionPath := filepath.Join(directory, "action.json")
	rowsPath := filepath.Join(directory, "rows.csv")
	if err := os.WriteFile(actionPath, []byte(`{"name":"crm","instructions":"Find the CRM.","template":"{{company}} {{domain}}","schema":{"crm":"string?","confidence":"low|medium|high"}}`), 0644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(rowsPath, []byte("company,domain\nLinear,linear.app\nVercel,vercel.com\n"), 0644); err != nil {
		t.Fatal(err)
	}
	request, rows, err := buildCLIRequest(actionPath, "", "", "", "", nil, rowsPath)
	if err != nil {
		t.Fatal(err)
	}
	if request.Name != "crm" || len(rows) != 2 || rows[1]["domain"] != "vercel.com" {
		t.Fatalf("unexpected CLI request: %#v %#v", request, rows)
	}
}

func TestBuildCLIRequestMergesRepeatedInputsAndRow(t *testing.T) {
	request, rows, err := buildCLIRequest(
		"",
		"Research.",
		"{{company}} {{domain}}",
		`{"summary":"string"}`,
		"domain=linear.app",
		repeatedInputs{"company": "Linear"},
		"",
	)
	if err != nil {
		t.Fatal(err)
	}
	if request.Template != "{{company}} {{domain}}" || rows[0]["company"] != "Linear" || rows[0]["domain"] != "linear.app" {
		t.Fatalf("unexpected merged row: %#v", rows)
	}
}

func TestResearchInstructionsIncludeDoctrineAndTaskRules(t *testing.T) {
	value := researchInstructions("Check only official sources.", `{"name":{"type":"string"}}`)
	for _, expected := range []string{"Never fabricate a URL", "Task-specific rules:", "Check only official sources.", "Answer schema:"} {
		if !strings.Contains(value, expected) {
			t.Fatalf("missing %q in instructions", expected)
		}
	}
}
