package main

import (
	"bufio"
	"context"
	"encoding/json"
	"errors"
	"io"
	"os"
	"strconv"
	"strings"
	"testing"
)

func TestCLIPickerChoosesReadyAgentAndExplainsUnavailable(t *testing.T) {
	var out strings.Builder
	p := newCLIPicker(bufio.NewReader(strings.NewReader("2\n1\n")), &out)
	agent, err := p.chooseAgent([]cliAgent{
		{ID: "codex", Label: "Codex", Ready: true},
		{ID: "antigravity", Label: "Antigravity", Ready: false, Reason: "Sign in first"},
	})
	if err != nil || agent.ID != "codex" {
		t.Fatalf("agent=%+v err=%v", agent, err)
	}
	if !strings.Contains(out.String(), "Sign in first") || strings.Contains(out.String(), "\x1b[") {
		t.Fatalf("missing reason or colored redirected output: %q", out.String())
	}
}

func TestCLIPickerChoosesRuntimeSessionWorkspace(t *testing.T) {
	for _, tc := range []struct {
		agent cliAgent
		raw   string
		want  cliChoice
	}{
		{cliAgent{ID: "codex"}, `{"data":[{"id":"01a1b731-2a1a-79e0-80fa-c6f24b2bd3b6","name":"Codex","cwd":"/code"}]}`, cliChoice{ID: "01a1b731-2a1a-79e0-80fa-c6f24b2bd3b6", Workspace: "/code"}},
		{cliAgent{ID: "antigravity"}, `{"data":[{"id":"agy:123","preview":"AGY"}]}`, cliChoice{ID: "agy:123", Workspace: "/fallback"}},
		{cliAgent{ID: "opencode"}, `{"data":[{"id":"oc:123","workspace":"/open"}]}`, cliChoice{ID: "oc:123", Workspace: "/open"}},
	} {
		p := newCLIPicker(bufio.NewReader(strings.NewReader("1\n")), io.Discard)
		got, err := p.chooseSession(context.Background(), tc.agent, "/fallback", func(context.Context, string) (json.RawMessage, error) {
			return json.RawMessage(tc.raw), nil
		})
		if err != nil || got != tc.want {
			t.Fatalf("%s: choice=%+v err=%v", tc.agent.ID, got, err)
		}
	}
}

func TestCLIPickerNewBackAndRuntimeValidation(t *testing.T) {
	list := func(raw string) func(context.Context, string) (json.RawMessage, error) {
		return func(context.Context, string) (json.RawMessage, error) { return json.RawMessage(raw), nil }
	}
	p := newCLIPicker(bufio.NewReader(strings.NewReader("0\n")), io.Discard)
	got, err := p.chooseSession(context.Background(), cliAgent{ID: "antigravity"}, "/work", list(`{"data":[]}`))
	if err != nil || got != (cliChoice{Workspace: "/work"}) {
		t.Fatalf("new=%+v err=%v", got, err)
	}
	p = newCLIPicker(bufio.NewReader(strings.NewReader("b\n")), io.Discard)
	_, err = p.chooseSession(context.Background(), cliAgent{ID: "antigravity"}, "/work", list(`{"data":[]}`))
	if !errors.Is(err, errCLIPickerBack) {
		t.Fatalf("back error=%v", err)
	}
	for _, tc := range []struct{ agent, id string }{{"codex", "agy:1"}, {"antigravity", "oc:1"}, {"opencode", "01a1b731-2a1a-79e0-80fa-c6f24b2bd3b6"}} {
		p = newCLIPicker(bufio.NewReader(strings.NewReader("1\n")), io.Discard)
		_, err = p.chooseSession(context.Background(), cliAgent{ID: tc.agent}, "/work", list(`{"data":[{"id":"`+tc.id+`"}]}`))
		if err == nil {
			t.Fatalf("%s accepted %s", tc.agent, tc.id)
		}
	}
}

func TestCLIRenderFrameSanitizesAndFitsNarrowOutput(t *testing.T) {
	t.Setenv("COLUMNS", "32")
	var out strings.Builder
	cliRenderFrame(&out, "Modex\x1b[31m", "工作区 / very long detail", []string{"→ Session 名称 with a very long suffix"}, "n next · q quit")
	if strings.ContainsRune(out.String(), '\x1b') || !strings.Contains(out.String(), "Modex [31m") {
		t.Fatalf("unsafe frame: %q", out.String())
	}
	for _, line := range strings.Split(strings.TrimSuffix(out.String(), "\n"), "\n") {
		if len([]rune(line)) > 32 {
			t.Fatalf("wide line %q", line)
		}
	}
}

func TestCLIFrameUsesExplicitRGBOnPTY(t *testing.T) {
	terminal := os.Stdout
	info, err := terminal.Stat()
	if err == nil && info.Mode()&os.ModeCharDevice == 0 {
		if tty, openErr := os.OpenFile("/dev/tty", os.O_WRONLY, 0); openErr == nil {
			terminal = tty
			defer tty.Close()
			info, err = terminal.Stat()
		}
	}
	if err != nil || info.Mode()&os.ModeCharDevice == 0 {
		t.Skip("stdout is not a terminal")
	}
	want := "137;180;250"
	if os.Getenv("COLORFGBG") == "0;15" {
		want = "37;86;140"
	}
	if got := cliFrameColor(terminal); got != want {
		t.Fatalf("RGB=%q want %q", got, want)
	}
	if value := os.Getenv("MODEX_EXPECT_COLUMNS"); value != "" {
		wantWidth, err := strconv.Atoi(value)
		if err != nil || cliTerminalWidth(terminal) != wantWidth {
			t.Fatalf("terminal width=%d want %s", cliTerminalWidth(terminal), value)
		}
	}
	cliRenderFrame(terminal, "Modex", "PTY capture", []string{"1  Session"}, "q quit")
}

func TestCLIFrameHonorsNoColor(t *testing.T) {
	t.Setenv("NO_COLOR", "1")
	if got := cliFrameColor(os.Stdout); got != "" {
		t.Fatalf("NO_COLOR produced %q", got)
	}
}

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
