package bridge

import (
	"context"
	"encoding/json"
	"errors"
	"testing"
)

func TestTakeoverRequiresConfirmationAndWorkspaceBeforeRelease(t *testing.T) {
	for _, params := range []string{`{"thread_id":"session"}`, `{"thread_id":"session","confirm":false}`, `{"thread_id":"session","confirm":null}`, `{"thread_id":"session","confirm":"true"}`} {
		called := false
		_, err := Handle(context.Background(), "/project", Command{Method: "threads/takeover", Params: json.RawMessage(params)}, func(context.Context, string, any) (json.RawMessage, error) { called = true; return nil, nil })
		if err == nil || called {
			t.Fatalf("accepted unsafe takeover %s", params)
		}
	}
	methods := []string{}
	_, err := Handle(context.Background(), "/project", Command{Method: "threads/takeover", Params: json.RawMessage(`{"thread_id":"session","confirm":true}`)}, func(_ context.Context, method string, _ any) (json.RawMessage, error) {
		methods = append(methods, method)
		return json.RawMessage(`{"thread":{"id":"session","cwd":"/elsewhere"}}`), nil
	})
	if err == nil || len(methods) != 1 || methods[0] != "thread/read" {
		t.Fatalf("workspace check bypassed: %v %v", methods, err)
	}
}

func TestTakeoverReleaseBeforeSameIDResumeWithoutTurnReplay(t *testing.T) {
	for _, rejected := range []bool{false, true} {
		methods := []string{}
		result, err := Handle(context.Background(), "/project", Command{Method: "threads/takeover", Params: json.RawMessage(`{"thread_id":"session","confirm":true}`)}, func(_ context.Context, method string, params any) (json.RawMessage, error) {
			methods = append(methods, method)
			if params.(map[string]any)["threadId"] != "session" {
				t.Fatal("changed target")
			}
			if method == "client/takeover" {
				if rejected {
					return nil, errors.New("failed")
				}
				return json.RawMessage(`{"released":true}`), nil
			}
			return json.RawMessage(`{"thread":{"id":"session","cwd":"/project"}}`), nil
		})
		if rejected {
			if err == nil || len(methods) != 2 {
				t.Fatalf("resumed after failed release %v %v", methods, err)
			}
		} else if err != nil || result == nil || len(methods) != 3 || methods[0] != "thread/read" || methods[1] != "client/takeover" || methods[2] != "thread/resume" {
			t.Fatalf("invalid takeover %v %v", methods, err)
		}
	}
}
