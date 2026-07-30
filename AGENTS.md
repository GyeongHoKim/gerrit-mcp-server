# AGENTS.md

Working notes for agents and humans changing this repository. For what the product *is*, read
[README.md](README.md) — this file is about how it is built.

## What this is

A Go binary that speaks Model Context Protocol over stdio and talks to a Gerrit host over its REST
API. Distributed on npm so that `npx -y @gyeonghokim/gerrit-mcp-server` just works.

**The process speaks JSON-RPC on stdout.** Never print to stdout. Not a debug line, not a progress
message, not a stray `fmt.Println`. Anything that is not protocol traffic corrupts the stream and
the client disconnects. Diagnostics go to stderr.

## Setup

```bash
mise trust && mise install   # pinned toolchain -- see mise.toml
just setup                   # npm dependencies and git hooks
```

If you are on Windows and `CLAUDE.md` shows up as a text file containing the word `AGENTS.md`, your
checkout did not restore symlinks: `git config core.symlinks true` and re-checkout.

## Commands

Always go through `just`. The recipes carry the right flags and CI runs the same ones, so a green
`just ci` locally means a green pipeline.

| Command | What it does |
| --- | --- |
| `just build` | Build into `bin/` |
| `just run` | Run from source |
| `just test` / `just test-race` | Tests, with and without the race detector |
| `just lint` / `just lint-fix` | Linters |
| `just fmt` / `just fmt-check` | Formatters |
| `just config-verify` | Validate `.golangci.yml` against the v2 schema |
| `just tidy-check` | Fail if `go.mod`/`go.sum` are untidy |
| `just vuln` | govulncheck |
| `just inspect` | Drive the server through the MCP Inspector |
| `just ci` | Everything CI runs |
| `just fetch-gerrit-docs` | Refetch the Gerrit REST reference into `doc/` |
| `just npm-smoke 1.2.3` | Build, pack and install the npm packages, then run the binary |

## Layout

```
cmd/gerrit-mcp-server/   entry point: flags, environment, wiring
internal/config/         environment parsing and validation
internal/gerrit/         the REST client -- HTTP lives here and nowhere else
internal/mcpserver/      tool definitions and registration
internal/version/        ldflags-injected build stamps
npm/                     published wrapper package (bin/cli.js dispatches by platform)
scripts/                 release plumbing, all Node so it runs on every OS
doc/                     fetched Gerrit REST reference (gitignored)
```

`internal/gerrit` must not import `internal/mcpserver`. HTTP concerns stay on one side of that line
and protocol concerns on the other.

## Conventions

**Dependencies are capped.** The only external module is the MCP Go SDK. `depguard` enforces this —
if you find yourself reaching for a helper library, write the twenty lines instead. A small
dependency graph is a feature of a binary that companies install internally.

**Wrap every error with context**: `fmt.Errorf("fetching change %s: %w", id, err)`. Sentinel errors
belong in `internal/gerrit/errors.go` so callers can use `errors.Is`.

**Never return raw Gerrit JSON to the model.** A `ChangeInfo` is enormous and most of it is noise.
Everything the model sees goes through `internal/mcpserver/render.go`, which compacts it. Tokens
are a budget.

**Write tools are opt-in.** Anything that mutates Gerrit is registered only when
`GERRIT_ALLOW_WRITE` is set. Adding a mutating tool to the read-only set is a bug.

## Gerrit API notes

These are the things that will waste your afternoon if you do not know them. The authoritative
reference is the AsciiDoc in `doc/` — run `just fetch-gerrit-docs` to get it.

- **Authenticated requests need an `/a/` prefix.** `/changes/` is anonymous; `/a/changes/` is
  authenticated. Auth itself is HTTP Basic with an account token.
- **Every JSON response starts with `)]}'`.** Gerrit prepends this line to defeat XSSI. Strip those
  four bytes before parsing or `json.Unmarshal` fails on every single call.
- **Change ids need escaping.** The triplet form is `project~branch~Ihash`, and `project` routinely
  contains `/`. URL-encode it.
- **Not every endpoint exists on every version.** We target 3.14 and support 3.12+. Draft comment
  endpoints are the ones that move.

## Commits

Conventional Commits, enforced by commitlint in the `commit-msg` hook and again in CI — bypassing
the hook locally only defers the failure.

```
feat(tools): add list_draft_comments
fix(gerrit): strip xssi prefix before decoding
```

A **scope is required**, and is free-form and lower-case — say which part of the system moved.
Scopes already in use: `gerrit`, `mcp`, `tools`, `config`, `auth`, `npm`, `release`, `ci`, `deps`,
`lint`, `docs`. Reach for one of those before inventing a synonym.

Allowed **types** are a fixed list in `commitlint.config.mjs`; add to that enum rather than working
around it. Subject is lower-case, no trailing period, 72 characters for the whole header.

Do not use `--no-verify`. If a hook is wrong, fix the hook.

## Testing

- `internal/gerrit`: `httptest.Server` returning recorded Gerrit payloads. Include the `)]}'` prefix
  in fixtures — it is part of what the client must handle.
- `internal/mcpserver`: golden files for rendered output, so token-bloat regressions are visible in
  the diff.
- Tests run with `-race` in CI on Linux, macOS and Windows.

## Releasing

Tag-driven and automated; do not publish by hand.

1. `git tag v1.2.3 && git push origin v1.2.3`
2. `.github/workflows/release.yml` runs goreleaser, assembles the npm packages, and publishes them
   with npm trusted publishing (OIDC).

The tag is the only version source — nothing in git carries a real version number. Never set
`NPM_TOKEN` in the release workflow: if npm sees one it silently uses the legacy token path instead
of OIDC.

Adding a platform means touching `scripts/platforms.mjs`, `.goreleaser.yaml`, `npm/bin/cli.js`, and
registering a trusted publisher for the new package on npmjs.com.
