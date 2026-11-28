package main

import (
	"context"
	"errors"
	"os"
	"os/exec"
	"time"
)

// Copy missing provider credentials through OpenCode's supported commands.
// The pipe never exposes them in arguments, terminal output, or files.
func seedNativeOpenCodeAuth(ctx context.Context, binary, server string, targetEnv []string) error {
	ctx, cancel := context.WithTimeout(ctx, 15*time.Second)
	defer cancel()
	reader, writer, err := os.Pipe()
	if err != nil {
		return errors.New("cannot connect OpenCode credential import")
	}
	defer reader.Close()
	defer writer.Close()
	source := exec.CommandContext(ctx, binary, "auth", "export")
	source.Env = os.Environ()
	source.Stdout = writer
	target := exec.CommandContext(ctx, binary, "auth", "import", "--server", server)
	target.Env = targetEnv
	target.Stdin = reader
	if err := target.Start(); err != nil {
		return errors.New("cannot initialize OpenCode provider import")
	}
	if err := source.Start(); err != nil {
		_ = writer.Close()
		_ = target.Wait()
		return errors.New("cannot read existing OpenCode providers")
	}
	_ = reader.Close()
	sourceErr := source.Wait()
	_ = writer.Close()
	targetErr := target.Wait()
	if sourceErr != nil || targetErr != nil {
		return errors.New("OpenCode provider import failed; check opencode auth list on the host")
	}
	return nil
}
