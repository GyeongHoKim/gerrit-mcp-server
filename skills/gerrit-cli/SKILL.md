---
name: gerrit-cli
description: Work with Gerrit code review from the terminal using the gerrit-cli binary - search changes, read diffs and inline comments, draft and publish review comments, add reviewers, set topic or WIP, abandon and revert. Use whenever the user mentions Gerrit, a CL or change number, a Change-Id, a gerrit.* URL, a patchset, or asks what code reviews are waiting on them. Not for GitHub or GitLab pull requests.
---

# gerrit-cli

`gerrit-cli` is a binary that talks to a Gerrit code review host. Every answer is
plain text on stdout, already compacted for reading — there is no JSON mode and
you do not need one.

## Preflight

Do this once per session, before the first real command.

1. **`gerrit-cli --version`.** If the command is not found, tell the user to run
   `npm i -g @gyeonghokim/gerrit-cli`, or use `npx -y @gyeonghokim/gerrit-cli`
   in place of `gerrit-cli` everywhere below.
2. **`gerrit-cli config`.** It prints every setting and where it came from, and
   names what is missing. If it reports missing values, or any command exits 3:
   - Ask the user for their Gerrit URL and username.
   - **Tell the user to run `gerrit-cli init` themselves, in their terminal.**
     Do not run it yourself: it asks for an auth token on stdin and will refuse
     a non-terminal anyway.
   - The token comes from *Settings → HTTP Credentials* in Gerrit.

Never invent a token. Never write a token into a file in the repository.

Run **`gerrit-cli doctor`** only when you need it: a command exits 4 saying the
host is too old, or the user is on a Gerrit you have not seen before. It reports
the host's release and which commands that rules out, in one request.

## Command shape

Subcommands are the words you would expect, with dashes: `query-changes`,
`get-file-diff`, `post-review-comment`. **Every value is a flag** — there are no
positional arguments, and a stray one is rejected rather than ignored.

Run `gerrit-cli help` for the full list and `gerrit-cli help <command>` for one
command's flags. Those are authoritative; prefer them over guessing.

A change is named by `--change-id`, which takes either the number from the URL
(`12345`) or the triplet `project~branch~Change-Id`.

## Recipes

**What is waiting on me**

```bash
gerrit-cli query-changes --query "is:open reviewer:self -owner:self"
```

**Read a change**

```bash
gerrit-cli get-change-details --change-id 12345
gerrit-cli list-change-files  --change-id 12345
gerrit-cli get-file-diff      --change-id 12345 --file src/main.go
```

**Leave a review.** Comments are staged as drafts and are invisible to anyone
else until published, so stage all of them first and publish once.

```bash
gerrit-cli post-review-comment --change-id 12345 \
    --file src/main.go --line 42 --message "this shadows the outer err"
gerrit-cli publish-drafts --change-id 12345 --message "one nit, otherwise good"
```

**Reply in a thread.** Get the comment id from the listing first.

```bash
gerrit-cli list-change-comments --change-id 12345
gerrit-cli post-review-comment  --change-id 12345 --file src/main.go \
    --in-reply-to 9f3c1a2b --message "agreed, fixed in patchset 3"
```

**Find and add reviewers**

```bash
gerrit-cli suggest-reviewers --change-id 12345
gerrit-cli add-reviewer      --change-id 12345 --reviewer bob@example.com
```

**Hand a change over**

```bash
gerrit-cli set-ready-for-review --change-id 12345
gerrit-cli set-work-in-progress --change-id 12345
gerrit-cli set-topic --change-id 12345 --topic release-blockers
```

## Write safety

Commands that modify Gerrit run only when `GERRIT_ALLOW_WRITE=true` is set. If
one is refused, say so and let the user decide — do not set the variable on
their behalf.

`publish-drafts`, `abandon-change`, `revert-change` and `revert-submission`
cannot be undone, and other people are notified. Confirm with the user before
running any of them.

## Exit codes

The status says what to do next. Read it before retrying.

| Code | Meaning | What to do |
| --- | --- | --- |
| 0 | success | — |
| 1 | something else failed | read the message on stderr |
| 2 | bad arguments | fix the command line; check `gerrit-cli help <command>` |
| 3 | not configured | run the preflight above |
| 4 | not permitted | ask the user; do not retry |
| 5 | not found | wrong change id, file path or comment id |
| 6 | conflict | the change is not in a state that allows this |

## Limitations

`gerrit-cli` cannot vote on labels (no `+2` or `-1`), cannot submit or rebase a
change, and cannot upload a patchset. For those, point the user at the web UI or
at `git push origin HEAD:refs/for/<branch>`.

Three commands need a Gerrit newer than the 2.14 floor and exit 4 on an older
host, naming the release they need:

- `set-work-in-progress` and `set-ready-for-review` need **2.15+**
- `revert-submission` needs **3.2+**

Nothing about the command will fix that; point the user at the web UI. Run
`gerrit-cli doctor` if you want the full picture for their host.

## Further reading

- [Gerrit query syntax](references/query-syntax.md) — the operators
  `query-changes` accepts. Read this before writing a non-obvious query.
- [The review workflow](references/review-workflow.md) — how drafts, sides,
  line numbers and reply ids fit together.
- [Troubleshooting](references/troubleshooting.md) — what a 401, 403 or 404
  from Gerrit actually means.
