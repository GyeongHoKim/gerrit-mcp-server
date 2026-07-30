// Package version reports build information stamped in at link time.
package version

import "fmt"

// These are overridden at build time with -ldflags "-X <pkg>.Version=...".
// The release pipeline derives all three from the git tag being built, so a
// published binary always reports the same version as its npm package.
var (
	// Version is the semantic version of this build.
	Version = "dev"
	// Commit is the git revision this build was produced from.
	Commit = "none"
	// Date is the RFC 3339 timestamp of this build.
	Date = "unknown"
)

// String renders the build information as a single human-readable line.
func String() string {
	return fmt.Sprintf("gerrit-mcp-server %s (commit %s, built %s)", Version, Commit, Date)
}
