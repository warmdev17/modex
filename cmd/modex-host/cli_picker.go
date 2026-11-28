package main

import (
	"bufio"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"time"
	"unicode"

	"github.com/warmdev/modex/internal/bridge"
)

var (
	errCLICancelled  = errors.New("CLI selection cancelled")
	errCLIPickerBack = errors.New("back to agent selection")
)

type cliAgent struct {
	ID     string `json:"id"`
	Label  string `json:"label"`
	Reason string `json:"reason"`
	Ready  bool   `json:"ready"`
}
type cliChoice struct{ ID, Workspace string }
type cliPicker struct {
	in  *bufio.Reader
	out io.Writer
}

func newCLIPicker(in *bufio.Reader, out io.Writer) *cliPicker { return &cliPicker{in: in, out: out} }

func (p *cliPicker) chooseAgent(agents []cliAgent) (cliAgent, error) {
	if len(agents) == 0 {
		return cliAgent{}, errors.New("the host returned no agent runtimes")
	}
	lines := make([]string, 0, len(agents))
	for i, agent := range agents {
		label := cliLabel(agent.Label)
		if label == "" {
			label = cliLabel(agent.ID)
		}
		line := fmt.Sprintf("%d  %s", i+1, label)
		if !agent.Ready {
			reason := cliLabel(agent.Reason)
			if reason == "" {
				reason = "Unavailable"
			}
			line += " — " + reason
		}
		lines = append(lines, line)
	}
	cliRenderFrame(p.out, "Modex", "Choose an agent", lines, "q quit")
	for {
		choice, err := p.readChoice()
		if err != nil || choice == "q" {
			if err != nil && !errors.Is(err, io.EOF) {
				return cliAgent{}, err
			}
			return cliAgent{}, errCLICancelled
		}
		n, err := strconv.Atoi(choice)
		if err == nil && n > 0 && n <= len(agents) && agents[n-1].Ready {
			return agents[n-1], nil
		}
		fmt.Fprintln(p.out, "Invalid choice; select an available agent.")
	}
}

func (p *cliPicker) chooseSession(ctx context.Context, agent cliAgent, cwd string, list func(context.Context, string) (json.RawMessage, error)) (cliChoice, error) {
	if !validCLIWorkspace(cwd) {
		return cliChoice{}, errors.New("invalid CLI workspace")
	}
	cursors := []string{""}
	for {
		raw, err := list(ctx, cursors[len(cursors)-1])
		if err != nil {
			return cliChoice{}, err
		}
		var page struct {
			Data []struct{ ID, Name, Preview, CWD, Workspace string } `json:"data"`
			Next *string                                              `json:"next_cursor"`
		}
		if json.Unmarshal(raw, &page) != nil || page.Data == nil {
			return cliChoice{}, errors.New("invalid session list")
		}
		lines := []string{"0  New conversation"}
		for i, session := range page.Data {
			if !validCLISessionID(agent.ID, session.ID) {
				return cliChoice{}, errors.New("invalid session ID in list")
			}
			title := cliLabel(session.Name)
			if title == "" {
				title = cliLabel(session.Preview)
			}
			if title == "" {
				title = "Untitled conversation"
			}
			summary := fmt.Sprintf("%d  %s · %s", i+1, title, shortCLIID(session.ID))
			workspace := session.Workspace
			if workspace == "" {
				workspace = session.CWD
			}
			if workspace != "" && workspace != cwd {
				summary += " · " + cliLabel(filepath.Base(workspace))
			}
			lines = append(lines, summary)
		}
		footer := "b back · q quit"
		if page.Next != nil && *page.Next != "" {
			footer = "n next · " + footer
		}
		if len(cursors) > 1 {
			footer = "p previous · " + footer
		}
		detail := cliLabel(agent.Label)
		if detail == "" {
			detail = cliLabel(agent.ID)
		}
		cliRenderFrame(p.out, "Sessions", detail+" · "+cliLabel(cwd), lines, footer)
		for {
			choice, err := p.readChoice()
			if err != nil || choice == "q" {
				if err != nil && !errors.Is(err, io.EOF) {
					return cliChoice{}, err
				}
				return cliChoice{}, errCLICancelled
			}
			if choice == "b" {
				return cliChoice{}, errCLIPickerBack
			}
			if choice == "n" && page.Next != nil && *page.Next != "" {
				for _, cursor := range cursors {
					if cursor == *page.Next {
						return cliChoice{}, errors.New("repeated session list cursor")
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
					return cliChoice{Workspace: cwd}, nil
				}
				session := page.Data[n-1]
				workspace := session.Workspace
				if workspace == "" {
					workspace = session.CWD
				}
				if workspace == "" {
					workspace = cwd
				}
				if !validCLIWorkspace(workspace) {
					return cliChoice{}, errors.New("invalid session workspace")
				}
				return cliChoice{ID: session.ID, Workspace: workspace}, nil
			}
			fmt.Fprintln(p.out, "Invalid choice; select one of the displayed options.")
		}
	}
}

func (p *cliPicker) readChoice() (string, error) {
	fmt.Fprint(p.out, "> ")
	line, err := p.in.ReadSlice('\n')
	if errors.Is(err, bufio.ErrBufferFull) {
		return "", errors.New("CLI selection is too long")
	}
	if len(line) > 1024 {
		return "", errors.New("CLI selection is too long")
	}
	if err != nil && len(line) == 0 {
		return "", err
	}
	return strings.ToLower(strings.TrimSpace(string(line))), nil
}

func validCLIWorkspace(value string) bool {
	return filepath.IsAbs(value) && len(value) <= 32768 && !strings.ContainsAny(value, "\r\n\x00")
}
func validCLISessionID(agent, id string) bool {
	switch agent {
	case "codex":
		return validTakeoverThread(id)
	case "antigravity":
		return strings.HasPrefix(id, "agy:") && validCLIIdentity(id)
	case "opencode":
		return strings.HasPrefix(id, "oc:") && validCLIIdentity(id)
	}
	return false
}
func validCLIIdentity(value string) bool {
	return len(value) > 3 && len(value) <= 4096 && strings.TrimSpace(value) == value && !strings.ContainsAny(value, "\r\n\x00")
}
func shortCLIID(id string) string {
	runes := []rune(id)
	if len(runes) > 8 {
		return string(runes[:8])
	}
	return id
}

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
	cwd, err := os.Getwd()
	if err != nil || !filepath.IsAbs(cwd) {
		cwd = string(filepath.Separator)
	}
	choice, err := newCLIPicker(bufio.NewReader(in), out).chooseSession(ctx, cliAgent{ID: "codex", Label: "Codex", Ready: true}, cwd, list)
	return choice.ID, err
}
