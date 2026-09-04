package agent

func guardedToolURL(tool Tool, input map[string]any) string {
	return tool.GuardedURL(input)
}
