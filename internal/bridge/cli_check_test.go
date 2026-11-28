package bridge

import (
	"bufio"
	"context"
	"encoding/json"
	"io"
	"strings"
	"testing"
	"time"
)

func TestCLICheckLaneRejectsFourthWithoutBlockingOrdinaryCommand(t *testing.T) {
	in, writer := io.Pipe()
	out, output := io.Pipe()
	started := make(chan struct{}, 3)
	release := make(chan struct{})
	done := make(chan error, 1)
	frames := make(chan map[string]json.RawMessage, 8)
	go func() {
		scanner := bufio.NewScanner(out)
		for scanner.Scan() {
			var frame map[string]json.RawMessage
			if json.Unmarshal(scanner.Bytes(), &frame) == nil {
				frames <- frame
			}
		}
	}()
	go func() {
		done <- ServeCommands(context.Background(), in, output, func(_ context.Context, cmd Command, _ func(string, json.RawMessage)) (any, error) {
			if cmd.Method == "cli/agents/check" {
				started <- struct{}{}
				<-release
			}
			return map[string]bool{"ok": true}, nil
		})
	}()
	defer func() { close(release); writer.Close(); <-done; output.Close(); out.Close() }()
	for _, line := range []string{"{\"id\":1,\"method\":\"cli/agents/check\"}\n", "{\"id\":2,\"method\":\"cli/agents/check\"}\n", "{\"id\":3,\"method\":\"cli/agents/check\"}\n"} {
		io.WriteString(writer, line)
		select {
		case <-started:
		case <-time.After(time.Second):
			t.Fatal("probe did not run concurrently")
		}
	}
	io.WriteString(writer, "{\"id\":4,\"method\":\"cli/agents/check\"}\n")
	select {
	case frame := <-frames:
		if string(frame["id"]) != "4" || frame["error"] == nil {
			t.Fatalf("fourth probe not rejected: %v", frame)
		}
	case <-time.After(time.Second):
		t.Fatal("probe lane not bounded")
	}
	io.WriteString(writer, "{\"id\":5,\"method\":\"threads/list\"}\n")
	select {
	case frame := <-frames:
		if string(frame["id"]) != "5" || frame["result"] == nil {
			t.Fatalf("ordinary command failed: %v", frame)
		}
	case <-time.After(time.Second):
		t.Fatal("ordinary command blocked by full probe lane")
	}
}

func TestCLICheckDoesNotBlockCommands(t *testing.T) {
	in, writer := io.Pipe()
	started := make(chan struct{})
	release := make(chan struct{})
	ordinary := make(chan struct{})
	done := make(chan error, 1)
	go func() {
		done <- ServeCommands(context.Background(), in, io.Discard, func(_ context.Context, cmd Command, _ func(string, json.RawMessage)) (any, error) {
			if cmd.Method == "cli/agents/check" {
				close(started)
				<-release
			} else {
				close(ordinary)
			}
			return nil, nil
		})
	}()
	io.WriteString(writer, "{\"id\":1,\"method\":\"cli/agents/check\"}\n")
	select {
	case <-started:
	case <-time.After(time.Second):
		t.Fatal("check did not start")
	}
	io.WriteString(writer, "{\"id\":2,\"method\":\"threads/list\"}\n")
	select {
	case <-ordinary:
	case <-time.After(time.Second):
		close(release)
		writer.Close()
		<-done
		t.Fatal("ordinary command blocked")
	}
	close(release)
	writer.Close()
	if err := <-done; err != nil && !strings.Contains(err.Error(), "closed") {
		t.Fatal(err)
	}
}
