// modex is a thin OpenSSH client for a target-owned Modex supervisor.
package main

import (
	"context"
	"flag"
	"fmt"
	"io"
	"os"
	"os/exec"
	"os/signal"

	"github.com/warmdev/modex/internal/sshtransport"
)

func main() {
	if err := run(); err != nil {
		fmt.Fprintln(os.Stderr, "modex:", err)
		os.Exit(1)
	}
}

func run() error {
	var o sshtransport.Options
	flag.StringVar(&o.Target, "target", "", "SSH hostname, user@host, or configured alias")
	flag.StringVar(&o.HostBinary, "host-binary", "", "Absolute POSIX path to remote modex-host")
	flag.StringVar(&o.StateDir, "state-dir", "", "Absolute private supervisor state directory on target")
	flag.StringVar(&o.Instance, "instance", "", "Supervisor instance for cursor reconnect")
	flag.Uint64Var(&o.Cursor, "cursor", 0, "Last fully handled journal sequence")
	flag.IntVar(&o.Port, "port", 0, "SSH port override")
	flag.StringVar(&o.Identity, "identity", "", "Local SSH private key file")
	flag.StringVar(&o.KnownHosts, "known-hosts", "", "Local verified known_hosts file")
	binary := flag.String("ssh", "ssh", "OpenSSH executable")
	flag.Parse()
	if flag.NArg() != 0 {
		return fmt.Errorf("unexpected positional arguments")
	}
	args, err := sshtransport.Args(o)
	if err != nil {
		return err
	}
	ctx, cancel := signal.NotifyContext(context.Background(), os.Interrupt)
	defer cancel()
	cmd := exec.CommandContext(ctx, *binary, args...)
	cmd.Stdin = os.Stdin
	cmd.Stdout = os.Stdout
	// OpenSSH diagnostics can contain local paths/config details. Keep the JSON channel clean.
	cmd.Stderr = io.Discard
	if cmd.Run() != nil {
		return fmt.Errorf("SSH attach ended unsuccessfully; check connectivity, verified host key, key authentication and remote supervisor; reconnect with last instance/cursor, never resend uncertain commands")
	}
	return nil
}
