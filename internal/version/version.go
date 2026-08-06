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

// ServerName is the program name gerrit-mcp-server reports.
const ServerName = "gerrit-mcp-server"

// StringFor renders the build information for the named program as a single
// human-readable line.
//
// The name is a parameter because two binaries are built from these stamps and
// a binary that reports another program's name is a binary nobody can file a
// useful bug against.
func StringFor(name string) string {
	return fmt.Sprintf("%s %s (commit %s, built %s)", name, Version, Commit, Date)
}

// String renders the build information for gerrit-mcp-server.
func String() string {
	return StringFor(ServerName)
}
