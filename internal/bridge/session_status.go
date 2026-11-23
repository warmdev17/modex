package bridge

import (
	"encoding/json"
	"errors"
)

var ErrSessionNotOwned = errors.New("session_not_owned: reload session")
var ErrSessionBusy = errors.New("session busy or pending decision")

type usageBreakdown struct {
	Total      *int64 `json:"totalTokens"`
	Input      *int64 `json:"inputTokens"`
	Cached     *int64 `json:"cachedInputTokens"`
	Output     *int64 `json:"outputTokens"`
	Reasoning  *int64 `json:"reasoningOutputTokens"`
	CacheWrite *int64 `json:"cacheWriteInputTokens,omitempty"`
}
type tokenUsage struct {
	Last   *usageBreakdown `json:"last"`
	Total  *usageBreakdown `json:"total"`
	Window *int64          `json:"modelContextWindow"`
}
type rateWindow struct {
	Used     *int64 `json:"usedPercent"`
	Duration *int64 `json:"windowDurationMins"`
	Resets   *int64 `json:"resetsAt"`
}
type rateLimits struct {
	ID        *string     `json:"limitId"`
	Name      *string     `json:"limitName"`
	Plan      *string     `json:"planType"`
	Primary   *rateWindow `json:"primary"`
	Secondary *rateWindow `json:"secondary"`
	Reached   string      `json:"rateLimitReachedType,omitempty"`
}

func safeRateLimits(raw json.RawMessage) any {
	var v struct {
		Limits *rateLimits `json:"rateLimits"`
	}
	if json.Unmarshal(raw, &v) != nil {
		return nil
	}
	if v.Limits != nil {
		switch v.Limits.Reached {
		case "rate_limit_reached", "workspace_owner_credits_depleted", "workspace_member_credits_depleted",
			"workspace_owner_usage_limit_reached", "workspace_member_usage_limit_reached":
		default:
			v.Limits.Reached = ""
		}
	}
	return v.Limits
}

// SafeSession projects native session settings without exposing policy scopes or instructions.
func SafeSession(raw json.RawMessage) (json.RawMessage, error) {
	v, e := projectSession(raw)
	if e != nil {
		return nil, e
	}
	return json.Marshal(v)
}

// SafeUsage returns only numeric token metadata.
func SafeUsage(raw json.RawMessage) json.RawMessage {
	if len(raw) == 0 || string(raw) == "null" {
		return nil
	}
	var v tokenUsage
	if json.Unmarshal(raw, &v) != nil {
		return nil
	}
	b, _ := json.Marshal(v)
	return b
}
