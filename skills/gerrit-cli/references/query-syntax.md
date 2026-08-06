# Gerrit query syntax

What `gerrit-cli query-changes --query "..."` accepts. This is the one thing the
binary's own help cannot teach you, because the syntax belongs to Gerrit.

Terms are ANDed by default. Quote the whole query in the shell.

## The operators worth knowing

| Operator | Matches |
| --- | --- |
| `is:open` / `is:closed` | changes still under review, or not |
| `is:merged` / `is:abandoned` | how a closed change ended |
| `is:wip` | work-in-progress, not asking for review |
| `status:open` | the same as `is:open`; both spellings are used |
| `owner:self` | changes you uploaded |
| `reviewer:self` | changes you are a reviewer on |
| `cc:self` | changes you are only kept informed about |
| `assignee:self` | changes assigned to you, on hosts that use assignees |
| `owner:alice@example.com` | anyone, by email, username or account id |
| `project:platform/base` | one repository |
| `branch:main` | one branch |
| `topic:release-blockers` | one topic |
| `file:^.*\.go` | changes touching files matching a regex |
| `message:"fix the parser"` | text in the commit message |
| `label:Code-Review=+2` | a specific vote |
| `label:Verified=-1` | a failing verification |
| `age:2d` | untouched for at least two days |
| `before:2026-01-01` / `after:2026-01-01` | last updated in a window |
| `limit:20` | cap in the query itself, as an alternative to `--limit` |

## Combining them

- Space means AND — `is:open project:platform/base`.
- `OR` gives alternatives — `is:merged OR is:abandoned`. Gerrit takes `or` in
  lower case as a synonym, but the upper case reads better beside the operators.
- `-` negates — `-owner:self`.
- Parentheses group — `is:open AND (owner:self OR reviewer:self)`.

## Queries worth having to hand

```bash
# waiting on me, excluding my own changes
gerrit-cli query-changes --query "is:open reviewer:self -owner:self"

# mine, still open, not yet ready for anyone else
gerrit-cli query-changes --query "is:open owner:self is:wip"

# open in one repository, already approved, so probably ready to submit
gerrit-cli query-changes --query "is:open project:platform/base label:Code-Review=+2"

# stale: open, mine, and untouched for a fortnight
gerrit-cli query-changes --query "is:open owner:self age:14d"

# everything under one topic, whatever its state
gerrit-cli query-changes --query "topic:release-blockers"
```

## Notes

- `self` needs an authenticated request. It works because `gerrit-cli` always
  authenticates; it will not work in a browser you are logged out of.
- Results are capped. When the output says more were not returned, narrow the
  query rather than raising `--limit` indefinitely — a broad query on a busy
  host is slow for Gerrit as well as for you.
- Not every operator exists on every Gerrit version. `assignee:` in particular
  was removed on newer hosts. If a query errors, drop the unusual term first.
