// Package sshtransport carries supervisor JSON over an authenticated OpenSSH exec channel.
package sshtransport

import (
	"errors"
	"fmt"
	"regexp"
	"strconv"
	"strings"
)

type Options struct {
	Target, HostBinary, StateDir, Instance, Identity, KnownHosts string
	Cursor                                                       uint64
	Port                                                         int
}

var targetPattern = regexp.MustCompile(`^[a-zA-Z0-9_][a-zA-Z0-9_.@:\[\]-]*$`)

// Args targets a POSIX remote shell. Quoting is required because SSH exec sends a command string.
func Args(o Options) ([]string, error) {
	if !targetPattern.MatchString(o.Target) {
		return nil, errors.New("invalid SSH destination; use a hostname or SSH config alias")
	}
	if !strings.HasPrefix(o.HostBinary, "/") || !strings.HasPrefix(o.StateDir, "/") {
		return nil, errors.New("remote host binary and state directory must be absolute POSIX paths")
	}
	for _, value := range []string{o.HostBinary, o.StateDir, o.Instance, o.Identity, o.KnownHosts} {
		if strings.ContainsAny(value, "\x00\r\n") {
			return nil, errors.New("SSH options must not contain control characters")
		}
	}
	if o.Cursor > 0 && o.Instance == "" {
		return nil, errors.New("instance is required with a nonzero cursor")
	}
	if o.Port < 0 || o.Port > 65535 {
		return nil, errors.New("invalid SSH port")
	}
	args := []string{"-T", "-o", "BatchMode=yes", "-o", "StrictHostKeyChecking=yes", "-o", "ConnectTimeout=10", "-o", "ServerAliveInterval=15", "-o", "ServerAliveCountMax=3"}
	if o.Port != 0 {
		args = append(args, "-p", strconv.Itoa(o.Port))
	}
	if o.Identity != "" {
		args = append(args, "-i", o.Identity, "-o", "IdentitiesOnly=yes")
	}
	if o.KnownHosts != "" {
		args = append(args, "-o", "UserKnownHostsFile="+o.KnownHosts)
	}
	quote := func(value string) string { return "'" + strings.ReplaceAll(value, "'", "'\"'\"'") + "'" }
	command := fmt.Sprintf("exec %s --attach --state-dir %s --cursor %d", quote(o.HostBinary), quote(o.StateDir), o.Cursor)
	if o.Instance != "" {
		command += " --instance " + quote(o.Instance)
	}
	return append(args, "--", o.Target, command), nil
}
