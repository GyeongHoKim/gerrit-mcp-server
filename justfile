# Development tasks for gerrit-mcp-server.
#
# Every task runs on Linux, macOS and Windows. Recipes that cannot be written
# once are split with the [unix] / [windows] attributes rather than branching
# inside the recipe body.
#
# Tool versions come from mise.toml, so `mise install` is the only prerequisite.

set windows-shell := ["powershell.exe", "-NoLogo", "-NoProfile", "-Command"]
set shell := ["bash", "-euco", "pipefail"]

BIN := "gerrit-mcp-server"
EXT := if os() == "windows" { ".exe" } else { "" }
OUT := "bin" / BIN + EXT
PKG := "./cmd/gerrit-mcp-server"

# The second frontend. Same packages underneath, a command line instead of a
# protocol, and both ship inside the same npm platform packages.
CLI := "gerrit-cli"
CLI_OUT := "bin" / CLI + EXT
CLI_PKG := "./cmd/gerrit-cli"

# Release builds get their stamps from goreleaser; local builds report "dev"
# unless these are exported, which keeps the recipe free of shell-specific
# git plumbing.
VERSION := env("VERSION", "dev")
COMMIT := env("COMMIT", "none")
DATE := env("DATE", "unknown")

MOD := "github.com/GyeongHoKim/gerrit-mcp-server"
LDFLAGS := "-s -w" + \
    " -X " + MOD + "/internal/version.Version=" + VERSION + \
    " -X " + MOD + "/internal/version.Commit=" + COMMIT + \
    " -X " + MOD + "/internal/version.Date=" + DATE

# Show the available tasks.
default:
    @just --list

# ---------------------------------------------------------------- setup

# Install the toolchain, JS dev dependencies and git hooks.
setup: && install-hooks
    mise install
    npm ci

# Install the lefthook git hooks into .git/hooks.
install-hooks:
    lefthook install

# ---------------------------------------------------------------- build

# Build both binaries into bin/.
build:
    go build -ldflags "{{ LDFLAGS }}" -o "{{ OUT }}" "{{ PKG }}"
    go build -ldflags "{{ LDFLAGS }}" -o "{{ CLI_OUT }}" "{{ CLI_PKG }}"

# Run the server straight from source. Speaks JSON-RPC on stdin/stdout.
run *ARGS:
    go run "{{ PKG }}" {{ ARGS }}

# Run the command-line client straight from source.
run-cli *ARGS:
    go run "{{ CLI_PKG }}" {{ ARGS }}

# Cross-compile every release target without publishing anything.
build-all:
    goreleaser build --snapshot --clean

# ---------------------------------------------------------------- quality

# Apply the configured formatters in place.
fmt:
    golangci-lint fmt

# Fail if anything is not formatted.
fmt-check:
    golangci-lint fmt --diff

# Run the linters.
lint:
    golangci-lint run

# Run the linters, fixing what can be fixed automatically.
lint-fix:
    golangci-lint run --fix

# Validate .golangci.yml against the v2 schema. Catches renamed linters.
config-verify:
    golangci-lint config verify

# Tidy the module graph.
tidy:
    go mod tidy

# Fail if go.mod or go.sum are not tidy.
tidy-check: tidy
    git diff --exit-code go.mod go.sum

# Report known vulnerabilities in the dependency graph.
vuln:
    go run golang.org/x/vuln/cmd/govulncheck@latest ./...

# ---------------------------------------------------------------- test

# Run the unit tests.
test:
    go test ./...

# Run the unit tests with the race detector.
test-race:
    go test -race ./...

# Run the tests and write a coverage profile. Keep the value attached on Unix,
# where this is the idiomatic Go flag spelling.
[unix]
test-cover:
    go test -coverprofile=coverage.out ./...

# PowerShell/native Windows argument handling in this toolchain can split the
# suffix of `-coverprofile=coverage.out` into a separate `.out` package
# argument. Pass the flag value as its own argument instead.
[windows]
test-cover:
    go test -coverprofile coverage.out ./...

# Open the coverage profile in a browser.
[unix]
cover-html: test-cover
    go tool cover -html=coverage.out

[windows]
cover-html: test-cover
    go tool cover -html coverage.out

# ---------------------------------------------------------------- mcp

# Build, then drive the server through the MCP Inspector.
inspect: build
    npx @modelcontextprotocol/inspector "{{ OUT }}"

# ---------------------------------------------------------------- release

# Build a full release locally without tagging or publishing.
release-snapshot:
    goreleaser release --snapshot --clean

# Assemble the npm wrapper and per-platform packages. Defaults to the version
# goreleaser stamped into the binaries, so the two can never disagree.
npm-build VERSION="": build-all
    node scripts/build-npm-packages.mjs "{{ VERSION }}"
    node scripts/check-npm-packages.mjs "{{ VERSION }}"

# Pack every npm package into a tarball for inspection.
npm-pack VERSION="": (npm-build VERSION)
    node scripts/pack-npm-packages.mjs "{{ VERSION }}"

# Install the packed tarballs into a scratch project and run the binary.
npm-smoke VERSION="": (npm-pack VERSION)
    node scripts/smoke-npm-packages.mjs "{{ VERSION }}"

# ---------------------------------------------------------------- docs

# Refetch the Gerrit REST API reference into doc/ (gitignored).
fetch-gerrit-docs:
    node scripts/fetch-gerrit-docs.mjs

# ---------------------------------------------------------------- housekeeping

# Remove build output. (unix)
[unix]
clean:
    rm -rf bin dist coverage.out coverage.html

# Remove build output. (windows)
#
# -ErrorAction SilentlyContinue hides the error but still leaves $? false, and
# powershell.exe -Command turns that into exit 1 -- so a clean checkout, where
# none of these exist yet, fails the recipe. Only delete what is actually there.
[windows]
clean:
    foreach ($p in "bin", "dist", "coverage.out", "coverage.html") { if (Test-Path $p) { Remove-Item -Recurse -Force $p } }

# ---------------------------------------------------------------- aggregate

# Everything CI runs. Keep this identical to the CI job so they cannot drift.
ci: config-verify fmt-check lint tidy-check test-race vuln
