package main

import (
	"bufio"
	"context"
	"io"
	"os"
	"strings"
	"unicode/utf8"

	"github.com/charmbracelet/bubbles/cursor"
	"github.com/charmbracelet/bubbles/textinput"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/x/ansi"
	"github.com/rivo/uniseg"
)

const (
	cliChatMaxBlockBytes = 32 << 10
	cliChatMaxBytes      = 128 << 10
	cliChatMaxBlocks     = 512
)

type cliChatUpdate struct {
	entry        cliChatEntry
	continuation bool
}

type cliChatReady struct{}
type cliChatEnded struct{ err error }

type cliChatBlock struct {
	entry cliChatEntry
	rows  []cliScreenRow
}

type cliChatModel struct {
	screen       cliScreen
	agent        string
	workspace    string
	editor       textinput.Model
	submissions  chan<- string
	blocks       []cliChatBlock
	bytes        int
	scroll       int
	ready        bool
	inputPending bool
	err          error
}

func newCLIChatModel(screen cliScreen, agent, workspace string, submissions chan<- string) *cliChatModel {
	editor := textinput.New()
	editor.CharLimit = cliChatMaxBlockBytes
	editor.Cursor.SetMode(cursor.CursorHide)
	editor.Focus()
	return &cliChatModel{screen: screen, agent: agent, workspace: workspace, editor: editor, submissions: submissions}
}

func (*cliChatModel) Init() tea.Cmd { return nil }

func (m *cliChatModel) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.screen.width, m.screen.height = msg.Width, msg.Height
		for i := range m.blocks {
			m.blocks[i].rows = m.wrap(m.blocks[i].entry)
		}
	case cliChatReady:
		m.ready = true
	case cliChatUpdate:
		m.append(msg)
	case cliChatEnded:
		m.err = msg.err
		return m, tea.Quit
	case tea.KeyMsg:
		switch msg.Type {
		case tea.KeyEsc, tea.KeyCtrlC:
			return m, tea.Quit
		case tea.KeyEnter:
			text := cliSafeText(m.editor.Value())
			if strings.TrimSpace(text) != "" {
				select {
				case m.submissions <- text:
					m.editor.Reset()
					m.scroll = 0
					m.inputPending = false
				default:
					m.inputPending = true
				}
			}
		case tea.KeyPgUp:
			page := m.screen.bodyHeight(m.footerRows())
			m.scroll = min(m.scroll+page, max(0, m.rowCount()-page))
		case tea.KeyPgDown:
			m.scroll = max(0, m.scroll-m.screen.bodyHeight(m.footerRows()))
		default:
			var cmd tea.Cmd
			m.editor, cmd = m.editor.Update(msg)
			return m, cmd
		}
	default:
		var cmd tea.Cmd
		m.editor, cmd = m.editor.Update(msg)
		return m, cmd
	}
	return m, nil
}

func (m *cliChatModel) wrap(entry cliChatEntry) []cliScreenRow {
	prefix, tone := "", cliScreenMuted
	switch entry.label {
	case "user":
		prefix, tone = "You › ", cliScreenAccent
	case "assistant":
		prefix, tone = m.agent+" › ", cliScreenBody
	case "assistant (updated)":
		prefix, tone = m.agent+" (updated) › ", cliScreenBody
	}
	var rows []cliScreenRow
	add := func(line string, color cliScreenTone) {
		for _, wrapped := range strings.Split(ansi.Wrap(line, m.screen.bodyWidth(), ""), "\n") {
			rows = append(rows, cliScreenRow{wrapped, color})
		}
	}
	format := entry.label == "assistant" || entry.label == "assistant (updated)"
	inCode := false
	for i, line := range strings.Split(entry.text, "\n") {
		color := tone
		if format {
			trimmed := strings.TrimSpace(line)
			head, rest, heading := strings.Cut(trimmed, " ")
			switch {
			case strings.HasPrefix(trimmed, "```") && (!inCode || strings.Trim(trimmed, "`") == ""):
				if trimmed == "```" && inCode {
					inCode = false
					continue
				}
				if !inCode {
					inCode = true
					line, color = "Code", cliScreenMuted
					if language := strings.TrimSpace(strings.TrimPrefix(trimmed, "```")); language != "" {
						line += " · " + language
					}
				}
			case inCode:
				color = cliScreenBody
			case heading && strings.Trim(head, "#") == "" && len(head) >= 1 && len(head) <= 6:
				line, color = rest, cliScreenAccent
			case strings.HasPrefix(line, "- "):
				line = "• " + line[2:]
			}
		}
		if i == 0 {
			line = prefix + line
		}
		add(line, color)
	}
	return rows
}

