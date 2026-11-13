//go:build !windows

package main

import (
	"os/exec"
	"syscall"

	"github.com/warmdev/modex/internal/hoststate"
)

func checkPrivatePath(path string, directory bool) error { return hoststate.Validate(path, directory) }

func detachSupervisor(cmd *exec.Cmd) { cmd.SysProcAttr = &syscall.SysProcAttr{Setsid: true} }
