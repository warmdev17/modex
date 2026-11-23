package diagnostic

import (
	"encoding/json"
	"strings"
)

// TurnNotice is the mobile-safe projection of an upstream turn error.
type TurnNotice struct {
	Category   string `json:"category"`
	Message    string `json:"message"`
	Retrying   bool   `json:"retrying"`
	HTTPStatus int    `json:"http_status,omitempty"`
}

func Notice(raw json.RawMessage, retrying bool) TurnNotice {
	notice := TurnNotice{Category: "turn_failed", Message: "Codex could not complete this turn. Review the conversation before trying again.", Retrying: retrying}
	var detail struct {
		Info    json.RawMessage `json:"codexErrorInfo"`
		Message string          `json:"message"`
	}
	_ = json.Unmarshal(raw, &detail)
	var kind string
	if json.Unmarshal(detail.Info, &kind) != nil {
		var variants map[string]json.RawMessage
		if json.Unmarshal(detail.Info, &variants) == nil && len(variants) == 1 {
			for key, value := range variants {
				kind = key
				var status struct {
					Code int `json:"httpStatusCode"`
				}
				if json.Unmarshal(value, &status) == nil && status.Code >= 100 && status.Code <= 599 {
					notice.HTTPStatus = status.Code
				}
			}
		}
	}
	switch kind {
	case "contextWindowExceeded":
		notice.Category = "context_window_exceeded"
		notice.Message = "This conversation exceeds the model context limit. Start a new conversation or compact it on the computer."
	case "sessionBudgetExceeded":
		notice.Category = "session_budget_exceeded"
		notice.Message = "This session has reached its budget limit."
	case "usageLimitExceeded":
		notice.Category = "usage_limit_exceeded"
		notice.Message = "Codex usage or quota limit reached. Check the provider account before trying again."
	case "rateLimitExceeded":
		notice.Category = "rate_limit_exceeded"
		notice.Message = "The provider is limiting requests. Wait before trying again."
	case "unauthorized":
		notice.Category = "unauthorized"
		notice.Message = "The provider rejected authentication or access. Check the account on the computer."
	case "modelNotFound":
		notice.Category = "model_not_found"
		notice.Message = "The selected model is unavailable or this account cannot access it."
	case "responseStreamDisconnected":
		notice.Category = "stream_disconnected"
		notice.Message = "The response stream disconnected before this turn finished."
	case "responseStreamConnectionFailed", "httpConnectionFailed":
		notice.Category = "stream_connection_failed"
		notice.Message = "Codex could not connect to the provider response stream."
	case "responseTooManyFailedAttempts":
		notice.Category = "retry_exhausted"
		notice.Message = "Codex exhausted its response retries. Review partial output before trying again."
	case "serverOverloaded", "flexUnavailable", "policyCapacityUnavailable":
		notice.Category = "provider_unavailable"
		notice.Message = "The provider is temporarily unavailable. Try again later."
	default:
		// Older servers omit structured codes; only recognize fixed diagnostic phrases.
		message := strings.ToLower(detail.Message)
		switch {
		case isUsageLimitMessage(message):
			notice.Category = "usage_limit_exceeded"
			notice.Message = "Codex usage or quota limit reached. Check Session Status for available reset times."
		case strings.Contains(message, "stream disconnected before completion"), strings.Contains(message, "stream closed before completion"), strings.Contains(message, "stream closed before completed"):
			notice.Category = "stream_disconnected"
			notice.Message = "The response stream disconnected before this turn finished."
		case strings.Contains(message, "retry limit"), strings.Contains(message, "retries exhausted"):
			notice.Category = "retry_exhausted"
			notice.Message = "Codex exhausted its response retries. Review partial output before trying again."
		}
		notice.HTTPStatus = 0
	}
	if notice.Category != "turn_failed" {
		switch notice.HTTPStatus {
		case 401, 403:
			notice.Category = "unauthorized"
			notice.Message = "The provider rejected authentication or access. Check the account on the computer."
		case 429:
			notice.Category = "rate_limit_exceeded"
			notice.Message = "The provider is limiting requests. Wait before trying again."
		}
	}
	if retrying {
		notice.Message += " Codex is retrying."
	}
	return notice
}

func isUsageLimitMessage(message string) bool {
	message = strings.ToLower(message)
	return strings.Contains(message, "hit your usage limit") || strings.Contains(message, "usage limit reached") ||
		strings.Contains(message, "usage limit exceeded") || strings.Contains(message, "insufficient_quota")
}
