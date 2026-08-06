package cli

import (
	"context"
	"flag"
	"fmt"
	"strings"

	"github.com/GyeongHoKim/gerrit-mcp-server/internal/gerrit"
	"github.com/GyeongHoKim/gerrit-mcp-server/internal/render"
)

// The commands here mirror the write tools in internal/mcpserver one for one.
// None of them runs unless the configuration allows writes; the dispatcher
// enforces that before a client is built, which is this frontend's counterpart
// of the server simply not registering the tool.

// writeCommands modify Gerrit and are refused unless writes are allowed.
//
// A function rather than a package-level slice, for the same reason
// mcpserver.writeTools is one: a var could be appended to from anywhere in the
// package, and the thing it would move is a destructive command into the set
// that runs without asking.
func writeCommands() []Command {
	return []Command{
		postReviewComment(),
		publishDrafts(),
		deleteDraftComment(),
		deleteDraftComments(),
		addReviewer(),
		setTopic(),
		setReadyForReview(),
		setWorkInProgress(),
		abandonChange(),
		revertChange(),
		createChange(),
		revertSubmission(),
	}
}

// mutating marks a command as one that modifies Gerrit.
//
// Spelled out at each constructor rather than applied in bulk by
// writeCommands, so that reading one command answers the only question anyone
// auditing this file has about it.
func mutating(command Command) Command {
	command.Write = true

	return command
}

// changeMessageCommand builds one of the commands that flips a change's state
// with an optional note. Five have exactly this shape.
func changeMessageCommand(
	name, summary, messageUsage string,
	act func(ctx context.Context, deps Deps, changeID, message string) error,
) Command {
	return mutating(Command{
		Name:    name,
		Summary: summary,
		Run: func(ctx context.Context, deps Deps, args []string) error {
			flags := newFlagSet(name, deps.Options.Stderr)
			changeID := changeIDFlag(flags)
			message := messageFlag(flags, messageUsage)

			if err := parse(flags, args); err != nil {
				return err
			}

			return act(ctx, deps, *changeID, *message)
		},
	})
}

// messageFlag binds --message, whose meaning differs enough between commands
// to be worth spelling out at each one.
func messageFlag(flags *flag.FlagSet, usage string) *string {
	return flags.String(flagMessage, "", usage)
}

// postReviewComment stages a draft comment.
func postReviewComment() Command {
	const name = "post-review-comment"

	return mutating(Command{
		Name:    name,
		Summary: "Stage a draft comment on a line. Publish it with publish-drafts.",
		Run: func(ctx context.Context, deps Deps, args []string) error {
			flags := newFlagSet(name, deps.Options.Stderr)
			changeID := changeIDFlag(flags)
			in := commentFlags(flags)

			if err := parse(flags, args); err != nil {
				return err
			}

			draft, err := deps.Gerrit.CreateDraftComment(ctx, *changeID, in())
			if err != nil {
				return fmt.Errorf("staging a draft comment on %s: %w", *changeID, err)
			}

			return emit(deps.Options.Stdout, render.DraftCreated(draft))
		},
	})
}

// commentFlags binds the fields of a draft comment and returns a reader for
// them, so that post-review-comment stays inside the complexity budget.
func commentFlags(flags *flag.FlagSet) func() *gerrit.CommentInput {
	file := flags.String(flagFile, "",
		"path of the file to comment on, as returned by list-change-files")
	message := messageFlag(flags, "the comment text")
	inReplyTo := flags.String("in-reply-to", "",
		"id of the comment being answered, as returned by list-change-comments")
	side := flags.String("side", "",
		"REVISION for the new file (default) or PARENT for the old one")
	line := flags.Int("line", 0,
		"line number from get-file-diff; omit to comment on the file as a whole")
	unresolved := flags.Bool("unresolved", false, "mark the comment as needing a reply")

	return func() *gerrit.CommentInput {
		return &gerrit.CommentInput{
			Path:       *file,
			Side:       *side,
			Message:    *message,
			InReplyTo:  *inReplyTo,
			Line:       *line,
			Unresolved: *unresolved,
		}
	}
}

