//go:build !unix

package antigravity

import "os"

func openAttachment(path string) (*os.File, error) { return os.Open(path) }
