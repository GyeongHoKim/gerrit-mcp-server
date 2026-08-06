# Troubleshooting

What a failure from Gerrit usually means, and what to do about it. Check the
exit code first — it narrows this down before you read anything.

## Exit 3 — not configured

`gerrit-cli config` names what is missing and where the rest came from. Either
the environment supplies `GERRIT_URL`, `GERRIT_USER` and `GERRIT_TOKEN`, or the
configuration file does.

The file lives under the OS configuration directory:

| OS | Path |
| --- | --- |
| Linux | `$XDG_CONFIG_HOME/gerrit-cli/config.json`, or `~/.config/gerrit-cli/config.json` |
| macOS | `~/Library/Application Support/gerrit-cli/config.json` |
| Windows | `%AppData%\gerrit-cli\config.json` |

`GERRIT_CONFIG` overrides the path. Environment variables always win over the
file, so a stale file cannot override a deliberate `GERRIT_TOKEN=... gerrit-cli`.

The user writes the file by running `gerrit-cli init` in their own terminal. An
agent should not run it — it reads the token from stdin.

## Exit 4 — not permitted

Three different causes, distinguishable from the message.

- **`GERRIT_ALLOW_WRITE` is not set.** The command modifies Gerrit and the
  opt-in is missing. Tell the user; do not set it yourself.
- **401 unauthorized.** The token is wrong, or it has expired. Tokens created
  on Gerrit 3.13 and newer can have a lifetime, and they expire silently — a
  session that worked yesterday failing today is usually this. The user
  generates a new one under *Settings → HTTP Credentials* and re-runs
  `gerrit-cli init`.
- **403 forbidden.** The account is authenticated but not allowed to do this on
  this project — a restricted repository, or a change owned by someone else on a
  host that limits who may abandon. Nothing about the command will fix it.

## Exit 5 — not found

Usually the change id, occasionally the file path or a draft id.

- Check the number against the Gerrit URL.
- A triplet must be passed exactly as Gerrit gave it. Project names contain `/`,
  and re-escaping an id that is already escaped 404s a change that exists.
- File paths come from `list-change-files` and are repository-relative. There is
  no leading `./` and no leading slash.
- Draft ids come from `list-draft-comments` and belong to one change.
- Some draft comment endpoints do not exist on Gerrit older than 3.12. If
  reading a change works but drafts 404, check the host's version.

## Exit 6 — conflict

The change is not in a state that allows what was asked.

- Abandoning an already-abandoned change, or one already merged.
- Reverting a change that was never submitted.
- Adding a group large enough that Gerrit wants `--confirm` first.
- Publishing drafts on a change that has moved on since they were staged.

Read the current state with `get-change-details` before deciding what to do.

## Exit 1 — everything else

- **Timeouts.** A very large diff can exceed the 30-second default. Raise it
  with `GERRIT_TIMEOUT=2m`, or fetch one file at a time rather than the whole
  change.
- **A response too large.** The same problem from the other end; narrow the
  query or the file.
- **Network and TLS.** An internal Gerrit behind a VPN fails here when the VPN
  is down, and the message will say so.

## Things that look like bugs and are not

- **Output goes to stdout, everything else to stderr.** Piping the output is
  safe; a usage message will never end up in the pipe.
- **`gerrit-cli help` lists write commands even when they are refused.** The
  commands exist; the opt-in is what is missing.
- **The underscore spelling works.** `query_changes` and `query-changes` are the
  same command, because the MCP server's tools are spelled with underscores.
- **Authenticated requests need an `/a/` prefix on Gerrit.** `gerrit-cli` adds
  it. If you are comparing against a hand-written `curl`, that is the difference.
