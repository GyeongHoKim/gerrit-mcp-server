package cli

import (
	"context"
	"fmt"

	"github.com/GyeongHoKim/gerrit-mcp-server/internal/render"
)

// The commands here mirror the read tools in internal/mcpserver one for one,
// down to the wording of the errors they wrap: the two frontends are the same
// product and a failure should read the same whichever one reported it.

// queryChanges searches for changes and renders the matches.
func queryChanges() Command {
	const name = "query-changes"

	return Command{
		Name:    name,
		Summary: "Search changes with Gerrit query syntax.",
		Run: func(ctx context.Context, deps Deps, args []string) error {
			flags := newFlagSet(name, deps.Options.Stderr)
			query := flags.String(flagQuery, "",
				"Gerrit search expression, for example 'status:open owner:self'")
			limit := flags.Int(flagLimit, 0,
				"maximum number of changes to return; omit for the server default")

			if err := parse(flags, args); err != nil {
				return err
			}

			changes, err := deps.Gerrit.QueryChanges(ctx, *query, *limit)
			if err != nil {
				return fmt.Errorf("querying changes: %w", err)
			}

			return emit(deps.Options.Stdout, render.Changes(changes))
		},
	}
}

// getChangeDetails retrieves one change and renders its review state.
func getChangeDetails() Command {
	return changeCommand(
		"get-change-details",
		"Show one change with its status, labels and reviewers.",
		func(ctx context.Context, deps Deps, changeID string) error {
			detail, err := deps.Gerrit.GetChange(ctx, changeID)
			if err != nil {
				return fmt.Errorf("fetching change %s: %w", changeID, err)
			}

			return emit(deps.Options.Stdout, render.ChangeDetail(detail))
		},
	)
}

// getCommitMessage retrieves the commit message of a change.
func getCommitMessage() Command {
	return changeCommand(
		"get-commit-message",
		"Show the commit message of the current patch set.",
		func(ctx context.Context, deps Deps, changeID string) error {
			message, err := deps.Gerrit.GetCommitMessage(ctx, changeID)
			if err != nil {
				return fmt.Errorf("fetching commit message for %s: %w", changeID, err)
			}

			return emit(deps.Options.Stdout, render.CommitMessage(message))
		},
	)
}

// listChangeFiles lists the files a change touches.
func listChangeFiles() Command {
	return changeCommand(
		"list-change-files",
		"List the files the current patch set modifies.",
		func(ctx context.Context, deps Deps, changeID string) error {
			files, err := deps.Gerrit.ListFiles(ctx, changeID)
			if err != nil {
				return fmt.Errorf("listing files for %s: %w", changeID, err)
			}

			return emit(deps.Options.Stdout, render.Files(files))
		},
	)
}

// getFileDiff retrieves and renders one file's diff.
func getFileDiff() Command {
	const name = "get-file-diff"

	return Command{
		Name:    name,
		Summary: "Show the diff of one file in a change.",
		Run: func(ctx context.Context, deps Deps, args []string) error {
			flags := newFlagSet(name, deps.Options.Stderr)
			changeID := changeIDFlag(flags)
			file := flags.String(flagFile, "",
				"path of the file within the change, as returned by list-change-files")

			if err := parse(flags, args); err != nil {
				return err
			}

			diff, err := deps.Gerrit.GetFileDiff(ctx, *changeID, *file)
			if err != nil {
				return fmt.Errorf("fetching diff of %s in %s: %w", *file, *changeID, err)
			}

			return emit(deps.Options.Stdout, render.Diff(*file, diff))
		},
	}
}

// listChangeComments lists the published comments on a change.
func listChangeComments() Command {
	return changeCommand(
		"list-change-comments",
		"List the published review comments, grouped by file.",
		func(ctx context.Context, deps Deps, changeID string) error {
			byFile, err := deps.Gerrit.ListComments(ctx, changeID)
			if err != nil {
				return fmt.Errorf("listing comments on %s: %w", changeID, err)
			}

			return emit(deps.Options.Stdout, render.Comments(byFile))
		},
	)
}

// listDraftComments lists the calling account's unpublished drafts.
func listDraftComments() Command {
	return changeCommand(
		"list-draft-comments",
		"List your own unpublished draft comments.",
		func(ctx context.Context, deps Deps, changeID string) error {
			byFile, err := deps.Gerrit.ListDraftComments(ctx, changeID)
			if err != nil {
				return fmt.Errorf("listing draft comments on %s: %w", changeID, err)
			}

			return emit(deps.Options.Stdout, render.Drafts(byFile))
		},
	)
}

// changesSubmittedTogether lists the changes that submit alongside one.
func changesSubmittedTogether() Command {
	return changeCommand(
		"changes-submitted-together",
		"List the changes that would submit alongside this one.",
		func(ctx context.Context, deps Deps, changeID string) error {
			changes, err := deps.Gerrit.ChangesSubmittedTogether(ctx, changeID)
			if err != nil {
				return fmt.Errorf("computing changes submitted with %s: %w", changeID, err)
			}

			return emit(deps.Options.Stdout, render.SubmittedTogether(changes))
		},
	)
}

// suggestReviewers asks Gerrit for reviewer suggestions.
func suggestReviewers() Command {
	const name = "suggest-reviewers"

	return Command{
		Name:    name,
		Summary: "Ask Gerrit who could review a change.",
		Run: func(ctx context.Context, deps Deps, args []string) error {
			flags := newFlagSet(name, deps.Options.Stderr)
			changeID := changeIDFlag(flags)
			query := flags.String(flagQuery, "",
				"text matched against names, emails and group names; omit for Gerrit's own ranking")
			limit := flags.Int(flagLimit, 0, "maximum number of suggestions to return")

			if err := parse(flags, args); err != nil {
				return err
			}

			suggestions, err := deps.Gerrit.SuggestReviewers(ctx, *changeID, *query, *limit)
			if err != nil {
				return fmt.Errorf("suggesting reviewers for %s: %w", *changeID, err)
			}

			return emit(deps.Options.Stdout, render.Reviewers(suggestions))
		},
	}
}

// getBugsFromCL extracts issue references from a change's commit message.
func getBugsFromCL() Command {
	return changeCommand(
		"get-bugs-from-cl",
		"Extract the issue references from the commit message.",
		func(ctx context.Context, deps Deps, changeID string) error {
			message, err := deps.Gerrit.GetCommitMessage(ctx, changeID)
			if err != nil {
				return fmt.Errorf("fetching commit message for %s: %w", changeID, err)
			}

			return emit(deps.Options.Stdout, render.Bugs(message.Bugs()))
		},
	)
}
