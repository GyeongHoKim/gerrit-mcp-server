#!/usr/bin/env node
// Locates the binary for the current platform and hands control to it.
//
// This process sits between an MCP client and a JSON-RPC server, so stdout is
// reserved for the protocol. Everything this wrapper says goes to stderr.

import { spawnSync } from "node:child_process";
import { createRequire } from "node:module";

const require = createRequire(import.meta.url);

// Kept in sync with scripts/platforms.mjs. It is duplicated rather than
// imported because this file ships on its own inside the published package.
//
// The same five packages back @gyeonghokim/gerrit-cli: one platform package
// carries both binaries, so a second frontend costs one wrapper rather than
// five more packages.
const PACKAGES = {
  "linux-x64": "@gyeonghokim/gerrit-mcp-server-linux-x64",
  "linux-arm64": "@gyeonghokim/gerrit-mcp-server-linux-arm64",
  "darwin-x64": "@gyeonghokim/gerrit-mcp-server-darwin-x64",
  "darwin-arm64": "@gyeonghokim/gerrit-mcp-server-darwin-arm64",
  "win32-x64": "@gyeonghokim/gerrit-mcp-server-win32-x64",
};

function fail(message) {
  process.stderr.write(`gerrit-mcp-server: ${message}\n`);
  process.exit(1);
}

const target = `${process.platform}-${process.arch}`;
const packageName = PACKAGES[target];

if (packageName === undefined) {
  fail(
    `unsupported platform ${target}. ` +
      `Supported platforms are ${Object.keys(PACKAGES).join(", ")}. ` +
      `Build from source instead: go install github.com/GyeongHoKim/gerrit-mcp-server/cmd/gerrit-mcp-server@latest`,
  );
}

const binary = process.platform === "win32" ? "gerrit-mcp-server.exe" : "gerrit-mcp-server";

let executable;
try {
  executable = require.resolve(`${packageName}/bin/${binary}`);
} catch {
  fail(
    `the ${packageName} package is not installed. ` +
      `It is an optional dependency, so this usually means the install ran with ` +
      `--no-optional, --omit=optional, or with a lockfile from another platform. ` +
      `Reinstall with optional dependencies enabled.`,
  );
}

const result = spawnSync(executable, process.argv.slice(2), { stdio: "inherit" });

if (result.error !== undefined) {
  fail(`could not start ${executable}: ${result.error.message}`);
}

process.exit(result.status ?? 1);
