package main

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"slices"
	"strings"

	tea "github.com/charmbracelet/bubbletea"
)

func pickCLIFullscreen(
	ctx context.Context, in, out *os.File,
	loadAgents func(context.Context) ([]cliAgent, error),
	cwd string,
	list func(context.Context, cliAgent, string) (json.RawMessage, error),
	checkAgent func(context.Context, string) (cliAgent, error),
) (cliAgent, cliChoice, error) {
	if !validCLIWorkspace(cwd) {
		return cliAgent{}, cliChoice{}, errors.New("invalid CLI workspace")
	}
	ctx, cancel := context.WithCancel(ctx)
	defer cancel()
	picker := newCLIFullscreenPicker(ctx, out, cwd, loadAgents, list)
	picker.checkAgent = checkAgent
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
	checkAgent   func(context.Context, string) (cliAgent, error)
	pages        map[string]cliSessionPage
	pageError    string

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
type cliPickerAgentMsg struct {
	id    string
	agent cliAgent
	err   error
}

func newCLIFullscreenPicker(
	ctx context.Context, out io.Writer, cwd string,
	loadAgents func(context.Context) ([]cliAgent, error),
	list func(context.Context, cliAgent, string) (json.RawMessage, error),
) *cliFullscreenPicker {
	return &cliFullscreenPicker{ctx: ctx, screen: newCLIScreen(out), cwd: cwd, loadAgents: loadAgents, list: list, pages: map[string]cliSessionPage{}}
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
	p.pageError = ""
	if page, ok := p.pages[agent.ID+"/"+cursor]; ok {
		p.sessionIndex = 0
		p.page = page
		return func() tea.Msg { return cliPickerPageMsg{id: requestID, page: page} }
	}
	ctx, cancel := context.WithCancel(p.ctx)
	p.cancelPage = cancel
	p.sessionIndex = 0
	return func() tea.Msg {
		defer cancel()
		raw, err := p.list(ctx, agent, cursor)
		if err != nil {
			return cliPickerPageMsg{id: requestID, err: err}
		}
		page, err := parseScopedCLISessionPage(agent, raw, p.cwd)
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
		var checks []tea.Cmd
		for _, agent := range p.agents {
			if agent.Checking && p.checkAgent != nil {
				checks = append(checks, p.check(agent.ID))
			}
		}
		return p, tea.Batch(checks...)
	case cliPickerAgentMsg:
		for i := range p.agents {
			if p.agents[i].ID == msg.id {
				if msg.err != nil || msg.agent.ID != msg.id {
					p.agents[i].Checking = false
					p.agents[i].Reason = "Check failed · press r to retry"
				} else {
					p.agents[i] = msg.agent
					p.agents[i].Checking = false
				}
			}
		}
	case cliPickerPageMsg:
		if msg.id != p.requestID || len(p.cursors) == 0 {
			break
		}
		p.cancelPage = nil
		if msg.err != nil {
			p.pageError = "Could not load sessions · press r to Retry"
			return p, nil
		}
		p.page = msg.page
		p.pages[p.agents[p.agentIndex].ID+"/"+p.cursors[len(p.cursors)-1]] = msg.page
	case tea.KeyMsg:
		if msg.Type == tea.KeyCtrlC || (msg.Type == tea.KeyEsc && len(p.cursors) == 0) {
			p.err = errCLICancelled
			return p, tea.Quit
		}
		if len(p.cursors) != 0 {
			if msg.String() == "r" {
				if p.cancelPage != nil {
					p.cancelPage()
					p.cancelPage = nil
				}
				clear(p.pages)
				return p, p.loadPage()
			}
			return p, p.sessionKey(msg.Type)
		}
		if msg.String() == "r" && len(p.agents) > 0 && p.checkAgent != nil && !p.agents[p.agentIndex].Checking {
			p.agents[p.agentIndex].Checking = true
			p.agents[p.agentIndex].Ready = false
			return p, p.check(p.agents[p.agentIndex].ID)
		}
		if len(p.agents) != 0 {
			return p, p.agentKey(msg.Type)
		}
	}
	return p, nil
}

func (p *cliFullscreenPicker) check(id string) tea.Cmd {
	return func() tea.Msg { agent, err := p.checkAgent(p.ctx, id); return cliPickerAgentMsg{id, agent, err} }
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
	if p.cancelPage != nil || p.pageError != "" {
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
	title, detail := "Modex / Choose an agent", "Project  "+cliLabel(p.cwd)
	footer := []cliScreenRow{{"↑↓ Move · Enter Open · r Check · Esc Quit", cliScreenMuted}}
	var rows []cliScreenRow
	index := p.agentIndex
	switch {
	case len(p.agents) == 0:
		rows = []cliScreenRow{{"Discovering agent runtimes…", cliScreenMuted}}
		footer[0].text = "Esc Cancel"
	case len(p.cursors) == 0:
		expanded := p.screen.height >= 16
		if expanded {
			index = p.agentIndex * 3
		}
		for _, agent := range p.agents {
			state := "Ready"
			if agent.Checking {
				state = "Checking…"
			} else if !agent.Ready {
				state = "Unavailable"
			}
			name := agent.icon() + "  " + agent.displayName()
			row := cliScreenRow{text: name + strings.Repeat(" ", max(2, 23-cliCellWidth(name))) + state}
			if !agent.Ready {
				row.tone = cliScreenMuted
			}
			rows = append(rows, row)
			if expanded {
				rows = append(rows, cliScreenRow{agent.description(), cliScreenMuted}, cliScreenRow{})
			}
		}
	default:
		title = "Modex / Sessions"
		detail = p.agents[p.agentIndex].displayName() + " · " + detail
		footer[0].text = "↑↓ Move · Enter Open · r Reload · Esc Back"
		index = p.sessionIndex
		if p.cancelPage != nil {
			rows = []cliScreenRow{{"Loading conversations…", cliScreenMuted}}
			footer[0].text = "Esc Back"
			break
		}
		if p.pageError != "" {
			rows = []cliScreenRow{{p.pageError, cliScreenMuted}}
			index = 0
			break
		}
		rows = append(rows, cliScreenRow{text: "New conversation", tone: cliScreenAccent})
		for _, session := range p.page.Data {
			rows = append(rows, cliScreenRow{text: session.menuLabel(p.cwd)})
		}
		if len(p.page.Data) == 0 {
			rows = append(rows, cliScreenRow{"No conversations in this project yet.", cliScreenMuted})
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
	if len(p.cursors) == 0 && len(p.agents) > 0 && p.screen.height >= 12 {
		agent := p.agents[p.agentIndex]
		description := "Open the native " + agent.displayName() + " terminal in this project."
		if agent.Checking {
			description = "Verifying sign-in and runtime compatibility…"
		} else if !agent.Ready {
			description = agent.Reason
		}
		footer = append([]cliScreenRow{{cliLabel(description), cliScreenMuted}}, footer...)
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
	start := min(max(0, len(rows)-1), max(0, index-height+1))
	return p.screen.render(title, detail, rows[start:min(len(rows), start+height)], footer)
}
