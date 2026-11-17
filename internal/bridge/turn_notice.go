package bridge

import (
	"encoding/json"
	"errors"

	"github.com/warmdev/modex/internal/diagnostic"
)

// This boundary is mobile-only: native TUI clients retain the original event.
func mobileEventParams(method string, raw json.RawMessage) (json.RawMessage, error) {
	if method != "error" && method != "turn/completed" {
		return raw, nil
	}
	var params map[string]json.RawMessage
	if json.Unmarshal(raw, &params) != nil || params == nil {
		return nil, errors.New("invalid Codex event")
	}
	if method == "error" {
		var retry bool
		_ = json.Unmarshal(params["willRetry"], &retry)
		notice := diagnostic.Notice(params["error"], retry)
		// Error envelopes are projected, not copied: unknown error fields may contain credentials.
		projected := map[string]any{"notice": notice, "error": map[string]string{"message": notice.Message}, "willRetry": retry}
		for _, key := range []string{"threadId", "turnId"} {
			var id string
			if json.Unmarshal(params[key], &id) == nil {
				projected[key] = id
			}
		}
		return json.Marshal(projected)
	}
	var turn map[string]json.RawMessage
	if json.Unmarshal(params["turn"], &turn) != nil || turn == nil {
		return nil, errors.New("invalid completed turn")
	}
	if errRaw := turn["error"]; len(errRaw) > 0 && string(errRaw) != "null" {
		notice := diagnostic.Notice(errRaw, false)
		turn["notice"], _ = json.Marshal(notice)
		turn["error"], _ = json.Marshal(map[string]string{"message": notice.Message})
		params["turn"], _ = json.Marshal(turn)
	}
	return json.Marshal(params)
}
