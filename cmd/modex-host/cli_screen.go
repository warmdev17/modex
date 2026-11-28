package main

import (
	"fmt"
	"io"
	"os"
	"strings"

	"github.com/charmbracelet/x/ansi"
	"github.com/charmbracelet/x/term"
)

type cliScreenTone uint8

const (
	cliScreenBody cliScreenTone = iota
	cliScreenMuted
	cliScreenAccent
	cliScreenSelected
)

type cliScreenRow struct {
	text string
	tone cliScreenTone
}

type cliScreen struct {
	width, height int
	accent, muted string
}

func newCLIScreen(out io.Writer) cliScreen {
	s := cliScreen{width: 80, height: 24}
	if f, ok := out.(*os.File); ok {
		if w, h, err := term.GetSize(f.Fd()); err == nil && w > 0 && h > 0 {
			s.width, s.height = w, h
		}
	}
	s.accent, s.muted = cliFramePalette(out)
	return s
}

func interactiveCLI(in, out *os.File) bool {
	return os.Getenv("TERM") != "dumb" && term.IsTerminal(in.Fd()) && term.IsTerminal(out.Fd())
}

func (s cliScreen) chromeHeight() int {
	switch {
	case s.height < 5 || s.width < 5:
		return 0
	case s.height < 9:
		return 3
	default:
		return 6
	}
}

func (s cliScreen) bodyWidth() int {
	if s.chromeHeight() == 0 {
		return max(1, s.width)
	}
	return s.width - 4
}

func (s cliScreen) bodyHeight(footerRows int) int {
	return max(1, s.height-s.chromeHeight()-footerRows)
}

func (s cliScreen) paint(row cliScreenRow, width int) string {
	text := strings.ReplaceAll(cliSafeText(row.text), "\n", " ")
	text = ansi.Truncate(text, width, "…")
	text += strings.Repeat(" ", max(0, width-ansi.StringWidth(text)))
	if s.accent == "" {
		return text
	}
	switch row.tone {
	case cliScreenMuted:
		return "\x1b[38;2;" + s.muted + "m" + text + "\x1b[0m"
	case cliScreenAccent:
		return "\x1b[1;38;2;" + s.accent + "m" + text + "\x1b[0m"
	case cliScreenSelected:
		ink := "21;29;45"
		if s.accent == "37;86;140" {
			ink = "244;248;255"
		}
		return "\x1b[38;2;" + ink + ";48;2;" + s.accent + "m" + text + "\x1b[0m"
	default:
		return text
	}
}

func (s cliScreen) render(title, detail string, body, footer []cliScreenRow) string {
	chrome := s.chromeHeight()
	footer = footer[:min(len(footer), max(0, s.height-chrome-1))]
	rows := make([]string, 0, max(1, s.height))
	border := func(left, middle, right string) {
		rule := left + strings.Repeat(middle, s.width-2) + right
		if s.muted != "" {
			rule = fmt.Sprintf("\x1b[38;2;%sm%s\x1b[0m", s.muted, rule)
		}
		rows = append(rows, rule)
	}
	row := func(content cliScreenRow) {
		text := s.paint(content, s.bodyWidth())
		if chrome != 0 {
			text = "│ " + text + " │"
		}
		rows = append(rows, text)
	}
	if chrome != 0 {
		border("┌", "─", "┐")
		row(cliScreenRow{title, cliScreenAccent})
	}
	if chrome == 6 {
		row(cliScreenRow{detail, cliScreenMuted})
		border("├", "─", "┤")
	}
	for i := range s.bodyHeight(len(footer)) {
		var item cliScreenRow
		if i < len(body) {
			item = body[i]
		}
		row(item)
	}
	if chrome == 6 {
		border("├", "─", "┤")
	}
	for _, item := range footer {
		row(item)
	}
	if chrome != 0 {
		border("└", "─", "┘")
	}
	return strings.Join(rows, "\n")
}
