package sshtransport

import (
	"reflect"
	"strings"
	"testing"
)

func TestSSHArgsPinsSecurityAndQuotesRemotePaths(t *testing.T) {
	args, err := Args(Options{Target: "user@host", HostBinary: "/opt/modex host's/bin", StateDir: "/home/user/state $(id)", Cursor: 42, Instance: "abc"})
	if err != nil {
		t.Fatal(err)
	}
	expected := []string{"-T", "-o", "BatchMode=yes", "-o", "StrictHostKeyChecking=yes", "-o", "ConnectTimeout=10", "-o", "ServerAliveInterval=15", "-o", "ServerAliveCountMax=3", "--", "user@host", "exec '/opt/modex host'\"'\"'s/bin' --attach --state-dir '/home/user/state $(id)' --cursor 42 --instance 'abc'"}
	if !reflect.DeepEqual(args, expected) {
		t.Fatalf("args=%q", args)
	}
}
func TestRejectUnsafeTargetsAndIncompleteCursor(t *testing.T) {
	for _, target := range []string{"", "-oProxyCommand=evil", "user host", "host\ncommand", "host;evil", "user@host$(id)"} {
		if _, err := Args(Options{Target: target, HostBinary: "/host", StateDir: "/state"}); err == nil {
			t.Fatalf("accepted target %q", target)
		}
	}
	for _, o := range []Options{{Target: "host", HostBinary: "/host", StateDir: "/state", Cursor: 1}, {Target: "host", StateDir: "/state"}, {Target: "host", HostBinary: "-bad", StateDir: "/state"}, {Target: "host", HostBinary: "/host", StateDir: "state"}, {Target: "host", HostBinary: "/host\ncmd", StateDir: "/state"}} {
		if _, err := Args(o); err == nil {
			t.Fatalf("accepted %+v", o)
		}
	}
}
func TestSSHOptionalFilesAndPort(t *testing.T) {
	args, err := Args(Options{Target: "host", HostBinary: "/host", StateDir: "/state", Port: 2222, Identity: "/key", KnownHosts: "/known"})
	if err != nil || !strings.Contains(strings.Join(args, " "), "-p 2222 -i /key -o IdentitiesOnly=yes -o UserKnownHostsFile=/known") {
		t.Fatalf("args=%q err=%v", args, err)
	}
}
