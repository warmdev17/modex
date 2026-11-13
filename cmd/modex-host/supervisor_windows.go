package main

import (
	"errors"
	"os/exec"
	"syscall"
)

// shortcut: Windows ACL validation is unavailable; fail closed until a native private-state ACL implementation is added.
func checkPrivatePath(string, bool) error {
	return errors.New("background supervisor requires private-state ACL support on Windows; use --serve")
}
func detachSupervisor(cmd *exec.Cmd) {
	cmd.SysProcAttr = &syscall.SysProcAttr{CreationFlags: 0x00000008 | 0x00000200}
}

func lockSupervisorState(string) (func(), error) {
	return nil, errors.New("Windows native supervisor is unsupported; use WSL2")
}
