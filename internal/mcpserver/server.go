// Package mcpserver exposes the Gerrit client as Model Context Protocol tools.
//
// Handlers here are deliberately thin: they unpack arguments, call one method
// on the Gerrit client, and hand the result to the renderer. Anything more
// belongs in internal/gerrit or internal/render.
package mcpserver

import (
	"slices"

	"github.com/modelcontextprotocol/go-sdk/mcp"

	"github.com/GyeongHoKim/gerrit-mcp-server/internal/gerrit"
	"github.com/GyeongHoKim/gerrit-mcp-server/internal/version"
)

// ServerName is how this server identifies itself to MCP clients.
const ServerName = "gerrit"

// toolRegistrar installs one tool onto a server.
//
// Tools are collected in the two functions below rather than registered by an
// init function, so that reading either list tells you exactly what a given
// configuration exposes.
type toolRegistrar func(*server, *mcp.Server)

// readTools never modify Gerrit and are registered unconditionally.
//
// A function rather than a package-level slice: the read/write split is the
// boundary GERRIT_ALLOW_WRITE exists to enforce, and a var could be appended
// to from anywhere in the package, moving a destructive tool into the set a
// client is entitled to run without asking.
func readTools() []toolRegistrar {
	return []toolRegistrar{
		registerQueryChanges,
		registerGetChangeDetails,
		registerGetCommitMessage,
		registerListChangeFiles,
		registerGetFileDiff,
		registerListChangeComments,
		registerListDraftComments,
		registerChangesSubmittedTogether,
		registerSuggestReviewers,
		registerGetBugsFromCL,
	}
}

// writeTools modify Gerrit and are registered only when writes are allowed.
func writeTools() []toolRegistrar {
	return []toolRegistrar{
		registerPostReviewComment,
		registerPublishDrafts,
		registerDeleteDraftComment,
		registerDeleteDraftComments,
		registerAddReviewer,
		registerSetTopic,
		registerSetReadyForReview,
		registerSetWorkInProgress,
		registerAbandonChange,
		registerRevertChange,
		registerCreateChange,
		registerRevertSubmission,
	}
}

// minVersions is the Gerrit release each tool needs, for the few that need
// one at all. A tool absent from the map works on every supported version.
//
// The releases themselves are declared in internal/gerrit beside the endpoints
// they describe, because they are facts about the Gerrit API rather than about
// this server. What lives here is only which tool calls which endpoint, and
// parity_test.go holds that equal to gerrit-cli's copy of the same claim.
//
// A function rather than a package-level map, for the reason readTools is one.
func minVersions() map[string]gerrit.ServerVersion {
	return map[string]gerrit.ServerVersion{
		"set_work_in_progress": gerrit.MinVersionWorkInProgress,
		"set_ready_for_review": gerrit.MinVersionWorkInProgress,
		"revert_submission":    gerrit.MinVersionRevertSubmission,
	}
}

// UnsupportedTools names the tools a Gerrit of this release cannot serve.
//
// The zero version determined nothing and hides nothing. Offering a tool that
// fails with an explanation is better than hiding one that would have worked,
// and an enterprise fork that backported an endpoint reports whatever release
// it forked from.
func UnsupportedTools(running gerrit.ServerVersion) []string {
	if running.IsZero() {
		return nil
	}

	var unsupported []string

	for tool, needs := range minVersions() {
		if running.Before(needs) {
			unsupported = append(unsupported, tool)
		}
	}

	// Sorted so that the log line a reader compares between two runs does not
	// depend on which key Go's map iteration happened to reach first.
	slices.Sort(unsupported)

	return unsupported
}

// server carries what the tool handlers need.
type server struct {
	gerrit *gerrit.Client
}

// New builds an MCP server exposing the Gerrit tools.
//
// Read tools are always registered. Write tools are registered only when
// allowWrite is true, so an agent cannot abandon a change on a host the
// operator meant to expose read-only.
func New(client *gerrit.Client, allowWrite bool) *mcp.Server {
	srv := mcp.NewServer(&mcp.Implementation{
		Name:    ServerName,
		Version: version.Version,
	}, nil)

	s := &server{gerrit: client}

	for _, register := range readTools() {
		register(s, srv)
	}

	if allowWrite {
		for _, register := range writeTools() {
			register(s, srv)
		}
	}

	return srv
}

// text wraps a rendered string as a tool result.
//
// Every tool answers through here, so none of them can hand the model raw
// Gerrit JSON by accident.
func text(body string) *mcp.CallToolResult {
	return &mcp.CallToolResult{
		Content: []mcp.Content{&mcp.TextContent{Text: body}},
	}
}
