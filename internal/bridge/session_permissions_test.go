package bridge

import (
	"context"
	"encoding/json"
	"strings"
	"testing"
)

func TestSessionPermissionsUseHostProfiles(t *testing.T) {
	for _, tc := range []struct {
		profile string
		allowed bool
	}{{"team-safe", true}, {"blocked", false}} {
		t.Run(tc.profile, func(t *testing.T) {
			var updated bool
			cmd, err := DecodeCommand([]byte(`{"id":1,"method":"threads/permissions","params":{"thread_id":"t","profile":"` + tc.profile + `"}}`))
			if err != nil {
				t.Fatal(err)
			}
			_, err = Handle(context.Background(), "/work", cmd, func(_ context.Context, method string, input any) (json.RawMessage, error) {
				switch method {
				case "thread/read":
					return json.RawMessage(`{"thread":{"id":"t","cwd":"/work"}}`), nil
				case "modex/thread/status":
					return json.RawMessage(`{}`), nil
				case "permissionProfile/list":
					return json.RawMessage(`{"data":[{"id":"team-safe","allowed":true},{"id":"blocked","allowed":false}]}`), nil
				case "modex/thread/permissions":
					p := input.(map[string]any)
					if p["threadId"] != "t" || p["permissions"] != tc.profile || p["approvalPolicy"] != "on-request" {
						t.Fatalf("incorrect update: %v", p)
					}
					updated = true
					return json.RawMessage(`{}`), nil
				default:
					t.Fatalf("unexpected call %s", method)
					return nil, nil
				}
			})
			if tc.allowed && (err != nil || !updated) {
				t.Fatalf("allowed profile not updated: %v", err)
			}
			if !tc.allowed && (updated || err == nil || !strings.Contains(err.Error(), "unavailable")) {
				t.Fatalf("blocked profile: updated=%v err=%v", updated, err)
			}
		})
	}
}
