package main

import (
	"context"
	"testing"
)

func TestInterruptNeverAcquiresMissingOrStaleWorker(t *testing.T) {
	s := newThreadProcesses(context.Background(), nil, nil)
	if _, err := s.Call(context.Background(), "turn/interrupt", map[string]any{"threadId": "missing", "turnId": "t"}); err == nil {
		t.Fatal("accepted missing owner")
	}
	s.workers["one"] = &threadWorker{busy: true, turn: "new-turn"}
	if _, err := s.Call(context.Background(), "turn/interrupt", map[string]any{"threadId": "one", "turnId": "old-turn"}); err == nil {
		t.Fatal("accepted stale turn")
	}
	if len(s.workers) != 1 {
		t.Fatal("interrupt created a writer")
	}
}
