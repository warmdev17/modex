package bridge

import (
	"encoding/json"
	"strings"
)

type toolActivity struct {
	Query  string `json:"query,omitempty"`
	Action *struct {
		Type    string   `json:"type"`
		Query   string   `json:"query,omitempty"`
		Queries []string `json:"queries,omitempty"`
		URL     string   `json:"url,omitempty"`
		Pattern string   `json:"pattern,omitempty"`
	} `json:"action,omitempty"`
	CommandActions []struct {
		Type  string `json:"type"`
		Name  string `json:"name,omitempty"`
		Path  string `json:"path,omitempty"`
		Query string `json:"query,omitempty"`
	} `json:"commandActions,omitempty"`
	Changes []struct {
		Path string `json:"path"`
		Diff string `json:"diff"`
	} `json:"changes,omitempty"`
}

func mobileActivityParams(method string, raw json.RawMessage) (json.RawMessage, error) {
	var event struct {
		ThreadID string `json:"threadId"`
		TurnID   string `json:"turnId"`
		ItemID   string `json:"itemId"`
		Item     struct {
			toolActivity
			ID     string `json:"id"`
			Type   string `json:"type"`
			Status string `json:"status,omitempty"`
		} `json:"item"`
		toolActivity
	}
	if json.Unmarshal(raw, &event) != nil {
		return raw, nil
	}
	budget := 65536
	switch {
	case method == "item/fileChange/patchUpdated":
		return json.Marshal(map[string]any{"threadId": event.ThreadID, "turnId": event.TurnID, "itemId": event.ItemID, "changes": event.toolActivity.boundChanges(&budget).Changes})
	case event.Item.Type == "webSearch" || event.Item.Type == "fileChange":
		event.Item.toolActivity = event.Item.toolActivity.boundChanges(&budget)
		return json.Marshal(map[string]any{"threadId": event.ThreadID, "turnId": event.TurnID, "item": event.Item})
	default:
		return raw, nil
	}
}

func (item toolActivity) boundChanges(budget *int) toolActivity {
	for i := range item.Changes {
		diff := item.Changes[i].Diff
		limit := min(len(diff), 65536, *budget)
		if limit < len(diff) {
			item.Changes[i].Diff = strings.ToValidUTF8(diff[:limit], "") + "\n[Diff truncated; view full diff on host]"
		}
		*budget -= limit
	}
	return item
}
