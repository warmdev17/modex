package bridge

import (
	"context"
	"encoding/json"
	"io"
	"strings"
	"testing"
	"time"
)

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
