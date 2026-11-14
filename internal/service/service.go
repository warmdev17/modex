package service

import (
	"errors"
	"flag"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"

	"github.com/warmdev/modex/internal/hoststate"
)

func registration(home string) (string, error) {
	switch runtime.GOOS {
	case "linux":
		root, err := os.UserConfigDir()
		if err != nil {
			return "", err
		}
		if !filepath.IsAbs(root) {
			return "", errors.New("configuration directory must be absolute")
		}
		return filepath.Join(root, "systemd", "user", Name+".service"), nil
	case "darwin":
		return filepath.Join(home, "Library", "LaunchAgents", Name+".plist"), nil
	default:
		return "", errors.New("native host services unsupported on Windows; install in WSL2")
	}
}

func Run(args []string, out io.Writer) error {
	if len(args) == 0 {
		return errors.New("usage: modex service install|start|stop|status|uninstall")
	}
	action := args[0]
	home, err := os.UserHomeDir()
	if err != nil {
		return err
	}
	path, err := registration(home)
	if err != nil {
		return err
	}
	if action == "install" {
		return install(args[1:], home, path, out)
	}
	if len(args) != 1 {
		return errors.New("unexpected service arguments")
	}
	if action != "start" && action != "stop" && action != "status" && action != "uninstall" {
		return errors.New("unknown service action")
	}
	info, err := os.Lstat(path)
	if err != nil {
		return fmt.Errorf("service is not installed: %w", err)
	}
	if !info.Mode().IsRegular() || info.Mode().Perm()&0077 != 0 {
		return errors.New("service registration must be a private regular file")
	}
	run := func(name string, args ...string) error {
		cmd := exec.Command(name, args...)
		cmd.Stdout = out
		cmd.Stderr = out
		return cmd.Run()
	}
	unit := Name + ".service"
	if runtime.GOOS == "linux" {
		switch action {
		case "start":
			if err := run("systemctl", "--user", "daemon-reload"); err != nil {
				return err
			}
			return run("systemctl", "--user", "enable", "--now", unit)
		case "stop":
			fmt.Fprintln(out, "Stopping can interrupt active turns; uncertain prompts are never replayed.")
			return run("systemctl", "--user", "disable", "--now", unit)
		case "status":
			return run("systemctl", "--user", "status", unit)
		case "uninstall":
			cmd := exec.Command("systemctl", "--user", "show", unit, "--property=ActiveState", "--value")
			state, err := cmd.Output()
			if err != nil {
				return errors.New("cannot confirm stopped service; check systemd user bus")
			}
			if s := strings.TrimSpace(string(state)); s != "inactive" && s != "failed" {
				return errors.New("stop the service before uninstalling")
			}
			if err := run("systemctl", "--user", "disable", unit); err != nil {
				return err
			}
			if err := os.Remove(path); err != nil {
				return err
			}
			return run("systemctl", "--user", "daemon-reload")
		}
	}
	domain := fmt.Sprintf("gui/%d", os.Getuid())
	target := domain + "/" + Name
	switch action {
	case "start":
		return run("launchctl", "bootstrap", domain, path)
	case "status":
		return run("launchctl", "print", target)
	case "stop":
		fmt.Fprintln(out, "Stopping can interrupt active turns; uncertain prompts are never replayed.")
		return run("launchctl", "bootout", target)
	case "uninstall":
		if err := exec.Command("launchctl", "print", domain).Run(); err != nil {
			return errors.New("no logged-in launchd domain; stop/uninstall from the user login session")
		}
		if err := exec.Command("launchctl", "print", target).Run(); err == nil {
			return errors.New("stop the service before uninstalling")
		}
		return os.Remove(path)
	}
	return nil
}

func install(args []string, home, path string, out io.Writer) error {
	f := flag.NewFlagSet("modex service install", flag.ContinueOnError)
	workspace := f.String("workspace", "", "Existing absolute workspace directory (required)")
	codex := f.String("codex", "codex", "Codex executable")
	state := f.String("state-dir", filepath.Join(home, ".local", "state", "modex-phone"), "New private state directory")
	codexHome := f.String("codex-home", os.Getenv("CODEX_HOME"), "Explicit Codex home")
	self, err := os.Executable()
	if err != nil {
		return err
	}
	binary := f.String("host-binary", filepath.Join(filepath.Dir(self), "modex-host"), "Absolute host executable")
	if err := f.Parse(args); err != nil {
		return err
	}
	if f.NArg() != 0 {
		return errors.New("unexpected install arguments")
	}
	if _, err := os.Lstat(path); !os.IsNotExist(err) {
		return errors.New("registration already exists; stop and uninstall before changing it")
	}
	for _, dir := range []string{*workspace, *state} {
		if !filepath.IsAbs(dir) {
			return errors.New("workspace and state directory must be absolute")
		}
	}
	for _, dir := range []string{*workspace, *codexHome} {
		if dir == "" {
			continue
		}
		info, err := os.Stat(dir)
		if err != nil || !info.IsDir() {
			return errors.New("workspace/CODEX_HOME must be existing directories")
		}
	}
	if *workspace == "" {
		return errors.New("workspace is required")
	}
	for _, p := range []*string{binary, codex} {
		resolved, err := exec.LookPath(*p)
		if err != nil {
			return fmt.Errorf("executable unavailable: %s", *p)
		}
		*p, err = filepath.Abs(resolved)
		if err != nil {
			return err
		}
	}
	if *codexHome != "" && !filepath.IsAbs(*codexHome) {
		return errors.New("CODEX_HOME must be absolute")
	}
	// Hold exclusive ownership until registration is written; preserve endpoint and attachments.
	release, err := hoststate.Acquire(*state)
	if err != nil {
		return err
	}
	defer release()
	content, err := Render(runtime.GOOS, Config{*binary, *workspace, *codex, *state, os.Getenv("PATH"), *codexHome})
	if err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(path), 0700); err != nil {
		return err
	}
	file, err := os.OpenFile(path, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0600)
	if err != nil {
		return err
	}
	_, writeErr := file.WriteString(content)
	closeErr := file.Close()
	if writeErr != nil {
		os.Remove(path)
		return writeErr
	}
	if closeErr != nil {
		os.Remove(path)
		return closeErr
	}
	fmt.Fprintf(out, "Installed (not started): %s\nAndroid host binary: %s\nAndroid state directory: %s\nRun modex service start when ready. Codex and sshd must be configured separately.\n", path, *binary, *state)
	return nil
}
