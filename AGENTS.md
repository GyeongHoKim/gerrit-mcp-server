# AGENTS.md

Working notes for agents and humans changing this repository. For what the product *is*, read
[README.md](README.md) — this file is about how it is built.

## What this is

Two Go binaries over one Gerrit REST client, distributed together on npm.

- **`gerrit-mcp-server`** speaks Model Context Protocol over stdio.
- **`gerrit-cli`** is a command line, driven by an agent skill in `skills/`. It exists because an
  MCP server's tool schemas sit in a model's context for a whole session, and a skill costs one
  line until it triggers.

They expose the same 22 operations from the same packages, and `internal/mcpserver/parity_test.go`
holds them to that.

**Stdout means different things in the two binaries, and getting it wrong is fatal in one of them.**

- `cmd/gerrit-mcp-server` speaks JSON-RPC on stdout. **Never print to stdout there.** Not a debug
  line, not a progress message, not a stray `fmt.Println`. Anything that is not protocol traffic
  corrupts the stream and the client disconnects. Diagnostics go to stderr.
- `cmd/gerrit-cli` prints rendered output on stdout and everything else -- usage, prompts,
  diagnostics, errors -- on stderr, so the answer stays pipeable.

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
| `just build` | Build both binaries into `bin/` |
| `just run` | Run the MCP server from source |
| `just run-cli` | Run gerrit-cli from source |
| `just test` / `just test-race` | Tests, with and without the race detector |
| `just lint` / `just lint-fix` | Linters |
| `just fmt` / `just fmt-check` | Formatters |
| `just config-verify` | Validate `.golangci.yml` against the v2 schema |
| `just tidy-check` | Fail if `go.mod`/`go.sum` are untidy |
| `just vuln` | govulncheck |
| `just inspect` | Drive the server through the MCP Inspector |
| `just ci` | Everything CI runs |
| `just check-skills` | Fail if a skill under `skills/` is not installable |
| `just fetch-gerrit-docs` | Refetch the Gerrit REST reference into `doc/` |
| `just npm-smoke` | Build, pack and install the npm packages, then run both binaries |

Run `just npm-smoke` without a version. Passing one sets the npm package version but not the stamp
goreleaser compiled in, and the smoke test compares the two.

## Layout

```text
cmd/gerrit-mcp-server/   MCP entry point: flags, environment, wiring
cmd/gerrit-cli/          CLI entry point: the same, plus the filesystem and a terminal
internal/config/         environment and file parsing, and validation
internal/gerrit/         the REST client -- HTTP lives here and nowhere else
internal/render/         Gerrit types to the compact text a caller reads
internal/mcpserver/      tool definitions and registration
internal/cli/            command definitions, dispatch, exit codes, init
internal/version/        ldflags-injected build stamps
npm/<wrapper>/           published wrapper packages (bin/cli.js dispatches by platform)
skills/<skill>/          agent skills, installed with `npx skills add`
scripts/                 release plumbing, all Node so it runs on every OS
doc/                     fetched Gerrit REST reference (gitignored)
```

`internal/gerrit` must not import `internal/mcpserver` or `internal/cli`. HTTP concerns stay on one
side of that line and frontend concerns on the other. The two frontends do not import each other
either -- the one exception is `internal/mcpserver/parity_test.go`, which imports `internal/cli` on
purpose so the inventory contract lives next to the list it defends.

## Conventions

**Dependencies are capped.** The only external module is the MCP Go SDK. `depguard` enforces this —
if you find yourself reaching for a helper library, write the twenty lines instead. A small
dependency graph is a feature of a binary that companies install internally.

**Wrap every error with context**: `fmt.Errorf("fetching change %s: %w", id, err)`. Sentinel errors
are declared in the file whose endpoints return them -- transport statuses in `client.go`,
argument validation next to the call it guards -- so callers can use `errors.Is`.

**Never return raw Gerrit JSON to the model.** A `ChangeInfo` is enormous and most of it is noise.
Everything the model sees goes through `internal/render`, which compacts it. Tokens
are a budget.

