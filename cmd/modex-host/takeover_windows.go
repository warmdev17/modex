//go:build windows

package main

import (
	"context"
	"encoding/json"
	"errors"
)

func runCLI(string, string, string, string) error {
	return errors.New("registered CLI takeover requires Linux or macOS")
}
func contactLauncher(context.Context, string, string) (json.RawMessage, error) {
	return nil, errors.New("registered CLI takeover requires Linux or macOS")
}
