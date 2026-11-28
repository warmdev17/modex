package bridge

import (
	"encoding/json"
	"errors"
	"strings"
)

type nativeGoal struct {
	ThreadID        string `json:"threadId"`
	Objective       string `json:"objective"`
	Status          string `json:"status"`
	TokenBudget     *int64 `json:"tokenBudget"`
	TokensUsed      int64  `json:"tokensUsed"`
	TimeUsedSeconds int64  `json:"timeUsedSeconds"`
	CreatedAt       int64  `json:"createdAt"`
	UpdatedAt       int64  `json:"updatedAt"`
}
type goal struct {
	ThreadID        string `json:"thread_id"`
	Objective       string `json:"objective"`
	Status          string `json:"status"`
	TokenBudget     *int64 `json:"token_budget"`
	TokensUsed      int64  `json:"tokens_used"`
	TimeUsedSeconds int64  `json:"time_used_seconds"`
	CreatedAt       int64  `json:"created_at"`
	UpdatedAt       int64  `json:"updated_at"`
}

func projectGoal(raw json.RawMessage, id string) (*goal, error) {
	if string(raw) == "null" {
		return nil, nil
	}
	var fields map[string]json.RawMessage
	var g nativeGoal
	if json.Unmarshal(raw, &fields) != nil || fields == nil || json.Unmarshal(raw, &g) != nil || g.ThreadID != id {
		return nil, errors.New("invalid goal response")
	}
	for _, key := range []string{"threadId", "objective", "status", "tokensUsed", "timeUsedSeconds", "createdAt", "updatedAt"} {
		if len(fields[key]) == 0 || string(fields[key]) == "null" {
			return nil, errors.New("invalid goal response")
		}
	}
	switch g.Status {
	case "active", "paused", "blocked", "usageLimited", "budgetLimited", "complete":
	default:
		return nil, errors.New("invalid goal response")
	}
	return &goal{g.ThreadID, g.Objective, g.Status, g.TokenBudget, g.TokensUsed, g.TimeUsedSeconds, g.CreatedAt, g.UpdatedAt}, nil
}
func threadGoal(method string, p map[string]json.RawMessage, cwd string, fetch func(string, any) (json.RawMessage, error)) (any, error) {
	var id string
	if json.Unmarshal(p["thread_id"], &id) != nil || strings.TrimSpace(id) == "" || len(id) > 256 || strings.ContainsAny(id, "\r\n\x00") {
		return nil, errors.New("invalid thread_id")
	}
	params := map[string]any{"threadId": id}
	if method == "threads/goal/set" {
		if len(p["objective"]) == 0 && len(p["status"]) == 0 && len(p["token_budget"]) == 0 {
			return nil, errors.New("goal update is empty")
		}
		if raw, ok := p["objective"]; ok {
			var v string
			if json.Unmarshal(raw, &v) != nil || strings.TrimSpace(v) == "" || len(v) > 32768 || strings.ContainsRune(v, '\x00') {
				return nil, errors.New("invalid goal objective")
			}
			params["objective"] = v
		}
		if raw, ok := p["status"]; ok {
			var v string
			if json.Unmarshal(raw, &v) != nil || (v != "active" && v != "paused") {
				return nil, errors.New("invalid goal status")
			}
			params["status"] = v
		}
		if raw, ok := p["token_budget"]; ok {
			var v int64
			if json.Unmarshal(raw, &v) != nil || v <= 0 {
				return nil, errors.New("invalid token budget")
			}
			params["tokenBudget"] = v
		}
	}
	if method != "threads/goal/get" {
		params["origin"] = "user"
	}
	raw, err := fetch("thread/read", map[string]any{"threadId": id, "includeTurns": false})
	if err != nil {
		return nil, err
	}
	var thread struct {
		Thread Thread `json:"thread"`
	}
	if json.Unmarshal(raw, &thread) != nil || thread.Thread.ID != id {
		return nil, errors.New("invalid thread response")
	}
	if thread.Thread.Cwd != cwd {
		return nil, errors.New("thread outside workspace")
	}
	raw, err = fetch(strings.Replace(method, "threads/", "thread/", 1), params)
	if err != nil {
		return nil, err
	}
	var response map[string]json.RawMessage
	if json.Unmarshal(raw, &response) != nil || response == nil {
		return nil, errors.New("invalid goal response; refresh before retrying")
	}
	if method == "threads/goal/clear" {
		var cleared bool
		if json.Unmarshal(response["cleared"], &cleared) != nil || string(response["cleared"]) == "null" {
			return nil, errors.New("invalid goal response; refresh before retrying")
		}
		return map[string]any{"cleared": cleared}, nil
	}
	value, err := projectGoal(response["goal"], id)
	if err != nil || (method == "threads/goal/set" && value == nil) {
		return nil, errors.New("invalid goal response; refresh before retrying")
	}
	return map[string]any{"goal": value}, nil
}
