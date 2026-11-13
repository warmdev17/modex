package service

import (
	"strings"
	"testing"
)

func TestRenderEscapesServiceArguments(t *testing.T) {
	c := Config{Binary: "/home/u/a %x/modex-host", Workspace: "/work/quote\" here", Codex: "/bin/codex", State: "/state", Path: "/bin:$HOME", CodexHome: "/home/u/.codex"}
	unit, err := Render("linux", c)
	if err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{"%%x", `quote\" here`, `--supervise`, `Restart=on-failure`, `PATH=/bin:$HOME`} {
		if !strings.Contains(unit, want) {
			t.Fatalf("missing %q in %s", want, unit)
		}
	}
	plist, err := Render("darwin", c)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(plist, "<string>/work/quote&#34; here</string>") || strings.Contains(plist, "--background") {
		t.Fatal(plist)
	}
	c.Workspace = "/work\nExecStart=evil"
	if _, err := Render("linux", c); err == nil {
		t.Fatal("accepted newline injection")
	}
}

func TestUnsupportedPlatform(t *testing.T) {
	if _, err := Render("windows", Config{}); err == nil {
		t.Fatal("native Windows service claimed")
	}
}
