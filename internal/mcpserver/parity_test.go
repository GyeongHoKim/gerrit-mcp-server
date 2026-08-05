package mcpserver

import (
	"slices"
	"strings"
	"testing"

	"github.com/google/go-cmp/cmp"

	"github.com/GyeongHoKim/gerrit-mcp-server/internal/cli"
)

// The two frontends are one product with one inventory, and this file is the
// only thing holding them to that.
//
// gerrit-cli has no written-down list of its own on purpose. Its command names
// are these tool names with underscores as dashes, so the contract already in
// inventory_test.go is the contract for both, and renaming a tool without
// renaming its command shows up here as a diff rather than in a bug report
// about a command that does not exist.
//
// Living in package mcpserver rather than in a third package is deliberate
// too: the lists it checks against are here, so a rename touches the contract
// and the assertion in one file. There is no import cycle to worry about --
// internal/cli imports gerrit, render, config and version, and never this
// package.

// commandName is the command gerrit-cli exposes for an MCP tool.
func commandName(tool string) string {
	return strings.ReplaceAll(tool, "_", "-")
}

func TestEveryToolHasACommand(t *testing.T) {
	t.Parallel()

	want := make([]string, 0, len(wantReadTools)+len(wantWriteTools))
	for _, tool := range slices.Concat(wantReadTools, wantWriteTools) {
		want = append(want, commandName(tool))
	}

	slices.Sort(want)

	got := make([]string, 0, len(cli.GerritCommands()))
	for _, command := range cli.GerritCommands() {
		got = append(got, command.Name)
	}

	slices.Sort(got)

	if diff := cmp.Diff(want, got); diff != "" {
		t.Errorf("command inventory mismatch (-want +got):\n%s", diff)
	}
}

// TestWriteToolsMapToWriteCommands is the half that costs something. A command
// left out of gerrit-cli's write set would run without GERRIT_ALLOW_WRITE,
// which is the exact failure the read/write split exists to prevent, and
// nothing else would notice: the command would work, on the wrong terms.
func TestWriteToolsMapToWriteCommands(t *testing.T) {
	t.Parallel()

	writes := make(map[string]bool, len(cli.GerritCommands()))
	for _, command := range cli.GerritCommands() {
		writes[command.Name] = command.Write
	}

	for _, tool := range wantWriteTools {
		name := commandName(tool)

		if !writes[name] {
			t.Errorf("%s is a write tool but %s is not gated on writes", tool, name)
		}
	}

	for _, tool := range wantReadTools {
		name := commandName(tool)

		if writes[name] {
			t.Errorf("%s is a read tool but %s is gated on writes", tool, name)
		}
	}
}
