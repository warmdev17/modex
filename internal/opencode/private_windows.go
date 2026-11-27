//go:build windows

package opencode

import "errors"

func validatePrivateDirectory(string) error {
	return errors.New("private state is unsupported on Windows")
}

func validateProcessDirectory(path string, temporary bool) error {
	if temporary {
		return nil
	}
	return validatePrivateDirectory(path)
}

func validatePrivateFile(string) error { return errors.New("private state is unsupported on Windows") }
