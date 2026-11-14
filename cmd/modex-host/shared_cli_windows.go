//go:build windows

package main

import "errors"

func runSharedCLI(string, string, string, string, string) error {
	return errors.New("shared native CLI hosting requires Linux or macOS; use WSL2 on Windows")
}
