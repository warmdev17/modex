package bridge

import (
	"encoding/json"
	"errors"

	"github.com/warmdev/modex/internal/diagnostic"
)

// This boundary is mobile-only: native TUI clients retain the original event.
func mobileEventParams(method string, raw json.RawMessage) (json.RawMessage, error) {
	if method == "item/started" || method == "item/completed" || method == "item/fileChange/patchUpdated" {
		return mobileActivityParams(method, raw)
	}
	if method == "thread/settings/updated" || method == "thread/tokenUsage/updated" || method == "account/rateLimits/updated" {
		var event struct {
			ThreadID string          `json:"threadId"`
			TurnID   string          `json:"turnId"`
			Settings json.RawMessage `json:"threadSettings"`
			Usage    json.RawMessage `json:"tokenUsage"`
			Limits   json.RawMessage `json:"rateLimits"`
		}
		if json.Unmarshal(raw, &event) != nil {
			return nil, errors.New("invalid Codex event")
		}
		switch method {
		case "thread/settings/updated":
			var native struct {
				Mode struct {
					Mode string `json:"mode"`
				} `json:"collaborationMode"`
				Model    string          `json:"model"`
				Effort   *string         `json:"effort"`
				Approval json.RawMessage `json:"approvalPolicy"`
				Reviewer string          `json:"approvalsReviewer"`
				Sandbox  json.RawMessage `json:"sandboxPolicy"`
				Profile  json.RawMessage `json:"activePermissionProfile"`
			}
			if json.Unmarshal(event.Settings, &native) != nil {
				return nil, errors.New("invalid settings event")
			}
			raw, _ := json.Marshal(map[string]any{"approvalPolicy": native.Approval, "approvalsReviewer": native.Reviewer, "sandbox": native.Sandbox, "activePermissionProfile": native.Profile})
			safe, err := projectSession(raw)
			if err != nil {
				return nil, err
			}
			return json.Marshal(map[string]any{"threadId": event.ThreadID, "threadSettings": map[string]any{"collaborationMode": map[string]string{"mode": native.Mode.Mode}, "model": native.Model, "effort": native.Effort, "permission_profile": safe.Profile, "sandbox": safe.Sandbox, "approval_policy": safe.Approval, "approvals_reviewer": safe.Reviewer}})
		case "thread/tokenUsage/updated":
			return json.Marshal(map[string]any{"threadId": event.ThreadID, "turnId": event.TurnID, "tokenUsage": SafeUsage(event.Usage)})
		default:
			return json.Marshal(map[string]any{"rateLimits": safeRateLimits(append(append([]byte(`{"rateLimits":`), event.Limits...), '}'))})
		}
	}
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
