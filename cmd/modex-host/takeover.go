package main

import (
	"context"
	"encoding/hex"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
)

func validTakeoverThread(id string) bool {
	if len(id) != 36 || id[8] != '-' || id[13] != '-' || id[18] != '-' || id[23] != '-' {
		return false
	}
	decoded, err := hex.DecodeString(strings.ReplaceAll(id, "-", ""))
	return err == nil && len(decoded) == 16
}

func singleLoadedThread(raw json.RawMessage, id string) bool {
	var page struct {
		Data []string `json:"data"`
		Next *string  `json:"nextCursor"`
	}
	return json.Unmarshal(raw, &page) == nil && len(page.Data) == 1 && page.Data[0] == id && page.Next == nil
}

func takeoverRoot(home string) (string, error) {
	if home == "" {
		home = os.Getenv("CODEX_HOME")
	}
	if home == "" {
		userHome, err := os.UserHomeDir()
		if err != nil {
			return "", err
		}
		home = filepath.Join(userHome, ".codex")
	}
	return filepath.Abs(filepath.Join(home, "modex-cli"))
}

// No PID registry: only the live launcher holding this private socket can stop its children.
func requestTakeover(ctx context.Context, home, id string) (json.RawMessage, error) {
	root, err := takeoverRoot(home)
	if err != nil {
		return nil, err
	}
	return contactLauncher(ctx, root, id)
}
