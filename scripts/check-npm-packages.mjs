#!/usr/bin/env node
// Checks the assembled npm packages before any of them is published.
//
//   node scripts/check-npm-packages.mjs 1.2.3
//
// Publishing is not a transaction. Seven packages go up one at a time, npm
// will not let a version be republished, and a set that turns out to be
// incomplete or inconsistent halfway through leaves a release nobody can
// install and nobody can redo. This is the last cheap moment to notice.
//
// What it cannot check is whether each package has a trusted publisher
// configured on npmjs.com. There is no CLI for that and no token fallback to
// fall back to, so it stays a human step -- see the release checklist in
// AGENTS.md.

import { readFile } from "node:fs/promises";
import path from "node:path";
import process from "node:process";

import { NAME, PLATFORMS, WRAPPERS, packageName, resolveVersion } from "./platforms.mjs";

const ROOT = path.resolve(import.meta.dirname, "..");
const OUT = path.join(ROOT, "dist", "npm");

async function readManifest(directory) {
  const manifest = path.join(OUT, directory, "package.json");

  try {
    return JSON.parse(await readFile(manifest, "utf8"));
  } catch {
    throw new Error(`${manifest} is missing. Run \`just npm-build\` first.`);
  }
}

function check(problems, condition, message) {
  if (!condition) {
    problems.push(message);
  }
}

async function checkPlatformPackages(problems, version) {
  for (const platform of PLATFORMS) {
    const directory = `${NAME}-${platform.npm}`;
    const manifest = await readManifest(directory);

    check(
      problems,
      manifest.name === packageName(platform),
      `${directory} is named ${manifest.name}, want ${packageName(platform)}`,
    );

    check(
      problems,
      manifest.version === version,
      `${directory} is version ${manifest.version}, want ${version}`,
    );
  }
}

async function checkWrapperPackages(problems, version) {
  for (const wrapper of WRAPPERS) {
    const manifest = await readManifest(wrapper.dir);

    check(
      problems,
      manifest.name === wrapper.name,
      `${wrapper.dir} is named ${manifest.name}, want ${wrapper.name}`,
    );

    check(
      problems,
      manifest.version === version,
      `${wrapper.dir} is version ${manifest.version}, want ${version}`,
    );

    // The wrapper runs one binary and one only. A bin entry naming the other
    // frontend would install a command that runs the wrong program.
    check(
      problems,
      Object.keys(manifest.bin ?? {}).join(",") === wrapper.binary,
      `${wrapper.dir} exposes ${Object.keys(manifest.bin ?? {}).join(",")}, want ${wrapper.binary}`,
    );

    // Both wrappers pin all five platform packages, at this exact version:
    // one platform package carries both binaries, so neither wrapper has a
    // smaller set to depend on.
    const pinned = manifest.optionalDependencies ?? {};

    for (const platform of PLATFORMS) {
      const name = packageName(platform);

      check(
        problems,
        pinned[name] === version,
        `${wrapper.dir} pins ${name} at ${pinned[name] ?? "nothing"}, want ${version}`,
      );
    }
  }
}

async function main() {
  const version = await resolveVersion(process.argv[2], ROOT);
  const problems = [];

  await checkPlatformPackages(problems, version);
  await checkWrapperPackages(problems, version);

  if (problems.length > 0) {
    throw new Error(`the assembled packages are not publishable:\n  ${problems.join("\n  ")}`);
  }

  const total = PLATFORMS.length + WRAPPERS.length;

  process.stderr.write(`${total} npm packages check out at version ${version}\n`);
}

await main();
