// Package mcpserver exposes the Gerrit client as Model Context Protocol tools.
//
// Handlers here are deliberately thin: they unpack arguments, call one method
// on the Gerrit client, and hand the result to the renderer. Anything more
// belongs in internal/gerrit or internal/render.
package mcpserver

import (
	"github.com/modelcontextprotocol/go-sdk/mcp"

	"github.com/GyeongHoKim/gerrit-mcp-server/internal/gerrit"
	"github.com/GyeongHoKim/gerrit-mcp-server/internal/version"
)

// ServerName is how this server identifies itself to MCP clients.
const ServerName = "gerrit"

// toolRegistrar installs one tool onto a server.
//
// Tools are collected in the two slices below rather than registered by an
// init function, so that reading either list tells you exactly what a given
// configuration exposes.
type toolRegistrar func(*server, *mcp.Server)

// readTools never modify Gerrit and are registered unconditionally.
var readTools = []toolRegistrar{
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

// writeTools modify Gerrit and are registered only when writes are allowed.
var writeTools = []toolRegistrar{
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

	for _, register := range readTools {
		register(s, srv)
	}

	if allowWrite {
		for _, register := range writeTools {
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
