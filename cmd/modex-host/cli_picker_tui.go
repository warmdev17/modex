package main

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"slices"

	tea "github.com/charmbracelet/bubbletea"
)

func pickCLIFullscreen(
	ctx context.Context, in, out *os.File,
	loadAgents func(context.Context) ([]cliAgent, error),
	cwd string,
	list func(context.Context, cliAgent, string) (json.RawMessage, error),
) (cliAgent, cliChoice, error) {
	if !validCLIWorkspace(cwd) {
		return cliAgent{}, cliChoice{}, errors.New("invalid CLI workspace")
	}
	ctx, cancel := context.WithCancel(ctx)
	defer cancel()
	picker := newCLIFullscreenPicker(ctx, out, cwd, loadAgents, list)
	_, err := tea.NewProgram(picker, tea.WithContext(ctx), tea.WithInput(in), tea.WithOutput(out),
		tea.WithAltScreen(), tea.WithoutBracketedPaste()).Run()
	if err != nil {
		return cliAgent{}, cliChoice{}, err
	}
	if picker.err != nil {
		return cliAgent{}, cliChoice{}, picker.err
	}
	if picker.choice.Workspace == "" {
		return cliAgent{}, cliChoice{}, errCLICancelled
	}
	return picker.agents[picker.agentIndex], picker.choice, nil
}

type cliFullscreenPicker struct {
	ctx        context.Context
	screen     cliScreen
	cwd        string
	loadAgents func(context.Context) ([]cliAgent, error)
	list       func(context.Context, cliAgent, string) (json.RawMessage, error)

	agents       []cliAgent
	agentIndex   int
	sessionIndex int
	cursors      []string
	page         cliSessionPage
	cancelPage   context.CancelFunc
	requestID    uint64

	choice cliChoice
	err    error
}

type cliPickerAgentsMsg struct {
	agents []cliAgent
	err    error
}

type cliPickerPageMsg struct {
	id   uint64
	page cliSessionPage
	err  error
}

func newCLIFullscreenPicker(
	ctx context.Context, out io.Writer, cwd string,
	loadAgents func(context.Context) ([]cliAgent, error),
	list func(context.Context, cliAgent, string) (json.RawMessage, error),
) *cliFullscreenPicker {
	return &cliFullscreenPicker{ctx: ctx, screen: newCLIScreen(out), cwd: cwd, loadAgents: loadAgents, list: list}
}

func (p *cliFullscreenPicker) Init() tea.Cmd {
	return func() tea.Msg {
		agents, err := p.loadAgents(p.ctx)
		return cliPickerAgentsMsg{agents, err}
	}
}

func (p *cliFullscreenPicker) loadPage() tea.Cmd {
	p.requestID++
	requestID := p.requestID
	agent, cursor := p.agents[p.agentIndex], p.cursors[len(p.cursors)-1]
	ctx, cancel := context.WithCancel(p.ctx)
	p.cancelPage = cancel
	p.sessionIndex = 0
	return func() tea.Msg {
		defer cancel()
		raw, err := p.list(ctx, agent, cursor)
		if err != nil {
			return cliPickerPageMsg{id: requestID, err: err}
		}
		page, err := parseCLISessionPage(agent, raw)
		return cliPickerPageMsg{id: requestID, page: page, err: err}
	}
}

func (p *cliFullscreenPicker) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	if p.err != nil || p.choice.Workspace != "" {
		return p, nil
	}
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		p.screen.width, p.screen.height = msg.Width, msg.Height
	case cliPickerAgentsMsg:
		if msg.err != nil {
			p.err = msg.err
			return p, tea.Quit
		}
		if len(msg.agents) == 0 {
			p.err = errors.New("the host returned no agent runtimes")
			return p, tea.Quit
		}
		p.agents = msg.agents
	case cliPickerPageMsg:
		if msg.id != p.requestID || len(p.cursors) == 0 {
			break
		}
		p.cancelPage = nil
		p.page, p.err = msg.page, msg.err
		if p.err != nil {
			return p, tea.Quit
		}
	case tea.KeyMsg:
		if msg.Type == tea.KeyCtrlC || (msg.Type == tea.KeyEsc && len(p.cursors) == 0) {
			p.err = errCLICancelled
			return p, tea.Quit
		}
		if len(p.cursors) != 0 {
			return p, p.sessionKey(msg.Type)
		}
		if len(p.agents) != 0 {
			return p, p.agentKey(msg.Type)
		}
	}
	return p, nil
}

