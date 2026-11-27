//go:build unix

package antigravity

import (
	"path/filepath"
	"syscall"
	"testing"
	"time"
)

func TestOpeningFIFOAfterPathRaceDoesNotBlock(t *testing.T) {
	path := filepath.Join(t.TempDir(), "fifo")
	if err := syscall.Mkfifo(path, 0600); err != nil {
		t.Fatal(err)
	}
	done := make(chan struct{})
	go func() {
		f, err := openAttachment(path)
		if err == nil {
			f.Close()
		}
		close(done)
	}()
	select {
	case <-done:
	case <-time.After(time.Second):
		t.Fatal("opening FIFO blocked")
	}
}