// publishDrafts publishes the staged comments.
func publishDrafts() Command {
	const name = "publish-drafts"

	return mutating(Command{
		Name:    name,
		Summary: "Publish your staged drafts. This cannot be undone.",
		Run: func(ctx context.Context, deps Deps, args []string) error {
			flags := newFlagSet(name, deps.Options.Stderr)
			changeID := changeIDFlag(flags)
			message := messageFlag(flags, "cover message posted alongside the comments")
			allRevisions := flags.Bool("all-revisions", false,
				"also publish drafts left on earlier patch sets; defaults to the current one")

			if err := parse(flags, args); err != nil {
				return err
			}

			if _, err := deps.Gerrit.PublishDrafts(ctx, *changeID, *message, *allRevisions); err != nil {
				return fmt.Errorf("publishing drafts on %s: %w", *changeID, err)
			}

			return emit(deps.Options.Stdout, render.DraftsPublished(*changeID, *allRevisions))
		},
	})
}

// deleteDraftComment discards one staged comment.
func deleteDraftComment() Command {
	const name = "delete-draft-comment"

	return mutating(Command{
		Name:    name,
		Summary: "Discard one staged draft comment. This cannot be undone.",
		Run: func(ctx context.Context, deps Deps, args []string) error {
			flags := newFlagSet(name, deps.Options.Stderr)
			changeID := changeIDFlag(flags)
			draftID := flags.String("draft-id", "",
				"id of the draft to discard, as returned by list-draft-comments")

			if err := parse(flags, args); err != nil {
				return err
			}

			if err := deps.Gerrit.DeleteDraftComment(ctx, *changeID, *draftID); err != nil {
				return fmt.Errorf("discarding draft %s on %s: %w", *draftID, *changeID, err)
			}

			return emit(deps.Options.Stdout, render.DraftDeleted(*changeID, *draftID))
		},
	})
}

// deleteDraftComments discards every staged comment on a change.
func deleteDraftComments() Command {
	return mutating(changeCommand(
		"delete-draft-comments",
		"Discard every draft you have staged. This cannot be undone.",
		func(ctx context.Context, deps Deps, changeID string) error {
			deleted, err := deps.Gerrit.DeleteAllDraftComments(ctx, changeID)
			if err != nil {
				// The count says how far it got. Reporting only the failure
				// would hide deletions that already happened and cannot be
				// undone.
				return fmt.Errorf("discarding drafts on %s after removing %d: %w", changeID, deleted, err)
			}

			return emit(deps.Options.Stdout, render.DraftsDeleted(changeID, deleted))
		},
	))
}

// addReviewer adds one reviewer or CC.
func addReviewer() Command {
	const name = "add-reviewer"

	return mutating(Command{
		Name:    name,
		Summary: "Add a reviewer or CC. Accepts a person or a group.",
		Run: func(ctx context.Context, deps Deps, args []string) error {
			flags := newFlagSet(name, deps.Options.Stderr)
			changeID := changeIDFlag(flags)
			reviewer := flags.String("reviewer", "",
				"account id, username, email or group name, as returned by suggest-reviewers")
			state := flags.String("state", "",
				"REVIEWER for someone expected to vote (default) or CC to only keep them informed")
			confirm := flags.Bool("confirm", false,
				"confirm adding a group large enough that Gerrit asks first")

			if err := parse(flags, args); err != nil {
				return err
			}

			result, err := deps.Gerrit.AddReviewer(ctx, *changeID, gerrit.ReviewerInput{
				Reviewer:  *reviewer,
				State:     *state,
				Confirmed: *confirm,
			})
			if err != nil {
				return fmt.Errorf("adding %s to %s: %w", *reviewer, *changeID, err)
			}

			return emit(deps.Options.Stdout, render.ReviewerAdded(*changeID, result))
		},
	})
}

// setTopic sets or clears a change's topic.
func setTopic() Command {
	const name = "set-topic"

	return mutating(Command{
		Name:    name,
		Summary: "Set the topic, or clear it by passing an empty one.",
		Run: func(ctx context.Context, deps Deps, args []string) error {
			flags := newFlagSet(name, deps.Options.Stderr)
			changeID := changeIDFlag(flags)
			topic := flags.String(flagTopic, "", "the new topic; pass an empty string to clear it")

			if err := parse(flags, args); err != nil {
				return err
			}

			trimmed := strings.TrimSpace(*topic)

			if err := deps.Gerrit.SetTopic(ctx, *changeID, trimmed); err != nil {
				return fmt.Errorf("setting the topic on %s: %w", *changeID, err)
			}

			return emit(deps.Options.Stdout, render.TopicSet(*changeID, trimmed))
		},
	})
}

