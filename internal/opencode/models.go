package opencode

// projectModels maps OpenCode provider/model pairs to the Modex picker shape.
// OpenCode has no reasoning-effort catalogue, so the effort lists stay empty
// and keep Android's picker code compatible.
func projectModels(models []modelRef) []map[string]any {
	data := make([]map[string]any, 0, len(models))
	seen := map[string]bool{}
	for _, model := range models {
		if model.ProviderID == "" || model.ModelID == "" {
			continue
		}
		id := model.ProviderID + "/" + model.ModelID
		if seen[id] {
			continue
		}
		seen[id] = true
		data = append(data, map[string]any{
			"id":                        id,
			"model":                     model.ModelID,
			"displayName":               model.ModelID,
			"defaultReasoningEffort":    "",
			"supportedReasoningEfforts": []any{},
		})
	}
	return data
}

func projectModelList(models []modelRef) map[string]any {
	return map[string]any{"data": projectModels(models), "next_cursor": nil}
}
