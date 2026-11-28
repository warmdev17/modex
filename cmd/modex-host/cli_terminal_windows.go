//go:build windows

package main

import "io"

func cliTerminalWidth(io.Writer) int { return 0 }
