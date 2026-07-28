package agent

import (
	"encoding/json"
	"errors"
	"fmt"
	"strings"

	"github.com/santhosh-tekuri/jsonschema/v6"
)

type CompiledSchema struct {
	Canonical json.RawMessage
	Document  map[string]any
	validator *jsonschema.Schema
}

func CompileOutputSchema(raw json.RawMessage) (*CompiledSchema, error) {
	var document map[string]any
	if err := json.Unmarshal(raw, &document); err != nil {
		return nil, fmt.Errorf("invalid schema JSON: %w", err)
	}
	if !isJSONSchema(document) {
		document = shortFormSchema(document)
	}
	if err := rejectRemoteRefs(document); err != nil {
		return nil, err
	}
	compiler := jsonschema.NewCompiler()
	compiler.DefaultDraft(jsonschema.Draft2020)
	if err := compiler.AddResource("urn:freegent:output", document); err != nil {
		return nil, fmt.Errorf("add output schema: %w", err)
	}
	validator, err := compiler.Compile("urn:freegent:output")
	if err != nil {
		return nil, fmt.Errorf("compile output schema: %w", err)
	}
	canonical, err := json.Marshal(document)
	if err != nil {
		return nil, err
	}
	return &CompiledSchema{Canonical: canonical, Document: document, validator: validator}, nil
}

func (s *CompiledSchema) Validate(value any) error {
	if s == nil || s.validator == nil {
		return errors.New("output schema is not compiled")
	}
	return s.validator.Validate(value)
}

func isJSONSchema(document map[string]any) bool {
	_, hasType := document["type"]
	_, hasProperties := document["properties"]
	_, hasSchema := document["$schema"]
	_, hasReference := document["$ref"]
	return hasType || hasProperties || hasSchema || hasReference
}

func shortFormSchema(shape map[string]any) map[string]any {
	properties := map[string]any{}
	required := make([]any, 0, len(shape))
	for name, spec := range shape {
		properties[name] = shortFieldSchema(spec)
		required = append(required, name)
	}
	return map[string]any{
		"$schema":              "https://json-schema.org/draft/2020-12/schema",
		"type":                 "object",
		"properties":           properties,
		"required":             required,
		"additionalProperties": false,
	}
}

func shortFieldSchema(spec any) map[string]any {
	if values, ok := spec.([]any); ok {
		enum := make([]any, 0, len(values))
		for _, value := range values {
			enum = append(enum, fmt.Sprint(value))
		}
		return map[string]any{"type": "string", "enum": enum}
	}
	value := strings.TrimSpace(fmt.Sprint(spec))
	nullable := strings.HasSuffix(value, "?")
	value = strings.TrimSpace(strings.TrimSuffix(value, "?"))
	parts := splitSpec(value)
	filtered := parts[:0]
	for _, part := range parts {
		if part == "null" {
			nullable = true
		} else {
			filtered = append(filtered, part)
		}
	}
	parts = filtered
	var schema map[string]any
	if len(parts) > 1 {
		enum := make([]any, len(parts))
		for index, part := range parts {
			enum[index] = part
		}
		schema = map[string]any{"type": "string", "enum": enum}
	} else {
		token := "string"
		if len(parts) == 1 {
			token = strings.ToLower(parts[0])
		}
		if strings.HasPrefix(token, "enum:") {
			values := splitComma(strings.TrimPrefix(token, "enum:"))
			enum := make([]any, len(values))
			for index, item := range values {
				enum[index] = item
			}
			schema = map[string]any{"type": "string", "enum": enum}
		} else {
			switch token {
			case "number", "boolean", "integer", "string":
				schema = map[string]any{"type": token}
			default:
				schema = map[string]any{"type": "string"}
			}
		}
	}
	if nullable {
		schema = map[string]any{"anyOf": []any{schema, map[string]any{"type": "null"}}}
	}
	return schema
}

func splitSpec(value string) []string {
	parts := strings.Split(value, "|")
	output := make([]string, 0, len(parts))
	for _, part := range parts {
		if trimmed := strings.TrimSpace(part); trimmed != "" {
			output = append(output, trimmed)
		}
	}
	return output
}

func splitComma(value string) []string {
	parts := strings.Split(value, ",")
	output := make([]string, 0, len(parts))
	for _, part := range parts {
		if trimmed := strings.TrimSpace(part); trimmed != "" {
			output = append(output, trimmed)
		}
	}
	return output
}

func rejectRemoteRefs(value any) error {
	switch typed := value.(type) {
	case map[string]any:
		for key, child := range typed {
			if key == "$ref" {
				if reference, ok := child.(string); ok && (strings.HasPrefix(reference, "http://") || strings.HasPrefix(reference, "https://")) {
					return fmt.Errorf("remote $ref is not supported: %s", reference)
				}
			}
			if err := rejectRemoteRefs(child); err != nil {
				return err
			}
		}
	case []any:
		for _, child := range typed {
			if err := rejectRemoteRefs(child); err != nil {
				return err
			}
		}
	}
	return nil
}
