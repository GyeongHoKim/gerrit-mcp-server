#!/usr/bin/env node
// Packs every assembled npm package into a tarball for inspection.
//
//   node scripts/pack-npm-packages.mjs 1.2.3
//
// Run after build-npm-packages.mjs. Tarballs land in dist/npm-tarballs/.

import { mkdir, readdir, rm } from "node:fs/promises";
import path from "node:path";
import process from "node:process";

import { resolveVersion } from "./platforms.mjs";
import { npm } from "./npm-cli.mjs";

const ROOT = path.resolve(import.meta.dirname, "..");
const PACKAGES = path.join(ROOT, "dist", "npm");
const TARBALLS = path.join(ROOT, "dist", "npm-tarballs");

async function main() {
  const version = await resolveVersion(process.argv[2], ROOT);

  let entries;
  try {
    entries = await readdir(PACKAGES, { withFileTypes: true });
  } catch {
    throw new Error(`${PACKAGES} not found. Run \`just npm-build ${version}\` first.`);
  }

  await rm(TARBALLS, { recursive: true, force: true });
  await mkdir(TARBALLS, { recursive: true });

  const directories = entries.filter((entry) => entry.isDirectory());

  for (const entry of directories) {
    npm(["pack", "--pack-destination", TARBALLS], path.join(PACKAGES, entry.name));
  }

  process.stderr.write(
    `packed ${directories.length} tarballs into ${path.relative(ROOT, TARBALLS)}\n`,
  );
}

await main();
