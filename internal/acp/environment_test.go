package acp

import (
	"slices"
	"testing"
)

func TestEnvironmentKeepsAGYLoginInPersistentFileStorage(t *testing.T) {
	for _, env := range [][]string{{"HOME=/user"}, {"HOME=/user", "AGY_CLI_FORCE_FILE_TOKEN_STORAGE=0"}} {
		got := Environment(env)
		if !slices.Contains(got, "AGY_CLI_FORCE_FILE_TOKEN_STORAGE=1") || slices.Contains(got, "AGY_CLI_FORCE_FILE_TOKEN_STORAGE=0") {
			t.Fatalf("persistent auth environment = %v", got)
		}
		if !slices.Contains(got, "HOME=/user") {
			t.Fatal("lost host home")
		}
	}
}
