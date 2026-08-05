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
  BINARIES,
  NAME,
  PLATFORMS,
  WRAPPERS,
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

function findBinary(artifacts, platform, id) {
  const match = artifacts.find(
    (artifact) =>
      artifact.goos === platform.goos &&
      artifact.goarch === platform.goarch &&
      // The build id is not optional. Two builds produce two binaries for
      // every target, and matching on goos/goarch alone would take whichever
      // goreleaser happened to list first -- silently shipping the server as
      // the CLI, or the reverse, with nothing failing until someone ran it.
      artifact.extra?.ID === id,
  );

  if (match === undefined) {
    throw new Error(
      `no ${id} binary built for ${platform.goos}/${platform.goarch}. ` +
        `Check the build matrix in .goreleaser.yaml.`,
    );
  }

  return path.isAbsolute(match.path) ? match.path : path.join(ROOT, match.path);
}

async function buildPlatformPackage(platform, version, artifacts) {
  const name = packageName(platform);
  const dir = path.join(OUT, `${NAME}-${platform.npm}`);

  await mkdir(path.join(dir, "bin"), { recursive: true });

  // One package, every frontend. files: ["bin/"] already ships the directory,
  // so a second binary needs nothing beyond being copied into it.
  for (const id of BINARIES) {
    const target = path.join(dir, "bin", binaryName(platform, id));
    await copyFile(findBinary(artifacts, platform, id), target);

    if (platform.os !== "win32") {
      await chmod(target, 0o755);
    }
  }

  await writeFile(
    path.join(dir, "package.json"),
    `${JSON.stringify(
      {
        name,
        version,
        description: `The ${platform.npm} binaries for ${WRAPPERS.map((w) => w.name).join(" and ")}.`,
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

async function buildWrapperPackage(wrapper, version, platformNames) {
  const source = path.join(ROOT, "npm", wrapper.dir);
  const dir = path.join(OUT, wrapper.dir);

  await mkdir(path.join(dir, "bin"), { recursive: true });

  const template = JSON.parse(await readFile(path.join(source, "package.json"), "utf8"));

  template.version = version;
  // Exact pins: a wrapper must never pair with a platform binary from a
  // different release. Both wrappers pin the same five packages, because both
  // binaries live in them.
  template.optionalDependencies = Object.fromEntries(
    platformNames.map((name) => [name, version]),
  );

  await writeFile(path.join(dir, "package.json"), `${JSON.stringify(template, null, 2)}\n`);

  const cli = path.join(dir, "bin", "cli.js");
  await copyFile(path.join(source, "bin", "cli.js"), cli);
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

  for (const wrapper of WRAPPERS) {
    await buildWrapperPackage(wrapper, version, names);
  }

  const total = names.length + WRAPPERS.length;

  process.stderr.write(
    `built ${total} npm packages at version ${version} in ${path.relative(ROOT, OUT)}\n`,
  );
}

await main();
