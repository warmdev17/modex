//go:build !windows

package main

import (
	"errors"
	"io"
	"os"
	"path/filepath"
	"syscall"
)

const ownershipFormat = "modex-flock-v1\n"

func lockSupervisorState(dir string) (func(), error) {
	path := filepath.Join(dir, "owner.lock")
	endpoint := filepath.Join(dir, "endpoint.json")
	// Unknown state is never evidence that an old supervisor is dead.
	_, statErr := os.Lstat(path)
	if os.IsNotExist(statErr) {
		if _, err := os.Lstat(endpoint); !os.IsNotExist(err) {
			return nil, errors.New("existing legacy endpoint; stop old supervisor before recovery")
		}
	}
	fd, err := syscall.Open(path, syscall.O_RDWR|syscall.O_CREAT|syscall.O_NOFOLLOW|syscall.O_CLOEXEC|syscall.O_EXCL, 0600)
	created := err == nil
	if errors.Is(err, syscall.EEXIST) {
		fd, err = syscall.Open(path, syscall.O_RDWR|syscall.O_NOFOLLOW|syscall.O_CLOEXEC, 0600)
	}
	if err != nil {
		return nil, err
	}
	file := os.NewFile(uintptr(fd), path)
	fail := func(err error) (func(), error) { file.Close(); return nil, err }
	if err := checkPrivatePath(path, false); err != nil {
		return fail(err)
	}
	if err := syscall.Flock(fd, syscall.LOCK_EX|syscall.LOCK_NB); err != nil {
		return fail(errors.New("supervisor already owns this state directory"))
	}
	if !created {
		data, err := io.ReadAll(io.LimitReader(file, 64))
		if err != nil || string(data) != ownershipFormat {
			return fail(errors.New("legacy owner marker; explicitly stop the old supervisor before migrating"))
		}
	} else {
		if _, err := file.WriteString(ownershipFormat); err != nil {
			return fail(err)
		}
		if err := file.Sync(); err != nil {
			return fail(err)
		}
	}
	if _, err := os.Lstat(endpoint); err == nil {
		if err := checkPrivatePath(endpoint, false); err != nil {
			return fail(err)
		}
		if err := os.Remove(endpoint); err != nil {
			return fail(err)
		}
	} else if !os.IsNotExist(err) {
		return fail(err)
	}
	// Never unlink the lock inode: another process may already have opened it.
	return func() { _ = os.Remove(endpoint); _ = file.Close() }, nil
}
