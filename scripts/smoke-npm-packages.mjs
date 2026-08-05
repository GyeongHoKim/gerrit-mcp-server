#!/usr/bin/env node
// Installs the packed tarballs into a throwaway project and runs the binary.
//
//   node scripts/smoke-npm-packages.mjs 1.2.3
//
// This is the check that catches the failure modes unit tests cannot see: a
// wrapper resolving the wrong package, a binary that was never marked
// executable, an optionalDependencies pin that does not match -- or, since two
// binaries ship in one platform package, both wrappers running the same one.

import { mkdtemp, readdir, rm, writeFile } from "node:fs/promises";
import { spawnSync } from "node:child_process";
import os from "node:os";
import path from "node:path";
import process from "node:process";

import { NAME, PLATFORMS, WRAPPERS, resolveVersion } from "./platforms.mjs";
import { npm } from "./npm-cli.mjs";

const ROOT = path.resolve(import.meta.dirname, "..");
const TARBALLS = path.join(ROOT, "dist", "npm-tarballs");

function currentPlatform() {
  const target = `${process.platform}-${process.arch}`;
  const platform = PLATFORMS.find((candidate) => candidate.npm === target);

  if (platform === undefined) {
    throw new Error(`this host (${target}) is not one of the shipped platforms`);
  }

  return platform;
}

function tarballFor(files, name, version) {
  // npm pack rewrites "@scope/name" as "scope-name-version.tgz". Matched
  // exactly rather than by substring: three of these names are prefixes of
  // each other, and picking the first that merely contains one would be luck
  // rather than a check.
  const wanted = `gyeonghokim-${name}-${version}.tgz`;

  if (!files.includes(wanted)) {
    throw new Error(`${wanted} is not in ${TARBALLS}`);
  }

  return path.join(TARBALLS, wanted);
}

function runVersion(scratch, wrapper, version) {
  const binary = wrapper.binary;

  // The wrapper's own shim, run through node rather than through the .bin
  // entry npm generates. This is the code that can actually be wrong -- the
  // require.resolve that picks a platform package and a binary inside it --
  // and reaching it directly keeps the check identical everywhere, where the
  // .bin entry is a shell script on Unix and a .cmd on Windows that recent
  // Node will not spawn without a shell.
  const shim = path.join(scratch, "node_modules", ...wrapper.name.split("/"), "bin", "cli.js");

  const result = spawnSync(process.execPath, [shim, "--version"], { encoding: "utf8" });

  if (result.error !== undefined) {
    throw new Error(`could not run ${shim}: ${result.error.message}`);
  }

  if (result.status !== 0) {
    throw new Error(
      `${shim} --version exited with ${result.status}
${result.stderr ?? ""}`,
    );
  }

  const output = (result.stdout ?? "").trim();

  if (!output.includes(version)) {
    throw new Error(`expected the version output to mention ${version}, got: ${output}`);
  }

  // Each binary names itself, so this is also what catches a platform package
  // that shipped one binary twice under two filenames.
  if (!output.startsWith(`${binary} `)) {
    throw new Error(`expected ${binary} to report itself, got: ${output}`);
  }

  return output;
}

async function main() {
  const version = await resolveVersion(process.argv[2], ROOT);
  const platform = currentPlatform();

  let files;
  try {
    files = await readdir(TARBALLS);
  } catch {
    throw new Error(`${TARBALLS} not found. Run \`just npm-pack ${version}\` first.`);
  }

  const native = tarballFor(files, `${NAME}-${platform.npm}`, version);
  const wrappers = WRAPPERS.map((wrapper) => tarballFor(files, wrapper.dir, version));

  const scratch = await mkdtemp(path.join(os.tmpdir(), "gerrit-mcp-smoke-"));

  try {
    await writeFile(
      path.join(scratch, "package.json"),
      `${JSON.stringify({ name: "smoke", version: "0.0.0", private: true }, null, 2)}\n`,
    );

    // Install the platform package first so each wrapper's optional dependency
    // resolves from disk rather than from the registry, which has not seen
    // this version yet. One platform package backs both wrappers.
    npm(["install", "--no-audit", "--no-fund", native, ...wrappers], scratch);

    const reported = WRAPPERS.map((wrapper) => runVersion(scratch, wrapper, version));

    process.stderr.write(`smoke test passed on ${platform.npm} at ${version}\n`);

    for (const line of reported) {
      process.stderr.write(`  ${line}\n`);
    }
  } finally {
    await rm(scratch, { recursive: true, force: true });
  }
}

await main();
