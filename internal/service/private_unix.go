//go:build !windows

package service

import (
	"errors"
	"os"
	"syscall"
)

func privateState(path string) error {
	if err := os.MkdirAll(path, 0700); err != nil {
		return err
	}
	info, err := os.Lstat(path)
	if err != nil {
		return err
	}
	owner, ok := info.Sys().(*syscall.Stat_t)
	if !info.IsDir() || info.Mode()&os.ModeSymlink != 0 || info.Mode().Perm()&0077 != 0 || !ok || owner.Uid != uint32(os.Geteuid()) {
		return errors.New("state must be a private current-user-owned directory without symlinks")
	}
	return nil
}
