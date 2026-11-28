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
	"slices"
	"strconv"
	"strings"
	"time"
	"unicode"

	"github.com/warmdev/modex/internal/bridge"
)

var (
	errCLICancelled    = errors.New("CLI selection cancelled")
	errCLIPickerBack   = errors.New("back to agent selection")
	errCLIPickerCursor = errors.New("repeated session list cursor")
)

type cliAgent struct {
	ID       string `json:"id"`
	Label    string `json:"label"`
	Reason   string `json:"reason"`
	Ready    bool   `json:"ready"`
	Checking bool   `json:"checking,omitempty"`
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
		lines = append(lines, fmt.Sprintf("%d  %s", i+1, agent.menuLabel()))
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
		page, err := parseScopedCLISessionPage(agent, raw, cwd)
		if err != nil {
			return cliChoice{}, err
		}
		lines := []string{"0  New conversation"}
		for i, session := range page.Data {
			lines = append(lines, fmt.Sprintf("%d  %s", i+1, session.menuLabel(cwd)))
		}
		footer := "b back · q quit"
		if page.Next != "" {
			footer = "n next · " + footer
		}
		if len(cursors) > 1 {
			footer = "p previous · " + footer
		}
		cliRenderFrame(p.out, "Sessions", agent.displayName()+" · "+cliLabel(cwd), lines, footer)
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
			if choice == "n" && page.Next != "" {
				if slices.Contains(cursors, page.Next) {
					return cliChoice{}, errCLIPickerCursor
				}
				cursors = append(cursors, page.Next)
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
				return page.Data[n-1].choice(cwd)
			}
			fmt.Fprintln(p.out, "Invalid choice; select one of the displayed options.")
		}
	}
}

func (a cliAgent) displayName() string {
	if label := cliLabel(a.Label); label != "" {
		return label
	}
	return cliLabel(a.ID)
}

func (a cliAgent) menuLabel() string {
	label := a.displayName()
	if a.Checking {
		return label + " · Checking…"
	}
	if a.Ready {
		return label
	}
	reason := cliLabel(a.Reason)
	if reason == "" {
		reason = "Unavailable"
	}
	return label + " — " + reason
}

func (a cliAgent) icon() string {
	locale := os.Getenv("LC_ALL")
	if locale == "" {
		locale = os.Getenv("LC_CTYPE")
	}
	if locale == "" {
		locale = os.Getenv("LANG")
	}
	plain := locale != "" && !strings.Contains(strings.ToUpper(locale), "UTF-8") && !strings.Contains(strings.ToUpper(locale), "UTF8")
	if os.Getenv("TERM") == "dumb" || plain {
		switch a.ID {
		case "codex":
			return "[C]"
		case "opencode":
			return "[O]"
		case "antigravity":
			return "[A]"
		}
	}
	switch a.ID {
	case "codex":
		return "◇"
	case "opencode":
		return "▣"
	case "antigravity":
		return "✦"
	}
	return "·"
}

func (a cliAgent) description() string {
	switch a.ID {
	case "codex":
		return "Codex terminal · shared native sessions"
	case "opencode":
		return "OpenCode terminal · managed local server"
	case "antigravity":
		return "Antigravity terminal · saved conversations"
	}
	return "Native agent terminal"
}

type cliSession struct{ ID, Name, Preview, CWD, Workspace string }

type cliSessionPage struct {
	Data []cliSession `json:"data"`
	Next string       `json:"next_cursor"`
}

func parseCLISessionPage(agent cliAgent, raw json.RawMessage) (cliSessionPage, error) {
	var page cliSessionPage
	if json.Unmarshal(raw, &page) != nil || page.Data == nil {
		return cliSessionPage{}, errors.New("invalid session list")
	}
	for _, session := range page.Data {
		if !validCLISessionID(agent.ID, session.ID) {
			return cliSessionPage{}, errors.New("invalid session ID in list")
		}
	}
	return page, nil
}

func parseScopedCLISessionPage(agent cliAgent, raw json.RawMessage, cwd string) (cliSessionPage, error) {
	page, err := parseCLISessionPage(agent, raw)
	if err != nil {
		return page, err
	}
	data := make([]cliSession, 0, len(page.Data))
	for _, session := range page.Data {
		workspace := session.workspace(cwd)
		if !validCLIWorkspace(workspace) {
			return cliSessionPage{}, errors.New("invalid session workspace")
		}
		if filepath.Clean(workspace) == filepath.Clean(cwd) {
			data = append(data, session)
		}
	}
	page.Data = data
	return page, nil
}

func (s cliSession) workspace(cwd string) string {
	if s.Workspace != "" {
		return s.Workspace
	}
	if s.CWD != "" {
		return s.CWD
	}
	return cwd
}

func (s cliSession) menuLabel(cwd string) string {
	title := cliLabel(s.Name)
	if title == "" {
		title = cliLabel(s.Preview)
	}
	if title == "" {
		title = "Untitled conversation"
	}
	summary := title + " · " + cliLabel(shortCLIID(s.ID))
	if workspace := s.workspace(cwd); workspace != cwd {
		summary += " · " + cliLabel(filepath.Base(workspace))
	}
	return summary
}

func (s cliSession) choice(cwd string) (cliChoice, error) {
	workspace := s.workspace(cwd)
	if !validCLIWorkspace(workspace) {
		return cliChoice{}, errors.New("invalid session workspace")
	}
	return cliChoice{ID: s.ID, Workspace: workspace}, nil
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
