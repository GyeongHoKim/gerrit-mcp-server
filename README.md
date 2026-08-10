# gerrit-mcp-server

[한국어 문서](README.ko.md)

[![CI](https://github.com/GyeongHoKim/gerrit-mcp-server/actions/workflows/ci.yml/badge.svg)](https://github.com/GyeongHoKim/gerrit-mcp-server/actions/workflows/ci.yml)
[![npm](https://img.shields.io/npm/v/@gyeonghokim/gerrit-mcp-server)](https://www.npmjs.com/package/@gyeonghokim/gerrit-mcp-server)
[![Go](https://img.shields.io/badge/go-1.26-00ADD8)](https://go.dev)
[![License](https://img.shields.io/badge/license-Elastic--2.0-005571)](LICENSE)

Connect your AI coding agent to Gerrit code review.

Ask your agent to find the changes waiting on you, read a diff, draft line comments, and publish a
review — without leaving the session and without pasting code review comments back and forth.

It ships as **two frontends over the same code**, so you can pick how much of your agent's context
you want to spend:

| | What it is | Context cost |
| --- | --- | --- |
| **`gerrit-cli` + skill** | A command-line binary, plus an [agent skill](skills/gerrit-cli/SKILL.md) that teaches an agent to drive it | One line, until the skill triggers |
| **`gerrit-mcp-server`** | A [Model Context Protocol](https://modelcontextprotocol.io) server over **stdio** | 22 tool schemas, for the whole session |

The skill route is the lighter one and works with any agent that reads skills. The MCP server needs
no shell access and works with any MCP client: Claude Code, Codex, Cursor, Zed, Continue, or your
own.

Either way it is a single static binary with no runtime dependencies. You self-host it; nothing is
sent anywhere except to the Gerrit host you configure.

## Quick start

Both routes start the same way.

**Create a Gerrit auth token.** In Gerrit, go to *Settings → HTTP Credentials* and generate one.
See [Credentials](#credentials) below if your Gerrit is older.

Node is needed only so that `npm` or `npx` can fetch the right binary for your machine. The binaries
are Go and have no Node runtime dependency.

### Route A — `gerrit-cli` + agent skill

The lighter one. Nothing sits in your agent's context until it needs Gerrit.

**1. Install the binary.**

```bash
npm i -g @gyeonghokim/gerrit-cli
```

**2. Install the skill.** This works for Claude Code, Codex, Cursor and many others.

```bash
npx skills add GyeongHoKim/gerrit-mcp-server
```

**3. Configure it.** Run this yourself in a terminal — it asks for your token on stdin, and will
refuse to run where nothing can type into it.

```bash
gerrit-cli init
```

**4. Check it.** `gerrit-cli config` reports every setting and where it came from, naming anything
still missing. Then ask your agent: *"What’s the verified score for XXX Change Id’s review?"*

To allow the commands that modify Gerrit, set `GERRIT_ALLOW_WRITE=true` — see
[Available tools and commands](#available-tools-and-commands).

You can also use the CLI on its own, without an agent:

```bash
gerrit-cli query-changes --query "is:open reviewer:self -owner:self"
gerrit-cli get-file-diff --change-id 12345 --file src/main.go
gerrit-cli help
```

### Route B — MCP server

No shell access needed, and it works with any MCP client.

**Add the server to your client.**

<details open>
<summary><b>Claude Code</b></summary>

```bash
claude mcp add gerrit \
  --env GERRIT_URL=https://gerrit.example.com \
  --env GERRIT_USER=your-username \
  --env GERRIT_TOKEN=your-token \
  -- npx -y @gyeonghokim/gerrit-mcp-server
```

</details>

<details>
<summary><b>Codex</b></summary>

Add this to `~/.codex/config.toml`, or to `.codex/config.toml` for a single trusted project:

```toml
[mcp_servers.gerrit]
command = "npx"
args = ["-y", "@gyeonghokim/gerrit-mcp-server"]
# npx downloads the binary on first run, which can exceed the 10s default.
startup_timeout_sec = 60

[mcp_servers.gerrit.env]
GERRIT_URL = "https://gerrit.example.com"
GERRIT_USER = "your-username"
GERRIT_TOKEN = "your-token"
```

Or let the CLI write it for you:

```bash
codex mcp add gerrit \
  --env GERRIT_URL=https://gerrit.example.com \
  --env GERRIT_USER=your-username \
  --env GERRIT_TOKEN=your-token \
  -- npx -y @gyeonghokim/gerrit-mcp-server
```

</details>

<details>
<summary><b>Cursor, Zed, and other clients</b></summary>

Add this to the client's MCP configuration file:

```jsonc
{
  "mcpServers": {
    "gerrit": {
      "command": "npx",
      "args": ["-y", "@gyeonghokim/gerrit-mcp-server"],
      "env": {
        "GERRIT_URL": "https://gerrit.example.com",
        "GERRIT_USER": "your-username",
        "GERRIT_TOKEN": "your-token"
      }
    }
  }
}
```

</details>

**Ask for something.** "What changes am I reviewing?" or "Summarise the diff on change 12345."

## Credentials

Gerrit authenticates REST clients with HTTP Basic using a token from your account settings, and
expects authenticated requests to be prefixed with `/a/`. Both binaries handle the prefix for you.

Generate a token under *Settings → HTTP Credentials*. On Gerrit 3.13 and newer you can name the
token and give it a lifetime (`90 days`, `1 year`, and so on) — worth doing, so the credential this
holds is scoped and expires on its own. Older Gerrit versions call the same thing an *HTTP
password*; it still works, as that endpoint is now an alias that creates a token with the id
`legacy`.

**Where the token lives depends on which frontend you use.**

`gerrit-mcp-server` reads the environment and only the environment, so its credentials live in your
MCP client's config file and nowhere else. It never reads a file of its own.

`gerrit-cli` has no client config to inherit from, so `gerrit-cli init` writes one under the OS
configuration directory:

| OS | Path |
| --- | --- |
| Linux | `$XDG_CONFIG_HOME/gerrit-cli/config.json`, or `~/.config/gerrit-cli/config.json` |
| macOS | `~/Library/Application Support/gerrit-cli/config.json` |
| Windows | `%AppData%\gerrit-cli\config.json` |

Set `GERRIT_CONFIG` to put it somewhere else. Environment variables always take precedence over the
file, so a one-off `GERRIT_TOKEN=... gerrit-cli ...` works and CI never needs a file at all.
`gerrit-cli config` prints where each value actually came from.

What holds for both:

- **The token only ever travels in an `Authorization` header.** It is never passed as a process
  argument, so it cannot be read out of `ps`, and it is never written to a log line or an error
  message. `gerrit-cli init` has no `--token` flag for exactly this reason.
- **Nothing goes anywhere but your Gerrit host.**

What is worth knowing about the file:

- On Linux and macOS it is written `0600`, readable only by you. On Windows it inherits the ACL of
  `%AppData%`, which is already restricted to your account plus SYSTEM and Administrators — setting
  a tighter one needs a dependency this project does not take. If your `%AppData%` is redirected to
  a network share, prefer keeping `GERRIT_TOKEN` in your environment instead.
- **`gerrit-cli init` echoes the token as you type it.** Hiding terminal input needs a dependency
  this project does not take either. Pipe it in if that matters:
  `printf 'https://gerrit.example.com
alice
%s
' "$TOKEN" | gerrit-cli init -non-interactive`
- **Do not commit it, and do not commit an MCP config either.** A project-level `.mcp.json` holding
  `GERRIT_TOKEN` is a credential in your repository. Keep it in your user-level client config, or
  gitignore it.
- **Use a dedicated token with a lifetime**, so it can be revoked without touching your other
  credentials.
- Your Gerrit permissions still apply. Neither frontend can see or do anything your account cannot.

## Configuration

Both frontends read the same variables. For the MCP server they live in your client's config; for
the CLI they are optional, since `gerrit-cli init` writes the same settings to a file.

| Variable | Required | Default | Description |
| --- | --- | --- | --- |
| `GERRIT_URL` | yes | — | Base URL of the Gerrit host, for example `https://gerrit.example.com` |
| `GERRIT_USER` | yes | — | Your Gerrit username |
| `GERRIT_TOKEN` | yes | — | Auth token from *Settings → HTTP Credentials* |
| `GERRIT_ALLOW_WRITE` | no | `false` | Set to `true` to enable the tools and commands that modify Gerrit |
| `GERRIT_TIMEOUT` | no | `30s` | Per-request timeout |
| `GERRIT_LOG_LEVEL` | no | `info` | `debug`, `info`, `warn`, or `error`. Logs go to stderr |
| `GERRIT_CONFIG` | no | — | `gerrit-cli` only. Path to the configuration file, overriding the default |

## Available tools and commands

**The two frontends expose exactly the same set**, and a test in the repository holds them there.
A CLI command is its MCP tool name with the underscores written as dashes — `query_changes` becomes
`query-changes` — and `gerrit-cli` accepts either spelling.

Reads are always available. **Writes are off unless you set `GERRIT_ALLOW_WRITE=true`**, so an
agent cannot abandon a change or post a review by accident. The MCP server does not register the
write tools at all; `gerrit-cli` still lists them in its help, marked, but refuses to run one.

### Read

| Tool | Description |
| --- | --- |
| `query_changes` | Search changes with Gerrit query syntax (`status:open owner:self`) |
| `get_change_details` | Full summary of one change |
| `get_commit_message` | Commit message of the current patch set |
| `list_change_files` | Files touched by the latest patch set |
| `get_file_diff` | Diff for one file in a change |
| `list_change_comments` | Published comments on a change |
| `list_draft_comments` | Your unpublished draft comments |
| `changes_submitted_together` | Changes that would submit alongside this one |
| `suggest_reviewers` | Reviewer suggestions for a change |
| `get_bugs_from_cl` | Bug ids referenced in the commit message |

Every value is a flag; `gerrit-cli` has no positional arguments. Run `gerrit-cli help <command>`
for one command's flags — that is authoritative and cannot go stale.

There is deliberately **no `--json` output**. Everything passes through the same renderer that keeps
responses inside a sensible token budget, and handing an agent raw Gerrit JSON would undo that.

### Write — requires `GERRIT_ALLOW_WRITE=true`

| Tool | Description |
| --- | --- |
| `post_review_comment` | Add a draft comment on a line, or reply in a thread |
| `publish_drafts` | Publish your draft comments as a review |
| `delete_draft_comment` | Delete one draft comment |
| `delete_draft_comments` | Delete every draft on a change |
| `add_reviewer` | Add a reviewer or CC |
| `set_topic` | Set or clear the topic |
| `set_ready_for_review` | Take a change out of WIP |
| `set_work_in_progress` | Mark a change WIP |
| `create_change` | Create a change |
| `abandon_change` | Abandon a change |
| `revert_change` | Revert a change |
| `revert_submission` | Revert a whole submission |

## Exit codes

`gerrit-cli` reports what to do about a failure, not just that one happened. Rendered output goes to
stdout and everything else to stderr, so the answer is safe to pipe.

| Code | Meaning |
| --- | --- |
| 0 | Success |
| 1 | Something else failed; read stderr |
| 2 | Bad arguments |
| 3 | Not configured — run `gerrit-cli init` |
| 4 | Not permitted — the account, or `GERRIT_ALLOW_WRITE` |
| 5 | No such change, file or comment |
| 6 | The change is not in a state that allows this |

## Supported Gerrit versions

Built against the Gerrit **3.14** REST API and tested against it. Expected to work with **3.12 and
newer**; older versions are missing some of the draft comment endpoints.

## Other ways to install

`npm` is the easy path, but the binaries stand alone.

```bash
# Go toolchain
go install github.com/GyeongHoKim/gerrit-mcp-server/cmd/gerrit-mcp-server@latest
go install github.com/GyeongHoKim/gerrit-mcp-server/cmd/gerrit-cli@latest
```

Or download the archive for your platform from the
[releases page](https://github.com/GyeongHoKim/gerrit-mcp-server/releases). It contains both
binaries and the agent skill, and you can point your MCP client's `command` straight at
`gerrit-mcp-server`. No Node required.

## Development

```bash
mise install     # toolchain, pinned in mise.toml
just setup       # dependencies and git hooks
just ci          # everything CI runs
just --list      # all tasks
```

If you are AI Coding Agent(Codex, Claude Code, OpenCode, etc.), See [AGENTS.md](AGENTS.md) for architecture, conventions, and the Gerrit API details worth knowing
before you touch the client.

## License

[Elastic License 2.0](LICENSE).

> It is prohibited to deploy this MCP server as a commercial service.

**Using this at work is fine.** ELv2 places exactly three restrictions on you: you may not offer
this software to third parties as a hosted or managed service, you may not circumvent license key
functionality, and you may not strip the copyright notices. Running it, modifying it, forking it,
and deploying it across your engineering organisation are all expressly permitted.

Note that ELv2 is source-available rather than OSI-approved open source. If your organisation
screens dependencies by license, it may need to be allowlisted.
