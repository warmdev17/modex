package diagnostic

import (
	"bufio"
	"bytes"
	"encoding/json"
	"errors"
	"strings"
	"testing"
)

func TestResponseSkipsNotificationAndMatchesID(t *testing.T) {
	input := `{"method":"status"}` + "\n" + `{"id":2,"result":{"ok":true}}` + "\n"
	raw, err := readResponse(bufio.NewReader(strings.NewReader(input)), 2)
	if err != nil || string(raw) != `{"ok":true}` {
		t.Fatalf("result=%s err=%v", raw, err)
	}
}

func TestResponseRejectsOversizedFrame(t *testing.T) {
	_, err := readResponse(bufio.NewReader(strings.NewReader(strings.Repeat("x", maxFrame+1))), 1)
	if err == nil {
		t.Fatal("accepted oversized frame")
	}
}

func TestResponseRejectsRPCErrorAndUnexpectedID(t *testing.T) {
	for _, input := range []string{`{"id":1,"error":{"code":-32600,"message":"secret"}}`, `{"id":9,"result":{}}`, `{bad}`} {
		_, err := readResponse(bufio.NewReader(strings.NewReader(input+"\n")), 1)
		if err == nil || strings.Contains(err.Error(), "secret") {
			t.Fatalf("invalid response handling: %v", err)
		}
	}
}

func TestCallEventsForwardsNotificationsBeforeResponse(t *testing.T) {
	var out bytes.Buffer
	var events []string
	input := bufio.NewReader(strings.NewReader(`{"method":"turn/started","params":{"turnId":"t1"}}
{"id":4,"result":{"turn":{"id":"t1"}}}
`))
	result, err := readResponseEvents(input, 4, func(method string, params json.RawMessage) { events = append(events, method+":"+string(params)) })
	if err != nil || string(result) != `{"turn":{"id":"t1"}}` || len(events) != 1 || events[0] != `turn/started:{"turnId":"t1"}` {
		t.Fatalf("result=%s events=%v err=%v", result, events, err)
	}
	_ = out
}

func TestCallEventsPreservesRPCErrorCategory(t *testing.T) {
	_, err := readResponseEvents(bufio.NewReader(strings.NewReader(`{"id":4,"error":{"code":"active_writer","message":"thread already has an active writer"}}`+"\n")), 4, nil)
	var rpc *RPCError
	if !errors.As(err, &rpc) || rpc.Category() != "active_writer" {
		t.Fatalf("err=%v", err)
	}
}

func TestRPCErrorClassifiesSafeCategories(t *testing.T) {
	err := NewRPCError(json.RawMessage(`{"code":"active_writer","message":"thread already has an active writer; secret-key"}`))
	if err.Category() != "active_writer" || strings.Contains(err.Error(), "secret-key") {
		t.Fatalf("category=%s error=%s", err.Category(), err)
	}
}