func (p *cliFullscreenPicker) agentKey(key tea.KeyType) tea.Cmd {
	switch key {
	case tea.KeyUp:
		p.agentIndex = max(0, p.agentIndex-1)
	case tea.KeyDown:
		p.agentIndex = min(len(p.agents)-1, p.agentIndex+1)
	case tea.KeyEnter:
		if p.agents[p.agentIndex].Ready {
			p.cursors = []string{""}
			return p.loadPage()
		}
	}
	return nil
}

func (p *cliFullscreenPicker) sessionKey(key tea.KeyType) tea.Cmd {
	if key == tea.KeyEsc {
		p.cursors = nil
		if p.cancelPage != nil {
			p.cancelPage()
			p.cancelPage = nil
		}
		return nil
	}
	if p.cancelPage != nil {
		return nil
	}
	switch key {
	case tea.KeyUp:
		p.sessionIndex = max(0, p.sessionIndex-1)
	case tea.KeyDown:
		p.sessionIndex = min(len(p.page.Data), p.sessionIndex+1)
	case tea.KeyPgUp, tea.KeyLeft:
		if len(p.cursors) > 1 {
			p.cursors = p.cursors[:len(p.cursors)-1]
			return p.loadPage()
		}
	case tea.KeyPgDown, tea.KeyRight:
		if p.page.Next != "" {
			if slices.Contains(p.cursors, p.page.Next) {
				p.err = errCLIPickerCursor
				return tea.Quit
			}
			p.cursors = append(p.cursors, p.page.Next)
			return p.loadPage()
		}
	case tea.KeyEnter:
		if p.sessionIndex == 0 {
			p.choice = cliChoice{Workspace: p.cwd}
		} else {
			p.choice, p.err = p.page.Data[p.sessionIndex-1].choice(p.cwd)
		}
		return tea.Quit
	}
	return nil
}

func (p *cliFullscreenPicker) View() string {
	title, detail := "Modex / Choose an agent", cliLabel(p.cwd)
	footer := []cliScreenRow{{"↑↓ Move · Enter Select · Esc Quit", cliScreenMuted}}
	var rows []cliScreenRow
	index := p.agentIndex
	switch {
	case len(p.agents) == 0:
		rows = []cliScreenRow{{"Discovering agent runtimes…", cliScreenMuted}}
		footer[0].text = "Esc Cancel"
	case len(p.cursors) == 0:
		for _, agent := range p.agents {
			row := cliScreenRow{text: agent.menuLabel()}
			if !agent.Ready {
				row.tone = cliScreenMuted
			}
			rows = append(rows, row)
		}
	default:
		title = "Modex / Sessions"
		detail = p.agents[p.agentIndex].displayName() + " · " + detail
		footer[0].text = "↑↓ Move · Enter Select · Esc Back"
		index = p.sessionIndex
		if p.cancelPage != nil {
			rows = []cliScreenRow{{"Loading conversations…", cliScreenMuted}}
			footer[0].text = "Esc Back"
			break
		}
		rows = append(rows, cliScreenRow{text: "New conversation", tone: cliScreenAccent})
		for _, session := range p.page.Data {
			rows = append(rows, cliScreenRow{text: session.menuLabel(p.cwd)})
		}
		if len(p.cursors) > 1 || p.page.Next != "" {
			hint := fmt.Sprintf("Page %d", len(p.cursors))
			if len(p.cursors) > 1 {
				hint += " · PgUp Previous"
			}
			if p.page.Next != "" {
				hint += " · PgDn Next"
			}
			footer = append(footer, cliScreenRow{hint, cliScreenMuted})
		}
	}
	for i := range rows {
		if i == index && len(p.agents) != 0 && p.cancelPage == nil {
			rows[i].text = "› " + rows[i].text
			rows[i].tone = cliScreenSelected
		} else {
			rows[i].text = "  " + rows[i].text
		}
	}
	height := p.screen.bodyHeight(len(footer))
	start := max(0, index-height+1)
	return p.screen.render(title, detail, rows[start:min(len(rows), start+height)], footer)
}