**Write tools are opt-in, and the two frontends enforce that differently.** `internal/mcpserver`
does not register a write tool at all without `GERRIT_ALLOW_WRITE`, because a model cannot call a
tool it cannot see. `internal/cli` still lists its write commands in `gerrit-cli help`, marked, and
refuses to run one -- a caller there can type any string, so hiding them would only teach a reader
the feature does not exist when the answer is one variable away. Both check before a client is
built, so nothing reaches Gerrit. Adding a mutating operation to either read set is a bug, and
`internal/mcpserver/parity_test.go` is what catches it.

**A new operation lands in both frontends or in neither.** `readTools`/`writeTools` in
`internal/mcpserver/server.go` and `readCommands`/`writeCommands` in `internal/cli/cli.go` are
functions rather than package-level slices for the same reason: a var could be appended to from
anywhere in the package.

**A minimum Gerrit version is declared once and associated twice.** The release itself goes in
`internal/gerrit`, beside the endpoint that needs it (`MinVersionWorkInProgress` in `actions.go`),
because it is a fact about the Gerrit API. Which operation calls that endpoint is a fact about a
frontend, so `minVersions()` in `internal/mcpserver` and `since(...)` in `internal/cli` each state
their own — and `parity_test.go` holds the two equal, the same way it holds the inventory.

The two frontends then act on it differently, mirroring the write split. `internal/mcpserver`
removes the tool after connecting and lets the SDK send `tools/list_changed`, because a model
cannot call a tool it cannot see — one background probe per session, and only on a server that
registered write tools, since every gated tool is a write tool. `internal/cli` lists the command
with the release it needs and lets an invoked command fail, because a caller there can type any
string. Neither gates the call itself: `internal/gerrit` converts the 404, which costs no request
until one has already failed.

## Gerrit API notes

These are the things that will waste your afternoon if you do not know them. The authoritative
reference is the AsciiDoc in `doc/` — run `just fetch-gerrit-docs` to get it.

- **Authenticated requests need an `/a/` prefix.** `/changes/` is anonymous; `/a/changes/` is
  authenticated. Auth itself is HTTP Basic with an account token.
- **Every JSON response starts with `)]}'`.** Gerrit prepends this line to defeat XSSI. Strip those
  four bytes before parsing or `json.Unmarshal` fails on every single call.
- **Change ids come in two forms, and one is pre-encoded.** `ChangeInfo.id` is
  `project~<number>` with `project` *already* URL encoded; `triplet_id` is
  `project~branch~Change-Id`. Project names routinely contain `/`, so an id must reach Gerrit as one
  escaped path segment — but escaping an id Gerrit handed you turns `%2F` into `%252F` and 404s a
  change that plainly exists. `changePath` unescapes before escaping so both forms work.
