//go:build !windows

package opencode

import "github.com/warmdev/modex/internal/hoststate"

func validatePrivateDirectory(path string) error { return hoststate.Validate(path, true) }

func validateProcessDirectory(path string, _ bool) error { return validatePrivateDirectory(path) }

func validatePrivateFile(path string) error { return hoststate.Validate(path, false) }
