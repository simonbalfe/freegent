package agent

import "strings"

func guardedToolURL(name string, input map[string]any) string {
	var value string
	switch name {
	case "fetch_page", "linkedin_profile":
		value, _ = input["url"].(string)
	case "linkedin_posts":
		value, _ = input["profileUrl"].(string)
	case "linkedin_post_reactions":
		value, _ = input["postUrl"].(string)
	case "linkedin_find_people", "linkedin_company", "crunchbase_company":
		value, _ = input["company"].(string)
	}
	value = strings.TrimSpace(value)
	if strings.HasPrefix(value, "http://") || strings.HasPrefix(value, "https://") {
		return value
	}
	return ""
}
