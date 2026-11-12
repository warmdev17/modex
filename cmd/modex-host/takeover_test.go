package main

import (
	"context"
	"encoding/json"
	"testing"
)

func TestTakeoverRequiresExactlyOneLoadedThread(t *testing.T) {
	for _, raw := range []string{`{}`, `{"data":[]}`, `{"data":["other"]}`, `{"data":["target","other"]}`, `{"data":["target"],"nextCursor":"more"}`} {
		if singleLoadedThread(json.RawMessage(raw), "target") {
			t.Fatalf("accepted %s", raw)
		}
	}
	if !singleLoadedThread(json.RawMessage(`{"data":["target"],"nextCursor":null}`), "target") {
		t.Fatal("rejected exact owner")
	}
}

func TestTakeoverUnmanagedNeverSignals(t *testing.T) {
	_, err := requestTakeover(context.Background(), t.TempDir(), "01a1b731-2a1a-79e0-80fa-c6f24b2bd3b6")
	if err == nil {
		t.Fatal("unregistered CLI accepted")
	}
}

func TestTakeoverRejectsPathThread(t *testing.T) {
	for _, id := range []string{"", "../other", "/tmp/target", "x\n", "01a1b731-2a1a-79e0-80fa-c6f24b2bd3--"} {
		if validTakeoverThread(id) {
			t.Fatalf("accepted %q", id)
		}
	}
	if !validTakeoverThread("01a1b731-2a1a-79e0-80fa-c6f24b2bd3b6") {
		t.Fatal("invalid UUID check")
	}
}
