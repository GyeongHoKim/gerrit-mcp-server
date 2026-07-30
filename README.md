# gerrit-mcp-server

[![CI](https://github.com/GyeongHoKim/gerrit-mcp-server/actions/workflows/ci.yml/badge.svg)](https://github.com/GyeongHoKim/gerrit-mcp-server/actions/workflows/ci.yml)
[![npm](https://img.shields.io/npm/v/@gyeonghokim/gerrit-mcp-server)](https://www.npmjs.com/package/@gyeonghokim/gerrit-mcp-server)
[![Go](https://img.shields.io/badge/go-1.26-00ADD8)](https://go.dev)
[![License](https://img.shields.io/badge/license-Elastic--2.0-005571)](LICENSE)

A [Model Context Protocol](https://modelcontextprotocol.io) server that connects your AI coding
agent to Gerrit code review.

Ask your agent to find the changes waiting on you, read a diff, draft line comments, and publish a
review — without leaving the session and without pasting change numbers back and forth.

It speaks MCP over **stdio**, so it works with any MCP client: Claude Code, Codex, Cursor, Zed,
Continue, or your own. It is a single static binary with no runtime dependencies. You self-host it;
nothing is sent anywhere except to the Gerrit host you configure.

## Quick start

You need Node installed only so that `npx` can fetch and run the right binary for your machine. The
server itself is Go and has no Node runtime dependency.

**1. Create a Gerrit auth token.** In Gerrit, go to *Settings → HTTP Credentials* and generate one.
See [Authentication](#authentication) below if your Gerrit is older.

**2. Add the server to your MCP client.**

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

**3. Ask for something.** "What changes am I reviewing?" or "Summarise the diff on change 12345."

## Authentication

Gerrit authenticates REST clients with HTTP Basic using a token from your account settings, and
expects authenticated requests to be prefixed with `/a/`. This server handles the prefix for you.

Generate a token under *Settings → HTTP Credentials*. On Gerrit 3.13 and newer you can name the
token and give it a lifetime (`90 days`, `1 year`, and so on) — worth doing, so the credential this
server holds is scoped and expires on its own.

Older Gerrit versions call the same thing an *HTTP password*. It still works: that endpoint is now
an alias that creates a token with the id `legacy`.

The token is sent in an `Authorization` header. It never appears in a command line, a log line, or
an error message.

## Configuration

All configuration is environment variables, so it lives in your MCP client config and nowhere else.

| Variable | Required | Default | Description |
| --- | --- | --- | --- |
| `GERRIT_URL` | yes | — | Base URL of the Gerrit host, for example `https://gerrit.example.com` |
| `GERRIT_USER` | yes | — | Your Gerrit username |
| `GERRIT_TOKEN` | yes | — | Auth token from *Settings → HTTP Credentials* |
| `GERRIT_ALLOW_WRITE` | no | `false` | Set to `true` to register the tools that modify Gerrit |
| `GERRIT_TIMEOUT` | no | `30s` | Per-request timeout |
| `GERRIT_LOG_LEVEL` | no | `info` | `debug`, `info`, `warn`, or `error`. Logs go to stderr |

## Available tools

Read-only tools are always registered. **Write tools are off unless you set
`GERRIT_ALLOW_WRITE=true`**, so an agent cannot abandon a change or post a review by accident.

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

## Security

- **The token stays in a header.** It is never passed as a process argument, so it cannot be read
  out of `ps`, and it is never written to logs.
- **Read-only by default.** Destructive tools are not registered unless you opt in.
- **Don't commit your MCP config.** A project-level `.mcp.json` with `GERRIT_TOKEN` in it is a
  credential in your repository. Keep it in your user-level client config, or gitignore it.
- **Use a dedicated token** with a lifetime, so it can be revoked without touching your other
  credentials.
- Your Gerrit permissions still apply. This server cannot see or do anything your account cannot.

## Supported Gerrit versions

Built against the Gerrit **3.14** REST API and tested against it. Expected to work with **3.12 and
newer**; older versions are missing some of the draft comment endpoints.

## Other ways to install

`npx` is the easy path, but the binary stands alone.

```bash
# Go toolchain
go install github.com/GyeongHoKim/gerrit-mcp-server/cmd/gerrit-mcp-server@latest
```

Or download a binary for your platform from the
[releases page](https://github.com/GyeongHoKim/gerrit-mcp-server/releases) and point your MCP
client's `command` at it directly. No Node required.

## Development

```bash
mise install     # toolchain, pinned in mise.toml
just setup       # dependencies and git hooks
just ci          # everything CI runs
just --list      # all tasks
```

See [AGENTS.md](AGENTS.md) for architecture, conventions, and the Gerrit API details worth knowing
before you touch the client.

## License

[Elastic License 2.0](LICENSE).

**Using this at work is fine.** ELv2 places exactly three restrictions on you: you may not offer
this software to third parties as a hosted or managed service, you may not circumvent license key
functionality, and you may not strip the copyright notices. Running it, modifying it, forking it,
and deploying it across your engineering organisation are all expressly permitted.

Note that ELv2 is source-available rather than OSI-approved open source. If your organisation
screens dependencies by license, it may need to be allowlisted.
