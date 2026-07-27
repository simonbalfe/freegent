package claygent

import (
	"encoding/json"
	"strings"
)

const maxToolCharacters = 12000

func objectSchema(properties map[string]any, required []string) map[string]any {
	return map[string]any{"type": "object", "properties": properties, "required": required, "additionalProperties": false}
}

func parseJSONObject(value string) (map[string]any, error) {
	cleaned := strings.TrimSpace(value)
	cleaned = strings.TrimPrefix(cleaned, "```json")
	cleaned = strings.TrimPrefix(cleaned, "```")
	cleaned = strings.TrimSuffix(strings.TrimSpace(cleaned), "```")
	answer := map[string]any{}
	if err := json.Unmarshal([]byte(cleaned), &answer); err != nil {
		return nil, err
	}
	return answer, nil
}

func stringValue(value any) string {
	text, _ := value.(string)
	return text
}

func intValue(value any, fallback int) int {
	switch number := value.(type) {
	case int:
		return number
	case int64:
		return int(number)
	case float64:
		return int(number)
	case json.Number:
		parsed, err := number.Int64()
		if err == nil {
			return int(parsed)
		}
	}
	return fallback
}

func firstNonEmpty(values ...string) string {
	for _, value := range values {
		if value != "" {
			return value
		}
	}
	return ""
}

func bounded(value string) string {
	value = strings.TrimSpace(value)
	if len(value) <= maxToolCharacters {
		return value
	}
	return value[:maxToolCharacters] + "\n[truncated]"
}

func uniqueStrings(values []string) []string {
	seen := map[string]bool{}
	result := []string{}
	for _, value := range values {
		if value != "" && !seen[value] {
			seen[value] = true
			result = append(result, value)
		}
	}
	return result
}
