package antigravity

import (
	"context"
	"encoding/json"
	"strings"
	"testing"

	"github.com/warmdev/modex/internal/bridge"
)

func TestNativeCLIReservesChatAndDisconnectRefreshesHistory(t *testing.T) {
	root, binary, state := stage3RouterFixture(t)
	router := New(context.Background(), binary, root, state)
	defer router.Close()
	value, _, err := router.Handle(context.Background(), bridge.Command{Method: "threads/start", Params: json.RawMessage(`{"agent":"antigravity"}`)}, nil)
	if err != nil {
		t.Fatal(err)
	}
	id := value.(map[string]any)["thread"].(map[string]any)["id"].(string)
	descriptor, err := router.NativeCLI(context.Background(), root, id)
	if err != nil || descriptor["session_id"] != "native" {
		t.Fatalf("native descriptor: %v %v", descriptor, err)
	}
	if _, err := router.NativeCLI(context.Background(), root, id); err == nil {
		t.Fatal("duplicate native handoff accepted")
	}
	params, _ := json.Marshal(map[string]string{"thread_id": id, "workspace": root, "text": "should not run"})
	_, _, err = router.Handle(context.Background(), bridge.Command{Method: "turn/start", Params: params}, nil)
	if err == nil || !strings.Contains(err.Error(), "native terminal") {
		t.Fatalf("concurrent ACP write: %v", err)
	}
	router.ReleaseNativeCLIs()
	params, _ = json.Marshal(map[string]string{"thread_id": id, "workspace": root})
	if _, _, err := router.Handle(context.Background(), bridge.Command{Method: "threads/resume", Params: params}, nil); err != nil {
		t.Fatalf("disconnect left stale session: %v", err)
	}
	if !router.chats[id].loaded {
		t.Fatal("history not reloaded after native exit")
	}
}
