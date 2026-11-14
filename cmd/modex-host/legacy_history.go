package main

import (
	"context"
	"crypto/sha256"
	"encoding/json"
	"errors"
	"fmt"
	"strconv"
	"strings"
)

func (s *sharedBackend) legacyHistory(ctx context.Context, params map[string]any) (json.RawMessage, error) {
	raw, err := s.p.CallPersistent(ctx, "thread/read", map[string]any{"threadId": params["threadId"], "includeTurns": true})
	if err != nil {
		return nil, err
	}
	return pageLegacyHistory(raw, params)
}

// Legacy JSONL history is read in one bounded upstream frame. Native paginated
// stores continue to use their own cursors without loading the whole history.
func pageLegacyHistory(raw json.RawMessage, params map[string]any) (json.RawMessage, error) {
	var result struct {
		Thread struct {
			Turns []json.RawMessage `json:"turns"`
		} `json:"thread"`
	}
	if json.Unmarshal(raw, &result) != nil || result.Thread.Turns == nil {
		return nil, errors.New("invalid legacy history")
	}
	limit := 10
	if v, ok := params["limit"]; ok {
		switch n := v.(type) {
		case int:
			limit = n
		case float64:
			limit = int(n)
		}
	}
	if limit < 1 || limit > 100 {
		return nil, errors.New("invalid legacy history limit")
	}
	direction, _ := params["sortDirection"].(string)
	if direction == "" {
		direction = "desc"
	}
	if direction != "desc" && direction != "asc" {
		return nil, errors.New("invalid history direction")
	}
	turns := result.Thread.Turns
	sum := sha256.Sum256(raw)
	prefix := fmt.Sprintf("modex-legacy-%x-%s:", sum[:8], direction)
	offset := 0
	if cursor, _ := params["cursor"].(string); cursor != "" {
		if !strings.HasPrefix(cursor, prefix) {
			return nil, errors.New("legacy history changed; reload latest page")
		}
		var err error
		offset, err = strconv.Atoi(strings.TrimPrefix(cursor, prefix))
		if err != nil || offset < 0 || offset > len(turns) {
			return nil, errors.New("invalid history cursor")
		}
	}
	end := offset + limit
	if end > len(turns) {
		end = len(turns)
	}
	page := make([]json.RawMessage, 0, end-offset)
	for i := offset; i < end; i++ {
		index := i
		if direction == "desc" {
			index = len(turns) - 1 - i
		}
		page = append(page, turns[index])
	}
	var next any
	if end < len(turns) {
		next = prefix + strconv.Itoa(end)
	}
	return json.Marshal(map[string]any{"data": page, "nextCursor": next})
}
