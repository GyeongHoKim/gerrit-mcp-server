#!/usr/bin/env node
// Assembles the npm packages for a release into dist/npm/.
//
// Binaries come from goreleaser's dist/artifacts.json manifest rather than
// from guessed directory names, so a change in goreleaser's layout surfaces as
// a clear error instead of a silently empty package.
//
//   node scripts/build-npm-packages.mjs 1.2.3
//
// The git tag is the only source of version truth. Nothing here reads or
// writes the version fields tracked in git, so the tree stays clean.

import { chmod, copyFile, mkdir, readFile, rm, writeFile } from "node:fs/promises";
import path from "node:path";
import process from "node:process";

import {
  NAME,
  PLATFORMS,
  WRAPPER,
  binaryName,
  packageName,
  resolveVersion,
} from "./platforms.mjs";

const ROOT = path.resolve(import.meta.dirname, "..");
const DIST = path.join(ROOT, "dist");
const OUT = path.join(DIST, "npm");

const REPOSITORY = {
  type: "git",
  url: "git+https://github.com/GyeongHoKim/gerrit-mcp-server.git",
};

async function readBinaryArtifacts() {
  const manifest = path.join(DIST, "artifacts.json");

  let raw;
  try {
    raw = await readFile(manifest, "utf8");
  } catch {
    throw new Error(
      `${manifest} not found. Run \`just build-all\` or \`just release-snapshot\` first.`,
    );
  }

  return JSON.parse(raw).filter((artifact) => artifact.type === "Binary");
}

function findBinary(artifacts, platform) {
  const match = artifacts.find(
    (artifact) => artifact.goos === platform.goos && artifact.goarch === platform.goarch,
  );

  if (match === undefined) {
    throw new Error(
      `no binary built for ${platform.goos}/${platform.goarch}. ` +
        `Check the build matrix in .goreleaser.yaml.`,
    );
  }

  return path.isAbsolute(match.path) ? match.path : path.join(ROOT, match.path);
}

async function buildPlatformPackage(platform, version, artifacts) {
  const name = packageName(platform);
  const dir = path.join(OUT, `${NAME}-${platform.npm}`);
  const binary = binaryName(platform);

  await mkdir(path.join(dir, "bin"), { recursive: true });

  const target = path.join(dir, "bin", binary);
  await copyFile(findBinary(artifacts, platform), target);

  if (platform.os !== "win32") {
    await chmod(target, 0o755);
  }

  await writeFile(
    path.join(dir, "package.json"),
    `${JSON.stringify(
      {
        name,
        version,
        description: `The ${platform.npm} binary for ${WRAPPER}.`,
        license: "Elastic-2.0",
        repository: REPOSITORY,
        os: [platform.os],
        cpu: [platform.cpu],
        files: ["bin/"],
        // Yarn PnP cannot execute a binary from inside a zip archive.
        preferUnplugged: true,
      },
      null,
      2,
    )}\n`,
  );

  await copyFile(path.join(ROOT, "LICENSE"), path.join(dir, "LICENSE"));

  return name;
}

async function buildWrapperPackage(version, platformNames) {
  const dir = path.join(OUT, NAME);
  await mkdir(path.join(dir, "bin"), { recursive: true });

  const template = JSON.parse(await readFile(path.join(ROOT, "npm", "package.json"), "utf8"));

  template.version = version;
  // Exact pins: the wrapper must never pair with a platform binary from a
  // different release.
  template.optionalDependencies = Object.fromEntries(
    platformNames.map((name) => [name, version]),
  );

  await writeFile(path.join(dir, "package.json"), `${JSON.stringify(template, null, 2)}\n`);

  const cli = path.join(dir, "bin", "cli.js");
  await copyFile(path.join(ROOT, "npm", "bin", "cli.js"), cli);
  await chmod(cli, 0o755);

  await copyFile(path.join(ROOT, "LICENSE"), path.join(dir, "LICENSE"));
  await copyFile(path.join(ROOT, "README.md"), path.join(dir, "README.md"));
}

async function main() {
  const version = await resolveVersion(process.argv[2], ROOT);
  const artifacts = await readBinaryArtifacts();

  await rm(OUT, { recursive: true, force: true });
  await mkdir(OUT, { recursive: true });

  const names = [];
  for (const platform of PLATFORMS) {
    names.push(await buildPlatformPackage(platform, version, artifacts));
  }

  await buildWrapperPackage(version, names);

  process.stderr.write(
    `built ${names.length + 1} npm packages at version ${version} in ${path.relative(ROOT, OUT)}\n`,
  );
}

await main();
