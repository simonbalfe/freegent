package agent

import (
	"encoding/json"
	"testing"
)

func TestShortFormSchemaValidation(t *testing.T) {
	schema, err := CompileOutputSchema(json.RawMessage(`{"name":"string","score":"number?","confidence":"low|medium|high","active":"boolean"}`))
	if err != nil {
		t.Fatal(err)
	}
	valid := map[string]any{"name": "Acme", "score": nil, "confidence": "high", "active": true}
	if err := schema.Validate(valid); err != nil {
		t.Fatalf("expected valid output: %v", err)
	}
	invalid := map[string]any{"name": "Acme", "score": "wrong", "confidence": "certain", "active": true}
	if err := schema.Validate(invalid); err == nil {
		t.Fatal("expected invalid output to fail")
	}
}

func TestJSONSchemaValidation(t *testing.T) {
	raw := json.RawMessage(`{"type":"object","properties":{"count":{"type":"integer","minimum":1}},"required":["count"],"additionalProperties":false}`)
	schema, err := CompileOutputSchema(raw)
	if err != nil {
		t.Fatal(err)
	}
	if err := schema.Validate(map[string]any{"count": float64(2)}); err != nil {
		t.Fatalf("expected valid output: %v", err)
	}
	if err := schema.Validate(map[string]any{"count": float64(0)}); err == nil {
		t.Fatal("expected minimum violation")
	}
}

func TestRemoteSchemaReferenceRejected(t *testing.T) {
	_, err := CompileOutputSchema(json.RawMessage(`{"$ref":"https://example.com/schema.json"}`))
	if err == nil {
		t.Fatal("expected remote reference to be rejected")
	}
}