// setReadyForReview marks a change ready.
func setReadyForReview() Command {
	return changeMessageCommand(
		"set-ready-for-review",
		"Take a change out of work-in-progress and notify its reviewers.",
		"optional note posted on the change explaining the action",
		func(ctx context.Context, deps Deps, changeID, message string) error {
			if err := deps.Gerrit.SetReadyForReview(ctx, changeID, message); err != nil {
				return fmt.Errorf("marking %s ready for review: %w", changeID, err)
			}

			return emit(deps.Options.Stdout, render.ReadyForReviewSet(changeID))
		},
	)
}

// setWorkInProgress marks a change work-in-progress.
func setWorkInProgress() Command {
	return changeMessageCommand(
		"set-work-in-progress",
		"Mark a change work-in-progress so it stops asking for attention.",
		"optional note posted on the change explaining the action",
		func(ctx context.Context, deps Deps, changeID, message string) error {
			if err := deps.Gerrit.SetWorkInProgress(ctx, changeID, message); err != nil {
				return fmt.Errorf("marking %s work-in-progress: %w", changeID, err)
			}

			return emit(deps.Options.Stdout, render.WorkInProgressSet(changeID))
		},
	)
}

// abandonChange abandons a change.
func abandonChange() Command {
	return changeMessageCommand(
		"abandon-change",
		"Abandon a change. gerrit-cli cannot restore it.",
		"optional note posted on the change explaining why",
		func(ctx context.Context, deps Deps, changeID, message string) error {
			change, err := deps.Gerrit.AbandonChange(ctx, changeID, message)
			if err != nil {
				return fmt.Errorf("abandoning %s: %w", changeID, err)
			}

			return emit(deps.Options.Stdout, render.ChangeAbandoned(change))
		},
	)
}

// revertChange creates a change that undoes another.
func revertChange() Command {
	return changeMessageCommand(
		"revert-change",
		"Open a change reverting a merged one. The revert still needs review.",
		"optional note for the revert's commit message",
		func(ctx context.Context, deps Deps, changeID, message string) error {
			revert, err := deps.Gerrit.RevertChange(ctx, changeID, message)
			if err != nil {
				return fmt.Errorf("reverting %s: %w", changeID, err)
			}

			return emit(deps.Options.Stdout, render.ChangeReverted(changeID, revert))
		},
	)
}

// revertSubmission reverts a whole submission.
func revertSubmission() Command {
	return changeMessageCommand(
		"revert-submission",
		"Open changes reverting everything submitted alongside this one.",
		"optional note for the reverts' commit messages",
		func(ctx context.Context, deps Deps, changeID, message string) error {
			reverts, err := deps.Gerrit.RevertSubmission(ctx, changeID, message)
			if err != nil {
				return fmt.Errorf("reverting the submission containing %s: %w", changeID, err)
			}

			return emit(deps.Options.Stdout, render.SubmissionReverted(changeID, reverts))
		},
	)
}

// createChange opens a new change.
func createChange() Command {
	const name = "create-change"

	return mutating(Command{
		Name:    name,
		Summary: "Open a new empty change for an edit to be published to.",
		Run: func(ctx context.Context, deps Deps, args []string) error {
			flags := newFlagSet(name, deps.Options.Stderr)
			project := flags.String("project", "", "the Gerrit project (repository) name")
			branch := flags.String("branch", "", "target branch, without the refs/heads/ prefix")
			subject := flags.String("subject", "", "first line of the commit message")
			topic := flags.String(flagTopic, "", "optional topic to group this change with others")
			workInProgress := flags.Bool("work-in-progress", false,
				"open the change without asking for review yet")

			if err := parse(flags, args); err != nil {
				return err
			}

			change, err := deps.Gerrit.CreateChange(ctx, gerrit.ChangeInput{
				Project:        *project,
				Branch:         *branch,
				Subject:        *subject,
				Topic:          *topic,
				WorkInProgress: *workInProgress,
			})
			if err != nil {
				return fmt.Errorf("creating a change on %s: %w", *project, err)
			}

			return emit(deps.Options.Stdout, render.ChangeCreated(change))
		},
	})
}