func (m *cliChatModel) append(update cliChatUpdate) {
	entry := update.entry
	entry.text = cliSafeText(entry.text)
	if entry.text == "" && entry.label == "" {
		return
	}
	oldRows := 0
	if update.continuation && len(m.blocks) != 0 {
		last := m.blocks[len(m.blocks)-1]
		m.blocks = m.blocks[:len(m.blocks)-1]
		m.bytes -= len(last.entry.text)
		oldRows = len(last.rows)
		entry.text = last.entry.text + entry.text
	}
	if len(entry.text) > cliChatMaxBlockBytes {
		start := len(entry.text) - cliChatMaxBlockBytes + len("…")
		for !utf8.RuneStart(entry.text[start]) {
			start++
		}
		entry.text = "…" + entry.text[start:]
	}
	block := cliChatBlock{entry: entry, rows: m.wrap(entry)}
	m.blocks = append(m.blocks, block)
	m.bytes += len(entry.text)
	if m.scroll != 0 {
		m.scroll = max(0, m.scroll+len(block.rows)-oldRows)
	}
	for m.bytes > cliChatMaxBytes || len(m.blocks) > cliChatMaxBlocks {
		m.bytes -= len(m.blocks[0].entry.text)
		m.blocks[0] = cliChatBlock{}
		m.blocks = m.blocks[1:]
	}
}

func (m *cliChatModel) rowCount() int {
	n := 0
	for _, block := range m.blocks {
		n += len(block.rows)
	}
	return n
}

func (m *cliChatModel) footerRows() int {
	return min(3, max(1, m.screen.height-m.screen.chromeHeight()-1))
}

func (m *cliChatModel) composer() string {
	value := []rune(m.editor.Value())
	pos := m.editor.Position()
	before := cliSafeText(string(value[:pos]))
	after := cliSafeText(string(value[pos:]))
	if len(value) == 0 {
		after = "Message " + m.agent
	}
	width := m.screen.bodyWidth()
	prefix := "› "
	if width < 4 {
		prefix = ""
	}
	available := max(0, width-ansi.StringWidth(prefix)-1)
	left := available
	if after != "" && left > 1 {
		left--
	}
	if width := ansi.StringWidth(before); width > left {
		clusters := uniseg.NewGraphemes(before)
		for width >= left && clusters.Next() {
			width -= clusters.Width()
			_, end := clusters.Positions()
			if width < left {
				before = "…" + before[end:]
			}
		}
		if left == 0 {
			before = ""
		}
	}
	after = ansi.Truncate(after, available-ansi.StringWidth(before), "…")
	return prefix + before + "▏" + after
}

func (m *cliChatModel) View() string {
	footer := []cliScreenRow{
		{m.composer(), cliScreenAccent},
		{"Enter send · PgUp/PgDn scroll · Esc quit", cliScreenMuted},
		{"/older · /stop · /quit", cliScreenMuted},
	}
	if m.inputPending {
		footer[1].text = "Input pending · Enter to retry"
	}
	footer = footer[:m.footerRows()]
	var body []cliScreenRow
	if !m.ready && len(m.blocks) == 0 {
		body = append(body, cliScreenRow{"Opening conversation…", cliScreenMuted})
	}
	page := m.screen.bodyHeight(len(footer))
	count := m.rowCount()
	start := max(0, count-page-m.scroll)
	end := min(count, start+page)
	for _, block := range m.blocks {
		if len(block.rows) > start && end > 0 {
			body = append(body, block.rows[start:min(end, len(block.rows))]...)
		}
		start = max(0, start-len(block.rows))
		end -= len(block.rows)
		if end <= 0 {
			break
		}
	}
	return m.screen.render(m.agent, m.workspace, body, footer)
}

func runCLIChatFullscreen(ctx context.Context, in, out *os.File, client *localCLIClient, agentID, agentLabel, workspace, threadID string) error {
	sessionCtx, cancel := context.WithCancel(ctx)
	defer cancel()
	reader, writer := io.Pipe()
	submissions := make(chan string, 1)
	model := newCLIChatModel(newCLIScreen(out), agentLabel, workspace, submissions)
	program := tea.NewProgram(model, tea.WithContext(sessionCtx), tea.WithInput(in), tea.WithOutput(out), tea.WithAltScreen())
	writerDone := make(chan struct{})
	go func() {
		defer close(writerDone)
		for {
			select {
			case <-sessionCtx.Done():
				return
			case text := <-submissions:
				if _, err := io.WriteString(writer, text+"\n"); err != nil {
					return
				}
			}
		}
	}()
	sessionDone := make(chan struct{})
	go func() {
		defer close(sessionDone)
		err := runAgentCLIChatWithOutput(sessionCtx, bufio.NewReader(reader), client, agentID, workspace, threadID, cliChatOutput{
			start: func() { program.Send(cliChatReady{}) },
			line:  func(entry cliChatEntry) { program.Send(cliChatUpdate{entry: entry}) },
			delta: func(text string, begin bool) {
				program.Send(cliChatUpdate{entry: cliChatEntry{"assistant", text}, continuation: !begin})
			},
		})
		program.Send(cliChatEnded{err})
	}()
	_, err := program.Run()
	cancel()
	_ = reader.Close()
	_ = writer.Close()
	<-writerDone
	<-sessionDone
	if err != nil {
		return err
	}
	return model.err
}
