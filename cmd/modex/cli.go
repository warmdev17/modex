package main

import (
	"errors"
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
)

func launchCLI(args []string) error {
	if runtime.GOOS == "windows" {
		return errors.New("shared CLI hosting requires Linux or macOS; use WSL2 on Windows")
	}
	cwd, err := os.Getwd()
	if err != nil {
		return err
	}
	executable, err := os.Executable()
	if err != nil {
		return err
	}
	flags := flag.NewFlagSet("modex cli", flag.ContinueOnError)
	host := flags.String("host-binary", filepath.Join(filepath.Dir(executable), "modex-host"), "Modex host executable (defaults to sibling of modex)")
	workspace := flags.String("workspace", cwd, "Workspace directory (defaults to current directory)")
	codex := flags.String("codex", "codex", "Codex executable")
	home := flags.String("codex-home", "", "Override CODEX_HOME")
	thread := flags.String("thread", "", "Optional existing session UUID; omit for the picker")
	state := flags.String("state-dir", "", "Supervisor state directory (defaults to ~/.local/state/modex-phone)")
	if err := flags.Parse(args); err != nil {
		if errors.Is(err, flag.ErrHelp) {
			return nil
		}
		return err
	}
	if flags.NArg() != 0 {
		return fmt.Errorf("unexpected positional arguments; use modex cli --help")
	}
	forward := []string{"--cli", "--workspace", *workspace, "--codex", *codex}
	if *state != "" {
		forward = append(forward, "--state-dir", *state)
	}
	if *home != "" {
		forward = append(forward, "--codex-home", *home)
	}
	if *thread != "" {
		forward = append(forward, "--thread", *thread)
	}
	if err := execCLI(*host, forward); err != nil {
		return fmt.Errorf("cannot start native CLI launcher: %w (install modex and modex-host beside each other)", err)
	}
	return nil
}
