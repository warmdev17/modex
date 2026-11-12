//go:build !windows

package main

import (
	"context"
	"encoding/json"
	"errors"
	"testing"
)

func TestWaitCLIThreadDiscoversNativeID(t *testing.T) {
	const id = "01a1b731-2a1a-79e0-80fa-c6f24b2bd3b6"
	count := 0
	got, err := waitCLIThread(context.Background(), make(chan struct{}), func(context.Context) (json.RawMessage, error) {
		count++
		if count == 1 {
			return json.RawMessage(`{"data":[]}`), nil
		}
		return json.RawMessage(`{"data":["` + id + `"]}`), nil
	})
	if err != nil || got != id {
		t.Fatalf("new native ID %q: %v", got, err)
	}
}

func TestWaitCLIThreadRefusesAmbiguousOrInvalidSession(t *testing.T) {
	for _, raw := range []string{`{"data":["invalid"]}`, `{"data":["a","b"]}`, `{"data":[],"nextCursor":"more"}`, `{"data":null}`, `{}`} {
		_, err := waitCLIThread(context.Background(), make(chan struct{}), func(context.Context) (json.RawMessage, error) { return json.RawMessage(raw), nil })
		if err == nil {
			t.Fatalf("accepted %s", raw)
		}
	}
	done := make(chan struct{})
	close(done)
	id, err := waitCLIThread(context.Background(), done, func(context.Context) (json.RawMessage, error) { t.Fatal("inspected exited CLI"); return nil, nil })
	if err != nil || id != "" {
		t.Fatalf("exited CLI: %q %v", id, err)
	}
}

func TestWaitCLIThreadCancellationAndObserverFailure(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	_, err := waitCLIThread(ctx, make(chan struct{}), func(context.Context) (json.RawMessage, error) {
		t.Fatal("inspected after cancellation")
		return nil, nil
	})
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("cancellation lost: %v", err)
	}
	_, err = waitCLIThread(context.Background(), make(chan struct{}), func(context.Context) (json.RawMessage, error) { return nil, errors.New("offline") })
	if err == nil {
		t.Fatal("observer failure became a new session")
	}
}
