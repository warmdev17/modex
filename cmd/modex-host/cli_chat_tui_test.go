package main

import (
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"
)

func TestCLIChatModelSubmitsEditedPromptAndCommands(t *testing.T) {
	submissions := make(chan string, 1)
	m := newCLIChatModel(cliScreen{width: 60, height: 16}, "OpenCode", "/work", submissions)
	m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("hllo")})
	m.Update(tea.KeyMsg{Type: tea.KeyHome})
	m.Update(tea.KeyMsg{Type: tea.KeyRight})
	m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("e")})
	m.Update(tea.KeyMsg{Type: tea.KeyEnter})
	if got := <-submissions; got != "hello" {
		t.Fatalf("submitted %q, want edited prompt", got)
	}
	for _, command := range []string{"/older", "/stop", "/quit"} {
		m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune(command)})
		m.Update(tea.KeyMsg{Type: tea.KeyEnter})
		select {
		case got := <-submissions:
			if got != command {
				t.Fatalf("submitted %q, want %q", got, command)
			}
		default:
			t.Fatalf("did not submit %q", command)
		}
	}
}

func TestCLIChatModelWrapsAndScrolls(t *testing.T) {
	m := newCLIChatModel(cliScreen{width: 34, height: 12}, "Antigravity", "/work", make(chan string, 1))
	m.Update(cliChatUpdate{entry: cliChatEntry{label: "user", text: "previous question that extends across the chat line"}})
	m.Update(cliChatUpdate{entry: cliChatEntry{label: "assistant", text: "first reply"}})
	m.Update(cliChatUpdate{entry: cliChatEntry{label: "assistant", text: " grows across several rows and finishes here"}, continuation: true})
	m.Update(cliChatUpdate{entry: cliChatEntry{text: "Turn completed."}})
	latest := m.View()
	if !strings.Contains(latest, "finishes here") || !strings.Contains(latest, "Turn completed.") || strings.Contains(latest, "previous question") {
		t.Fatalf("latest transcript:\n%s", latest)
	}
	m.Update(tea.KeyMsg{Type: tea.KeyPgUp})
	older := m.View()
	if !strings.Contains(older, "previous question") || !strings.Contains(older, "extends across the chat line") || !strings.Contains(older, "Antigravity › first reply") || !strings.Contains(older, "› ▏") {
		t.Fatalf("earlier transcript:\n%s", older)
	}
	m.Update(tea.KeyMsg{Type: tea.KeyPgDown})
	if got := m.View(); got != latest {
		t.Fatalf("page down did not return to latest transcript:\n%s", got)
	}
}

func TestCLIChatModelShowsCaretForLongInputAndExits(t *testing.T) {
	m := newCLIChatModel(cliScreen{width: 28, height: 12}, "OpenCode", "/work", make(chan string, 1))
	m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("prefix " + strings.Repeat("界", 25) + " suffix")})
	if got := m.View(); !strings.Contains(got, "suffix▏") || strings.Contains(got, "prefix") {
		t.Fatalf("end of long input not visible:\n%s", got)
	}
	m.Update(tea.KeyMsg{Type: tea.KeyHome})
	if got := m.View(); !strings.Contains(got, "▏prefix") {
		t.Fatalf("beginning of long input not visible:\n%s", got)
	}
	for _, key := range []tea.KeyType{tea.KeyEsc, tea.KeyCtrlC} {
		_, cmd := m.Update(tea.KeyMsg{Type: key})
		if cmd == nil {
			t.Fatalf("%v did not request exit", key)
		}
		if _, ok := cmd().(tea.QuitMsg); !ok {
			t.Fatalf("%v did not request exit", key)
		}
	}
}

func TestCLIChatFormatsMarkdownSectionsAndKeepsCodeIndentation(t *testing.T) {
	m := newCLIChatModel(cliScreen{width: 65, height: 20}, "Antigravity", "/work", make(chan string, 1))
	m.Update(cliChatUpdate{entry: cliChatEntry{label: "assistant", text: "### Usage\n\n```bash\n  python3 size_by_ext.py\n```\n- Run the script"}})
	view := m.View()
	for _, want := range []string{"Antigravity", "Usage", "bash", "│   python3 size_by_ext.py", "• Run the script"} {
		if !strings.Contains(view, want) {
			t.Errorf("missing %q in view:\n%s", want, view)
		}
	}
	if strings.Contains(view, "###") || strings.Contains(view, "```") {
		t.Fatalf("exposed markdown structure:\n%s", view)
	}
}
