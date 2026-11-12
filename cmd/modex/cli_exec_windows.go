//go:build windows

package main

import "errors"

func execCLI(string, []string) error {
	return errors.New("registered CLI takeover requires Linux or macOS")
}
