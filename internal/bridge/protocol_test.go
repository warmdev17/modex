package bridge

import (
	"encoding/json"
	"strings"
	"testing"
)

func TestDecodeCommandRequiresSupportedMethod(t *testing.T) {
	cmd, err := DecodeCommand([]byte(`{"id":7,"method":"threads/list","params":{"limit":5}}`))
	if err != nil || cmd.ID != 7 || cmd.Method != "threads/list" {
		t.Fatalf("command=%+v err=%v", cmd, err)
	}
	if _, err := DecodeCommand([]byte(`{"id":7,"method":"turn/interrupt"}`)); err == nil {
		t.Fatal("accepted unsupported mutating method")
	}
}

func TestEncodeErrorDoesNotEchoSensitiveDetails(t *testing.T) {
	b, err := EncodeError(3, "app-server rejected request; raw error suppressed")
	if err != nil || strings.Contains(string(b), "token") {
		t.Fatalf("response=%s err=%v", b, err)
	}
	var got map[string]any
	if json.Unmarshal(b, &got) != nil || got["id"] != float64(3) {
		t.Fatalf("bad response=%s", b)
	}
}
