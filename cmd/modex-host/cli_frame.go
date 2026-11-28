package main

import (
	"fmt"
	"io"
	"os"
	"strconv"
	"strings"
	"unicode"
)

func cliRenderFrame(out io.Writer, heading, detail string, lines []string, footer string) {
	width := 72
	if value, err := strconv.Atoi(os.Getenv("COLUMNS")); err == nil && value >= 24 {
		width = value
	} else if value := cliTerminalWidth(out); value >= 24 {
		width = value
	}
	if width > 100 {
		width = 100
	}
	inner := width - 4
	type row struct{ text, tone string }
	rows := []row{{"┌" + strings.Repeat("─", width-2) + "┐", "accent"}}
	appendRow := func(value, tone string) {
		value = fitCLIText(cliLabel(value), inner)
		rows = append(rows, row{value + strings.Repeat(" ", inner-cliCellWidth(value)), tone})
	}
	appendRow(heading, "accent")
	if detail != "" {
		appendRow(detail, "muted")
	}
	if len(lines) != 0 {
		rows = append(rows, row{"├" + strings.Repeat("─", width-2) + "┤", "accent"})
		for _, line := range lines {
			appendRow(line, "body")
		}
	}
	if footer != "" {
		rows = append(rows, row{"├" + strings.Repeat("─", width-2) + "┤", "accent"})
		appendRow(footer, "muted")
	}
	rows = append(rows, row{"└" + strings.Repeat("─", width-2) + "┘", "accent"})
	accent, muted := cliFramePalette(out)
	for _, row := range rows {
		if strings.HasPrefix(row.text, "┌") || strings.HasPrefix(row.text, "├") || strings.HasPrefix(row.text, "└") {
			writeCLIColor(out, accent, row.text)
			fmt.Fprintln(out)
			continue
		}
		color := ""
		if row.tone == "accent" {
			color = accent
		} else if row.tone == "muted" {
			color = muted
		}
		writeCLIColor(out, accent, "│ ")
		writeCLIColor(out, color, row.text)
		writeCLIColor(out, accent, " │")
		fmt.Fprintln(out)
	}
}

func cliFrameColor(out io.Writer) string {
	accent, _ := cliFramePalette(out)
	return accent
}

func cliFramePalette(out io.Writer) (string, string) {
	if os.Getenv("NO_COLOR") != "" {
		return "", ""
	}
	file, ok := out.(*os.File)
	if !ok {
		return "", ""
	}
	info, err := file.Stat()
	if err != nil || info.Mode()&os.ModeCharDevice == 0 || cliTerminalWidth(out) == 0 {
		return "", ""
	}
	fields := strings.Split(os.Getenv("COLORFGBG"), ";")
	if len(fields) > 0 {
		background, err := strconv.Atoi(fields[len(fields)-1])
		if err == nil && background >= 7 {
			return "37;86;140", "90;96;115"
		}
	}
	if strings.EqualFold(os.Getenv("TERM_BACKGROUND"), "light") {
		return "37;86;140", "90;96;115"
	}
	return "137;180;250", "147;153;178"
}

func writeCLIColor(out io.Writer, color, value string) {
	if color == "" {
		fmt.Fprint(out, value)
		return
	}
	fmt.Fprintf(out, "\x1b[38;2;%sm%s\x1b[0m", color, value)
}

func fitCLIText(value string, width int) string {
	if cliCellWidth(value) <= width {
		return value
	}
	if width <= 1 {
		return "…"
	}
	var b strings.Builder
	used := 0
	for _, r := range value {
		cells := cliRuneWidth(r)
		if used+cells > width-1 {
			break
		}
		b.WriteRune(r)
		used += cells
	}
	return b.String() + "…"
}
func cliCellWidth(value string) int {
	width := 0
	for _, r := range value {
		width += cliRuneWidth(r)
	}
	return width
}
func cliRuneWidth(r rune) int {
	if unicode.Is(unicode.Mn, r) || unicode.Is(unicode.Me, r) {
		return 0
	}
	if r >= 0x1100 && (r <= 0x115f || r >= 0x2e80 && r <= 0xa4cf || r >= 0xac00 && r <= 0xd7a3 || r >= 0xf900 && r <= 0xfaff || r >= 0xfe10 && r <= 0xfe6f || r >= 0xff00 && r <= 0xff60 || r >= 0x1f300 && r <= 0x1faff) {
		return 2
	}
	return 1
}
