package version

import "fmt"

// Set by the release build; local source builds remain identifiable.
var Version = "dev"
var Commit = "unknown"

func String(name string) string { return fmt.Sprintf("%s %s (%s)", name, Version, Commit) }
