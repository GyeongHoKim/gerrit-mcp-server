# The review workflow

How drafts, sides, line numbers and reply ids fit together. Getting these wrong
is the usual reason a comment lands in the wrong place, or does not land at all.

## Drafts are staged, then published

`post-review-comment` creates a **draft**. Drafts are yours alone — nobody else
sees them and no notification goes out. They stay that way until
`publish-drafts` posts the whole batch as one review.

That means:

- Stage every comment first, then publish once. Publishing per comment sends a
  notification each time.
- `publish-drafts --message "..."` attaches a cover message to the review. Use
  it for the overall verdict; use the inline comments for specifics.
- Publishing cannot be undone. Before it, `delete-draft-comment --draft-id ...`
  discards one and `delete-draft-comments` discards all of them.
- `list-draft-comments` shows what is staged. Worth running before publishing,
  because a draft left on an old change is easy to forget.

## Which change

`--change-id` takes either form:

- **The number** from the URL — `12345`. Simplest, and unambiguous on one host.
- **The triplet** — `project~branch~Change-Id`, for example
  `platform/base~main~I8473b95934b5732ac55d26311a706c9c2bde9385`.

Project names routinely contain `/`. Pass the triplet exactly as Gerrit gave it
to you and let `gerrit-cli` handle the escaping; escaping it yourself produces a
404 for a change that plainly exists.

## Which line, on which side

Line numbers come from `get-file-diff`. Read the diff first — do not count lines
in the file on disk, because the patchset under review is not necessarily what
is checked out.

`--side` picks which version of the file the line number refers to:

- **`REVISION`** — the new file, after the change. This is the default, and it
  is what you want when commenting on code the change adds or modifies.
- **`PARENT`** — the old file, before the change. Use it to comment on a line
  the change deleted.

Omit `--line` entirely to comment on the file as a whole rather than on a line.

## Replying in a thread

`--in-reply-to` takes the id of the comment being answered. Ids come from
`list-change-comments`; they are not guessable and not stable across changes.

```bash
gerrit-cli list-change-comments --change-id 12345
gerrit-cli post-review-comment --change-id 12345 --file src/main.go \
    --in-reply-to 9f3c1a2b --message "good catch, fixed"
```

A reply inherits the thread's file and line, but `--file` is still required.

## Resolved and unresolved

`--unresolved` marks a comment as needing an answer. Gerrit shows unresolved
threads prominently and many projects will not submit a change while any remain
open. Use it for things that must change; leave it off for observations.

## Reviewers and CCs

`add-reviewer --state REVIEWER` (the default) adds someone expected to vote.
`--state CC` keeps them informed without asking anything of them.

Adding a group large enough that Gerrit asks for confirmation fails until you
pass `--confirm`. That refusal is deliberate — it means the group is big enough
that notifying all of them is a decision worth making on purpose.

## What this cannot do

Voting on labels (`+2`, `-1`), submitting, rebasing and uploading a patchset are
not available here. Publishing a review with a cover message is as far as it
goes; the vote itself belongs to the web UI, and a new patchset belongs to
`git push origin HEAD:refs/for/<branch>`.
