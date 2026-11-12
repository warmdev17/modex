package main

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"strings"
	"testing"
)

func TestCLIPickerNavigationAndTitle(t *testing.T) {
	const id = "01a1b731-2a1a-79e0-80fa-c6f24b2bd3b6"
	var out strings.Builder
	var cursors []string
	list := func(_ context.Context, cursor string) (json.RawMessage, error) {
		cursors = append(cursors, cursor)
		if cursor == "next" {
			return json.RawMessage(`{"data":[{"id":"` + id + `","name":"Second"}],"next_cursor":null}`), nil
		}
		return json.RawMessage(`{"data":[{"id":"` + id + `","name":"Named\u001b[31m\n session","preview":"first prompt"}],"next_cursor":"next"}`), nil
	}
	selected, err := pickCLIThread(context.Background(), strings.NewReader("bad\n2\nn\np\n1\n"), &out, list)
	if err != nil || selected != id {
		t.Fatalf("selection %q: %v", selected, err)
	}
	if strings.Join(cursors, ",") != ",next," || !strings.Contains(out.String(), "Named") || strings.Contains(out.String(), "first prompt") || strings.ContainsRune(out.String(), '\x1b') {
		t.Fatalf("unsafe title or incorrect pagination: %q %q", cursors, out.String())
	}
}

func TestCLIPickerNewCancelAndFailure(t *testing.T) {
	for _, tc := range []struct {
		input, data string
		wantErr     bool
	}{
		{"0\n", `{"data":[],"next_cursor":null}`, false},
		{"q\n", `{"data":[],"next_cursor":null}`, true},
		{"", `{"data":[],"next_cursor":null}`, true},
		{"1\n", `{"data":[{"id":"invalid","name":"Bad"}]}`, true},
		{"0\n", `{"data":null}`, true},
		{strings.Repeat("1", 1025), `{"data":[]}`, true},
	} {
		t.Run(tc.input+tc.data, func(t *testing.T) {
			id, err := pickCLIThread(context.Background(), strings.NewReader(tc.input), io.Discard, func(context.Context, string) (json.RawMessage, error) { return json.RawMessage(tc.data), nil })
			if id != "" || (err != nil) != tc.wantErr {
				t.Fatalf("id %q, error %v", id, err)
			}
		})
	}
	_, err := pickCLIThread(context.Background(), strings.NewReader("0\n"), io.Discard, func(context.Context, string) (json.RawMessage, error) { return nil, errors.New("list failed") })
	if err == nil {
		t.Fatal("list error became a new session")
	}
}

func TestCLIPickerRefusesCursorLoop(t *testing.T) {
	_, err := pickCLIThread(context.Background(), strings.NewReader("n\nn\n"), io.Discard, func(context.Context, string) (json.RawMessage, error) {
		return json.RawMessage(`{"data":[],"next_cursor":"repeat"}`), nil
	})
	if err == nil {
		t.Fatal("cursor loop accepted")
	}
}
