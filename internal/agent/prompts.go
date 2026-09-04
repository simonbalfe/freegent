package agent

import (
	_ "embed"
	"strings"
)

//go:embed prompts/research.md
var researchSystemPrompt string

//go:embed prompts/finalizer.md
var finalizerSystemPrompt string

//go:embed prompts/business-fields.md
var businessFieldRules string

func ResearchInstructions(userInstructions string, schema string) string {
	parts := []string{
		strings.TrimSpace(researchSystemPrompt),
		strings.TrimSpace(businessFieldRules),
		"Task-specific rules:\n" + strings.TrimSpace(userInstructions),
		"Return only a JSON object matching the answer schema exactly.\nAnswer schema: " + schema,
	}
	return strings.Join(parts, "\n\n")
}

func FinalizerInstructions(userInstructions string) string {
	parts := []string{
		strings.TrimSpace(finalizerSystemPrompt),
		strings.TrimSpace(businessFieldRules),
	}
	if strings.TrimSpace(userInstructions) != "" {
		parts = append(parts, "Task-specific rules:\n"+strings.TrimSpace(userInstructions))
	}
	return strings.Join(parts, "\n\n")
}
