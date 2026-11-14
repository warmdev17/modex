package bridge

import (
	"crypto/sha256"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
)

func historyPageOptions(p map[string]json.RawMessage, include bool) (int, string, error) {
	raw, paged := p["limit"]
	cursorRaw, hasCursor := p["cursor"]
	if !paged {
		if hasCursor {
			return 0, "", errors.New("history cursor requires limit")
		}
		return 0, "", nil
	}
	var limit int
	if !include || string(raw) == "null" || json.Unmarshal(raw, &limit) != nil || limit < 1 || limit > 50 {
		return 0, "", errors.New("history limit must be 1..50 with include_turns")
	}
	cursor := ""
	if hasCursor && (string(cursorRaw) == "null" || json.Unmarshal(cursorRaw, &cursor) != nil || len(cursor) > 8192 || strings.ContainsAny(cursor, "\r\n\x00")) {
		return 0, "", errors.New("invalid history cursor")
	}
	return limit, cursor, nil
}

func fetchHistoryPage(fetch func(string, any) (json.RawMessage, error), cwd, id string, limit int, cursor string) (json.RawMessage, string, string, error) {
	meta, err := fetch("thread/read", map[string]any{"threadId": id, "includeTurns": false})
	if err != nil {
		return nil, "", "", err
	}
	var thread struct {
		Thread Thread `json:"thread"`
	}
	if json.Unmarshal(meta, &thread) != nil || thread.Thread.ID != id {
		return nil, "", "", errors.New("invalid thread response")
	}
	if thread.Thread.Cwd != cwd {
		return nil, "", "", errors.New("thread outside workspace")
	}
	params := map[string]any{"threadId": id, "limit": limit, "itemsView": "full", "sortDirection": "desc"}
	if cursor != "" {
		params["cursor"] = cursor
	}
	raw, err := fetch("thread/turns/list", params)
	if err != nil {
		return nil, "", "", err
	}
	var page struct {
		Data []json.RawMessage `json:"data"`
		Next string            `json:"nextCursor"`
	}
	if json.Unmarshal(raw, &page) != nil || page.Data == nil {
		return nil, "", "", errors.New("invalid history page")
	}
	if page.Next != "" && page.Next == cursor {
		return nil, "", "", errors.New("history cursor did not advance")
	}
	for i, j := 0, len(page.Data)-1; i < j; i, j = i+1, j-1 {
		page.Data[i], page.Data[j] = page.Data[j], page.Data[i]
	}
	result, err := json.Marshal(map[string]any{"thread": struct {
		Thread
		Turns []json.RawMessage `json:"turns"`
	}{thread.Thread, page.Data}})
	sum := sha256.Sum256(append(append([]byte(nil), meta...), raw...))
	return result, page.Next, fmt.Sprintf("%x", sum), err
}
