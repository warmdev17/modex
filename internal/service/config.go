// Package service manages only current-user registrations; it never installs SSH or credentials.
package service

import (
	"bytes"
	"encoding/xml"
	"fmt"
	"strings"
)

type Config struct{ Binary, Workspace, Codex, State, Path, CodexHome string }

const Name = "dev.modex.host"

func Render(platform string, c Config) (string, error) {
	for _, s := range []string{c.Binary, c.Workspace, c.Codex, c.State, c.Path, c.CodexHome} {
		if strings.IndexFunc(s, func(r rune) bool { return r < 32 || r == 127 }) >= 0 {
			return "", fmt.Errorf("service paths/environment must be single-line")
		}
	}
	args := []string{c.Binary, "--supervise", "--workspace", c.Workspace, "--codex", c.Codex, "--state-dir", c.State}
	if c.CodexHome != "" {
		args = append(args, "--codex-home", c.CodexHome)
	}
	switch platform {
	case "linux":
		quote := func(s string) string {
			return `"` + strings.NewReplacer(`\`, `\\`, `"`, `\"`, `%`, `%%`).Replace(s) + `"`
		}
		quoted := make([]string, len(args))
		for i, s := range args {
			quoted[i] = quote(strings.ReplaceAll(s, "$", "$$"))
		}
		return "[Unit]\nDescription=Modex Codex host\nStartLimitIntervalSec=60\nStartLimitBurst=5\n[Service]\nType=simple\nExecStart=" + strings.Join(quoted, " ") + "\nEnvironment=" + quote("PATH="+c.Path) + "\nUMask=0077\nRestart=on-failure\nRestartSec=5\nTimeoutStopSec=30\n[Install]\nWantedBy=default.target\n", nil
	case "darwin":
		escape := func(s string) string { var b bytes.Buffer; _ = xml.EscapeText(&b, []byte(s)); return b.String() }
		var b strings.Builder
		b.WriteString(`<?xml version="1.0" encoding="UTF-8"?><!DOCTYPE plist PUBLIC "-//Apple//DTD PLIST 1.0//EN" "http://www.apple.com/DTDs/PropertyList-1.0.dtd"><plist version="1.0"><dict><key>Label</key><string>` + Name + `</string><key>ProgramArguments</key><array>`)
		for _, s := range args {
			b.WriteString("<string>" + escape(s) + "</string>")
		}
		b.WriteString("</array><key>EnvironmentVariables</key><dict><key>PATH</key><string>" + escape(c.Path) + "</string></dict><key>RunAtLoad</key><true/><key>KeepAlive</key><dict><key>SuccessfulExit</key><false/></dict><key>ThrottleInterval</key><integer>10</integer><key>Umask</key><integer>63</integer></dict></plist>\n")
		return b.String(), nil
	default:
		return "", fmt.Errorf("native services require Linux/systemd or macOS/launchd; on Windows use WSL2")
	}
}