- **Not every endpoint exists on every version.** We build and test against 3.14 and support
  **2.14+**. Only three operations are actually missing on an older host: `/wip` and `/ready`
  arrived in 2.15, `/revert_submission` in 3.2. Draft comment endpoints are *not* among them —
  they work on 2.14 — and neither is anything else in the inventory. The inventory comes from
  Gerrit's own release notes; the floor itself has not been exercised against a real 2.14 host,
  and what that leaves unverified is under [Testing](#testing).
- **`GET /changes/{id}/message` is 3.x only.** It hands back footers Gerrit parsed for you, which
  is why it is tempting. `GET /changes/{id}/revisions/{rev}/commit` answers the same question on
  every supported release, so `GetCommitMessage` uses that one and `parseFooters` reads the git
  trailers itself. One code path beats a branch on which host answered.
- **`total_comment_count` and `unresolved_comment_count` are absent before 3.0.** They are `*int`
  for that reason: an absent count is not a count of zero, and the renderer omits the line rather
  than inventing a number.
- **Version handling fails open.** A release that cannot be determined — a proxy in the way, an
  unreadable string, an enterprise fork reporting whatever it forked from — offers everything.
  Hiding an operation that would have worked is worse than offering one that fails with an
  explanation, and `unsupportedIfOlder` is built so that fail-open is the fallthrough rather than
  a branch someone can forget.

## Commits

Conventional Commits, enforced by commitlint in the `commit-msg` hook and again in CI — bypassing
the hook locally only defers the failure.

```text
feat(tools): add list_draft_comments
fix(gerrit): strip xssi prefix before decoding
```

A **scope is required**, and is free-form and lower-case — say which part of the system moved.
Scopes already in use: `gerrit`, `mcp`, `cli`, `tools`, `config`, `auth`, `npm`, `release`, `ci`,
`deps`, `lint`, `docs`, `skill`. Reach for one of those before inventing a synonym.

commitlint reads any body line starting `word:` as a footer, and then objects that it has no blank
line before it. Reflow rather than fighting it.

Allowed **types** are a fixed list in `commitlint.config.mjs`; add to that enum rather than working
around it. Subject is lower-case, no trailing period, 72 characters for the whole header.

Do not use `--no-verify`. If a hook is wrong, fix the hook.

## Testing

- `internal/gerrit`: `httptest.Server` returning recorded Gerrit payloads. Include the `)]}'` prefix
  in fixtures — it is part of what the client must handle.
- `internal/render`: golden files written by hand, not captured with `-update`. A golden taken from
  the implementation records only what the code happens to do.
- `internal/mcpserver`: tools driven over an in-memory transport against a stub Gerrit.
- `internal/cli`: commands driven end to end -- flags, client, renderer, stdout -- against the same
  kind of stub. No new golden files; `internal/render` already goldens every function, and the CLI
  tests assert which renderer was called rather than what it emits.
- The filesystem and the terminal are injected through `cli.Options`, never reached for. That is
  what makes `TestInitNeverReadsStdinWithoutATerminal` possible, and that test is the reason `init`
  is shaped the way it is.
- Tests run with `-race` in CI on Linux, macOS and Windows.
- **Version behaviour is covered by stubs that report an old release.** A handler answering
  `"2.14.22"` is indistinguishable from a real 2.14 for everything this client does, because the
  only things the code reads are that string and the 404s. The floor itself cannot be run here —
  official Docker images start around 2.16 — so a change to the version logic is verified by adding
  a case to `ParseServerVersion`'s table and to the 404-conversion tests, not by standing up a
  container. That table is an inventory of strings seen in the wild; add to it rather than
  inventing cases.
- One assumption rests on nothing we can execute: that a 2.14 host answers **404**, not 405 or 400,
  for an endpoint it never had. Fail-open contains the damage either way — a non-404 stays whatever
  Gerrit said — but the "too old" diagnosis simply would not fire. Confirm it against a real 2.14
  host before claiming the floor in a release.

## Releasing

Tag-driven and automated; do not publish by hand.

1. `git tag v1.2.3 && git push origin v1.2.3`
2. `.github/workflows/release.yml` runs goreleaser, assembles the npm packages, and publishes them
   with npm trusted publishing (OIDC).

The tag is the only version source — nothing in git carries a real version number. Never set
`NPM_TOKEN` in the release workflow: if npm sees one it silently uses the legacy token path instead
of OIDC.

**Seven packages go out**: five platform packages, each carrying both binaries, plus one wrapper
per frontend. Every one of them needs its own trusted publisher on npmjs.com naming the workflow
*filename* `release.yml`. There is no token fallback, so a package that is not configured fails the
publish outright -- after goreleaser has already cut the GitHub release and the earlier packages are
already up, which is a release nobody can install and nobody can redo. Configure the publisher
before tagging. `scripts/check-npm-packages.mjs` catches what it can before the first publish, but
it cannot see whether a trusted publisher exists.

Adding a platform means touching `scripts/platforms.mjs`, `.goreleaser.yaml`, the `PACKAGES` table
in **every** `npm/*/bin/cli.js`, and registering a trusted publisher for the new package.

Adding a frontend means a second `builds:` entry in `.goreleaser.yaml`, an entry in `BINARIES` and
`WRAPPERS`, a new `npm/<name>/` source tree, a publish step in `release.yml`, and one more trusted
publisher. `findBinary` in `scripts/build-npm-packages.mjs` matches artifacts on the goreleaser
build id precisely because more than one build now produces a binary per target.
