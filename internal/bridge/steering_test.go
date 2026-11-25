package bridge

import (
	"context"
	"encoding/json"
	"errors"
	"reflect"
	"testing"

	"github.com/warmdev/modex/internal/diagnostic"
)

func TestSteerTurn(t *testing.T) {
	for _, scenario := range []string{"active", "completed", "changed turn", "uncertain", "wrong workspace", "missing turn"} {
		t.Run(scenario, func(t *testing.T) {
			var calls []string
			cwd, turn := "/work", "active"
			if scenario == "wrong workspace" {
				cwd = "/other"
			}
			if scenario == "missing turn" {
				turn = ""
			}
			cmd := Command{Method: "turn/steer", Params: json.RawMessage(`{"thread_id":"one","turn_id":"` + turn + `","text":"follow up","model":"next-model"}`)}
			_, err := Handle(context.Background(), "/work", cmd, func(_ context.Context, method string, raw any) (json.RawMessage, error) {
				calls = append(calls, method)
				params := raw.(map[string]any)
				switch method {
				case "thread/read":
					return json.RawMessage(`{"thread":{"id":"one","cwd":"` + cwd + `"}}`), nil
				case "turn/steer":
					expected := turn
					if scenario == "changed turn" && len(calls) == 3 {
						expected = "successor"
					}
					input := []map[string]string{{"type": "text", "text": "follow up"}}
					if params["threadId"] != "one" || params["expectedTurnId"] != expected || params["model"] != nil || !reflect.DeepEqual(params["input"], input) {
						t.Fatalf("invalid steer params: %#v", params)
					}
					switch scenario {
					case "changed turn":
						if expected == turn {
							return nil, diagnostic.NewRPCError(json.RawMessage("{\"message\":\"expected active turn id `active` but found `successor`\"}"))
						}
					case "completed":
						return nil, diagnostic.NewRPCError(json.RawMessage(`{"code":-32600,"message":"no active turn to steer"}`))
					case "uncertain":
						return nil, errors.New("transport failed")
					}
					return json.RawMessage(`{"turnId":"active"}`), nil
				case "turn/start":
					if params["model"] != "next-model" || params["expectedTurnId"] != nil {
						t.Fatalf("invalid fallback params: %#v", params)
					}
					return json.RawMessage(`{"turn":{"id":"next"}}`), nil
				}
				t.Fatalf("unexpected method %q", method)
				return nil, nil
			})
			var want []string
			switch scenario {
			case "active", "uncertain":
				want = []string{"thread/read", "turn/steer"}
			case "changed turn":
				want = []string{"thread/read", "turn/steer", "turn/steer"}
			case "completed":
				want = []string{"thread/read", "turn/steer", "turn/start"}
			case "wrong workspace":
				want = []string{"thread/read"}
			}
			if !reflect.DeepEqual(calls, want) || (err == nil) != (scenario == "active" || scenario == "completed" || scenario == "changed turn") {
				t.Fatalf("calls = %v, err = %v; want calls %v", calls, err, want)
			}
		})
	}
}
