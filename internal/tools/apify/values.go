package apify

import (
	"encoding/json"
	"net/url"
	"strconv"
	"strings"
)

const maxToolCharacters = 12000

func boundedInt(value any, fallback, minimum, maximum int) int {
	result := intValue(value, fallback)
	if result < minimum {
		return minimum
	}
	if result > maximum {
		return maximum
	}
	return result
}

func object(value any) map[string]any {
	result, _ := value.(map[string]any)
	if result == nil {
		return map[string]any{}
	}
	return result
}

func objectSlice(value any) []map[string]any {
	values, _ := value.([]any)
	result := make([]map[string]any, 0, len(values))
	for _, value := range values {
		if item, ok := value.(map[string]any); ok {
			result = append(result, item)
		}
	}
	return result
}

func stringSlice(value any) []string {
	values, _ := value.([]any)
	result := []string{}
	for _, value := range values {
		if item := text(value); item != "" {
			result = append(result, item)
		}
	}
	return result
}

func text(value any) string {
	switch typed := value.(type) {
	case string:
		return typed
	case json.Number:
		return typed.String()
	case float64:
		return strconv.FormatFloat(typed, 'f', -1, 64)
	case float32:
		return strconv.FormatFloat(float64(typed), 'f', -1, 64)
	case int:
		return strconv.Itoa(typed)
	case int64:
		return strconv.FormatInt(typed, 10)
	default:
		return ""
	}
}

func boolValue(value any) bool {
	result, _ := value.(bool)
	return result
}

func nullableNumber(value any) any {
	switch value.(type) {
	case float64, float32, int, int64, json.Number:
		return value
	default:
		return nil
	}
}

func numberOrZero(value any) any {
	if result := nullableNumber(value); result != nil {
		return result
	}
	return 0
}

func nullableValue(value any) any {
	if value == nil || text(value) == "" {
		return nil
	}
	return value
}

func firstValue(values ...any) any {
	for _, value := range values {
		if value == nil {
			continue
		}
		if stringValue, ok := value.(string); ok && strings.TrimSpace(stringValue) == "" {
			continue
		}
		return value
	}
	return nil
}

func firstString(value any) string {
	if values := stringSlice(value); len(values) > 0 {
		return values[0]
	}
	return ""
}

func foundedYear(value any) any {
	if nested := object(value); len(nested) > 0 {
		return firstValue(nested["year"])
	}
	return firstValue(value)
}

func entityNames(value any, limit int) []string {
	values, _ := value.([]any)
	result := []string{}
	for _, value := range values {
		name := text(value)
		if name == "" {
			name = text(object(value)["name"])
		}
		if name != "" {
			result = append(result, name)
		}
		if len(result) == limit {
			break
		}
	}
	return result
}

func firstCurrentPosition(positions []map[string]any) map[string]any {
	for _, position := range positions {
		if boolValue(position["current"]) {
			return position
		}
	}
	if len(positions) > 0 {
		return positions[0]
	}
	return map[string]any{}
}

func emailValue(item map[string]any) any {
	if email := text(item["email"]); email != "" {
		return map[string]any{"address": email}
	}
	if email := object(item["email"]); len(email) > 0 {
		address := firstNonEmpty(text(email["email"]), text(email["address"]))
		if address != "" {
			result := map[string]any{"address": address}
			if status := text(email["status"]); status != "" {
				result["status"] = status
			}
			if quality := nullableNumber(email["qualityScore"]); quality != nil {
				result["qualityScore"] = quality
			}
			return result
		}
	}
	for _, field := range []string{"emails", "contactEmails"} {
		if values := stringSlice(item[field]); len(values) > 0 {
			return map[string]any{"address": values[0]}
		}
	}
	return nil
}

func clipLength(value string, maximum int) string {
	value = strings.TrimSpace(value)
	if len(value) <= maximum {
		return value
	}
	return value[:maximum]
}

func isHTTPURL(value string) bool {
	parsed, err := url.Parse(value)
	return err == nil && (parsed.Scheme == "http" || parsed.Scheme == "https") && parsed.Host != ""
}

func validURLs(values ...string) []string {
	result := []string{}
	for _, value := range values {
		if isHTTPURL(value) {
			result = append(result, value)
		}
	}
	return uniqueStrings(result)
}

func limitStrings(values []string, maximum int) []string {
	return values[:min(maximum, len(values))]
}

func joinNonEmpty(values ...string) string {
	result := []string{}
	for _, value := range values {
		if strings.TrimSpace(value) != "" {
			result = append(result, strings.TrimSpace(value))
		}
	}
	return strings.Join(result, ", ")
}

func objectSchema(properties map[string]any, required []string) map[string]any {
	return map[string]any{"type": "object", "properties": properties, "required": required, "additionalProperties": false}
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
