package acp

import "strings"

// Environment shares AGY's durable OAuth token between terminal and ACP.
func Environment(env []string) []string {
	result := make([]string, 0, len(env)+1)
	for _, pair := range env {
		if !strings.HasPrefix(pair, "AGY_CLI_FORCE_FILE_TOKEN_STORAGE=") {
			result = append(result, pair)
		}
	}
	return append(result, "AGY_CLI_FORCE_FILE_TOKEN_STORAGE=1")
}
