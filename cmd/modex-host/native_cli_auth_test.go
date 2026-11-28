package main

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestNativeOpenCodeImportsThroughPrivatePipe(t *testing.T) {
	binary := filepath.Join(t.TempDir(), "opencode")
	script := `#!/usr/bin/env python3
import json, os, sys
if sys.argv[1:3] == ["auth", "export"]:
    print(json.dumps({"credential": "synthetic-provider-token"}))
elif sys.argv[1:3] == ["auth", "import"]:
    assert sys.argv[3:] == ["--server", "http://127.0.0.1:12345"]
    assert os.environ["OPENCODE_PASSWORD"] == "synthetic-server-password"
    assert json.load(sys.stdin)["credential"] == "synthetic-provider-token"
else:
    sys.exit(3)
`
	if err := os.WriteFile(binary, []byte(script), 0700); err != nil {
		t.Fatal(err)
	}
	env := append(os.Environ(), "OPENCODE_PASSWORD=synthetic-server-password")
	if err := seedNativeOpenCodeAuth(context.Background(), binary, "http://127.0.0.1:12345", env); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(binary, []byte("#!/bin/sh\necho secret-should-stay-private >&2\nexit 1\n"), 0700); err != nil {
		t.Fatal(err)
	}
	err := seedNativeOpenCodeAuth(context.Background(), binary, "http://127.0.0.1:12345", env)
	if err == nil || strings.Contains(err.Error(), "secret") {
		t.Fatalf("provider failure was not sanitized: %v", err)
	}
}
