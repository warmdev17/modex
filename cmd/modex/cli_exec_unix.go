//go:build !windows

package main

import (
	"os"
	"os/exec"
	"syscall"
)

func execCLI(binary string, args []string) error {
	path, err := exec.LookPath(binary)
	if err != nil {
		return err
	}
	// Replace the wrapper so terminal signals go straight to the registered launcher.
	return syscall.Exec(path, append([]string{binary}, args...), os.Environ())
}
