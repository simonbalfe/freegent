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
		var err error
		document, err = shortFormSchema(document)
		if err != nil {
			return nil, err
		}
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

func shortFormSchema(shape map[string]any) (map[string]any, error) {
	properties := map[string]any{}
	required := make([]any, 0, len(shape))
	for name, spec := range shape {
		field, err := shortFieldSchema(spec)
		if err != nil {
			return nil, fmt.Errorf("invalid shorthand schema field %q: %w", name, err)
		}
		properties[name] = field
		required = append(required, name)
	}
	return map[string]any{
		"$schema":              "https://json-schema.org/draft/2020-12/schema",
		"type":                 "object",
		"properties":           properties,
		"required":             required,
		"additionalProperties": false,
	}, nil
}

func shortFieldSchema(spec any) (map[string]any, error) {
	value, ok := spec.(string)
	if !ok {
		return nil, errors.New("type must be a string")
	}
	value = strings.TrimSpace(value)
	if strings.HasPrefix(strings.ToLower(value), "enum:") {
		return nil, errors.New("enum: syntax is not supported")
	}
	nullable := strings.HasSuffix(value, "?")
	value = strings.TrimSpace(strings.TrimSuffix(value, "?"))
	parts := splitSpec(value)
	for _, part := range parts {
		if strings.EqualFold(part, "null") {
			return nil, errors.New("use ? for nullable fields")
		}
	}
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
		switch token {
		case "number", "boolean", "integer", "string":
			schema = map[string]any{"type": token}
		default:
			return nil, fmt.Errorf("unsupported shorthand type %q; use full JSON Schema for arrays and objects", value)
		}
	}
	if nullable {
		schema = map[string]any{"anyOf": []any{schema, map[string]any{"type": "null"}}}
	}
	return schema, nil
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
