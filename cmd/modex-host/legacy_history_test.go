package main

import (
	"encoding/json"
	"testing"
)

func TestLegacyHistoryPagesAndRejectsChangedCursor(t *testing.T) {
	raw := json.RawMessage(`{"thread":{"turns":[{"id":"old"},{"id":"middle"},{"id":"new"}]}}`)
	page, err := pageLegacyHistory(raw, map[string]any{"limit": 2})
	if err != nil {
		t.Fatal(err)
	}
	var result struct {
		Data []struct {
			ID string `json:"id"`
		} `json:"data"`
		Next string `json:"nextCursor"`
	}
	if json.Unmarshal(page, &result) != nil || len(result.Data) != 2 || result.Data[0].ID != "new" || result.Data[1].ID != "middle" || result.Next == "" {
		t.Fatalf("page=%s", page)
	}
	page, err = pageLegacyHistory(raw, map[string]any{"limit": 2, "cursor": result.Next})
	if err != nil {
		t.Fatal(err)
	}
	if json.Unmarshal(page, &result) != nil || len(result.Data) != 1 || result.Data[0].ID != "old" {
		t.Fatalf("page=%s", page)
	}
	if _, err = pageLegacyHistory(json.RawMessage(`{"thread":{"turns":[{"id":"changed"}]}}`), map[string]any{"cursor": result.Next}); err == nil {
		t.Fatal("changed history cursor accepted")
	}
}
