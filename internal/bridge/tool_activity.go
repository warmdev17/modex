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
		ThreadID string          `json:"threadId"`
		TurnID   string          `json:"turnId"`
		ItemID   string          `json:"itemId"`
		Item     json.RawMessage `json:"item"`
		toolActivity
	}
	var item struct {
		toolActivity
		ID     string `json:"id"`
		Type   string `json:"type"`
		Status string `json:"status,omitempty"`
	}
	eventErr := json.Unmarshal(raw, &event)
	itemErr := json.Unmarshal(event.Item, &item)
	if isAgentActivity(item.Type) {
		var native nativeAgentActivity
		if json.Unmarshal(event.Item, &native) != nil {
			native = nativeAgentActivity{}
		}
		return json.Marshal(map[string]any{"threadId": event.ThreadID, "turnId": event.TurnID, "item": native.project(item.ID, item.Type, item.Status)})
	}
	if eventErr != nil || (len(event.Item) > 0 && itemErr != nil) {
		return raw, nil
	}
	budget := 65536
	switch {
	case method == "item/fileChange/patchUpdated":
		return json.Marshal(map[string]any{"threadId": event.ThreadID, "turnId": event.TurnID, "itemId": event.ItemID, "changes": event.toolActivity.boundChanges(&budget).Changes})
	case item.Type == "webSearch" || item.Type == "fileChange":
		item.toolActivity = item.toolActivity.boundChanges(&budget)
		return json.Marshal(map[string]any{"threadId": event.ThreadID, "turnId": event.TurnID, "item": item})
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
