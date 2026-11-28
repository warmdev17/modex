package main

import (
	"strings"
	"testing"

	"github.com/charmbracelet/x/ansi"
)

func TestCLIScreenFillsTerminalAndKeepsControlsVisible(t *testing.T) {
	screen := cliScreen{width: 44, height: 12, accent: "137;180;250", muted: "147;153;178"}
	body := []cliScreenRow{{text: "› Antigravity\x1b[2J", tone: cliScreenSelected}}
	footer := []cliScreenRow{{text: "↑/↓ Move  Enter Open", tone: cliScreenMuted}}
	view := screen.render("Modex", "Choose an agent", body, footer)
	if !strings.Contains(view, "48;2;137;180;250m") || strings.Contains(view, "\x1b[2J") {
		t.Fatalf("missing RGB focus or unchecked control sequence: %q", view)
	}
	rows := strings.Split(ansi.Strip(view), "\n")
	if len(rows) != screen.height || !strings.Contains(rows[screen.height-2], "Enter Open") {
		t.Fatalf("footer is out of view: %#v", rows)
	}
	for _, row := range rows {
		if got := ansi.StringWidth(row); got != screen.width {
			t.Errorf("row width %d, want %d: %q", got, screen.width, row)
		}
	}
}

func TestCLIScreenFitsSmallPlainTerminal(t *testing.T) {
	screen := cliScreen{width: 14, height: 6}
	view := screen.render("Modex", "Agent sessions", []cliScreenRow{{text: "› Phiên hiện tại 😄", tone: cliScreenSelected}}, []cliScreenRow{{text: "Enter Open"}})
	rows := strings.Split(view, "\n")
	if len(rows) != screen.height || strings.Contains(view, "\x1b") || !strings.Contains(view, "› Phiên") || !strings.Contains(view, "Enter") {
		t.Fatalf("small screen dropped navigation or drew color: %q", view)
	}
	for _, row := range rows {
		if got := ansi.StringWidth(row); got > screen.width {
			t.Errorf("row overflow: %d: %q", got, row)
		}
	}
}
