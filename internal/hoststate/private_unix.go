//go:build !windows

package hoststate

import (
	"errors"
	"os"
	"syscall"
)

func Validate(path string, directory bool) error {
	info, err := os.Lstat(path)
	if err != nil {
		return errors.New("private supervisor state does not exist")
	}
	owner, ok := info.Sys().(*syscall.Stat_t)
	if info.Mode()&os.ModeSymlink != 0 || info.IsDir() != directory || (!directory && !info.Mode().IsRegular()) || info.Mode().Perm()&0077 != 0 || !ok || owner.Uid != uint32(os.Geteuid()) {
		return errors.New("supervisor state must be owned by current user, private (0700 directory/0600 file), and not a symlink")
	}
	return nil
}
