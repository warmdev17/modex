package main

import (
	"bufio"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"runtime"
	"strconv"
	"strings"
	"time"
	"unicode"

	"github.com/warmdev/modex/internal/bridge"
)

var errCLICancelled = errors.New("CLI selection cancelled")

func chooseCLIThread(cwd, binary, home string) (string, error) {
	if runtime.GOOS == "windows" {
		return "", errors.New("registered CLI takeover requires Linux or macOS")
	}
	env := os.Environ()
	if home != "" {
		env = append(env, "CODEX_HOME="+home)
	}
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	p, err := bridge.StartProcess(ctx, binary, []string{"app-server", "--listen", "stdio://"}, cwd, env)
	if err != nil {
		return "", err
	}
	defer p.CloseIdle()
	initCtx, stop := context.WithTimeout(ctx, 20*time.Second)
	err = p.Initialize(initCtx)
	stop()
	if err != nil {
		return "", err
	}
	fmt.Fprintln(os.Stdout, "Modex · "+cliLabel(cwd))
	return pickCLIThread(ctx, os.Stdin, os.Stdout, func(ctx context.Context, cursor string) (json.RawMessage, error) {
		ctx, stop := context.WithTimeout(ctx, 20*time.Second)
		defer stop()
		params, _ := json.Marshal(map[string]any{"limit": 20, "cursor": cursor})
		page, err := bridge.Handle(ctx, cwd, bridge.Command{Method: "threads/list", Params: params}, p.Call)
		if err != nil {
			return nil, errors.New("cannot list Codex sessions; check the Codex executable and configuration")
		}
		return json.Marshal(page)
	})
}

func cliLabel(value string) string {
	value = strings.Map(func(r rune) rune {
		if unicode.IsControl(r) || unicode.In(r, unicode.Cf, unicode.Zl, unicode.Zp) {
			return ' '
		}
		return r
	}, value)
	runes := []rune(strings.TrimSpace(value))
	if len(runes) > 100 {
		return string(runes[:100]) + "…"
	}
	return string(runes)
}

func pickCLIThread(ctx context.Context, in io.Reader, out io.Writer, list func(context.Context, string) (json.RawMessage, error)) (string, error) {
	input := bufio.NewScanner(in)
	input.Buffer(make([]byte, 1024), 1024)
	cursors := []string{""}
	for {
		raw, err := list(ctx, cursors[len(cursors)-1])
		if err != nil {
			return "", err
		}
		var page struct {
			Data []struct{ ID, Name, Preview string } `json:"data"`
			Next *string                              `json:"next_cursor"`
		}
		if json.Unmarshal(raw, &page) != nil || page.Data == nil {
			return "", errors.New("invalid session list")
		}
		fmt.Fprintln(out, "\n  0  New conversation")
		for i, thread := range page.Data {
			if !validTakeoverThread(thread.ID) {
				return "", errors.New("invalid session ID in list")
			}
			title := cliLabel(thread.Name)
			if title == "" {
				title = cliLabel(thread.Preview)
			}
			if title == "" {
				title = "Untitled conversation"
			}
			fmt.Fprintf(out, "%3d  %s · %s\n", i+1, title, thread.ID[:8])
		}
		fmt.Fprint(out, "\nChoose a number")
		if page.Next != nil && *page.Next != "" {
			fmt.Fprint(out, " · n next")
		}
		if len(cursors) > 1 {
			fmt.Fprint(out, " · p previous")
		}
		fmt.Fprintln(out, " · q quit")
		for {
			fmt.Fprint(out, "> ")
			if !input.Scan() {
				if err := input.Err(); err != nil {
					return "", err
				}
				return "", errCLICancelled
			}
			choice := strings.ToLower(strings.TrimSpace(input.Text()))
			if choice == "q" {
				return "", errCLICancelled
			}
			if choice == "n" && page.Next != nil && *page.Next != "" {
				for _, cursor := range cursors {
					if cursor == *page.Next {
						return "", errors.New("repeated session list cursor")
					}
				}
				cursors = append(cursors, *page.Next)
				break
			}
			if choice == "p" && len(cursors) > 1 {
				cursors = cursors[:len(cursors)-1]
				break
			}
			n, err := strconv.Atoi(choice)
			if err == nil && n >= 0 && n <= len(page.Data) {
				if n == 0 {
					return "", nil
				}
				return page.Data[n-1].ID, nil
			}
			fmt.Fprintln(out, "Invalid choice; select one of the displayed options.")
		}
	}
}
