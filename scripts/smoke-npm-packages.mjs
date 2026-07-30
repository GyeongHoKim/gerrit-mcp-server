#!/usr/bin/env node
// Installs the packed tarballs into a throwaway project and runs the binary.
//
//   node scripts/smoke-npm-packages.mjs 1.2.3
//
// This is the check that catches the failure mode unit tests cannot see: the
// wrapper resolving the wrong package, a binary that was never marked
// executable, or an optionalDependencies pin that does not match.

import { mkdtemp, readdir, rm, writeFile } from "node:fs/promises";
import { spawnSync } from "node:child_process";
import os from "node:os";
import path from "node:path";
import process from "node:process";

import { NAME, PLATFORMS, WRAPPER, resolveVersion } from "./platforms.mjs";
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

function tarballFor(files, suffix) {
  // npm pack rewrites "@scope/name" as "scope-name-version.tgz".
  const match = files.find((file) => file.includes(suffix) && file.endsWith(".tgz"));

  if (match === undefined) {
    throw new Error(`no tarball matching ${suffix} in ${TARBALLS}`);
  }

  return path.join(TARBALLS, match);
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

  const wrapper = tarballFor(files, `${NAME}-${version}`);
  const native = tarballFor(files, `${NAME}-${platform.npm}-${version}`);

  const scratch = await mkdtemp(path.join(os.tmpdir(), "gerrit-mcp-smoke-"));

  try {
    await writeFile(
      path.join(scratch, "package.json"),
      `${JSON.stringify({ name: "smoke", version: "0.0.0", private: true }, null, 2)}\n`,
    );

    // Install the platform package first so the wrapper's optional dependency
    // resolves from disk rather than from the registry, which has not seen
    // this version yet.
    npm(["install", "--no-audit", "--no-fund", native, wrapper], scratch);

    const binary = path.join(
      scratch,
      "node_modules",
      ".bin",
      process.platform === "win32" ? `${NAME}.cmd` : NAME,
    );

    const result = spawnSync(binary, ["--version"], { encoding: "utf8" });

    if (result.status !== 0) {
      throw new Error(
        `${binary} --version exited with ${result.status}\n${result.stderr ?? ""}`,
      );
    }

    const output = (result.stdout ?? "").trim();

    if (!output.includes(version)) {
      throw new Error(`expected the version output to mention ${version}, got: ${output}`);
    }

    process.stderr.write(`smoke test passed for ${WRAPPER}@${version} on ${platform.npm}\n`);
    process.stderr.write(`  ${output}\n`);
  } finally {
    await rm(scratch, { recursive: true, force: true });
  }
}

await main();
