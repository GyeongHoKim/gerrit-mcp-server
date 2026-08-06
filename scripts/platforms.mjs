// The single source of truth for which platforms we ship, and which binaries
// go in them.
//
// Adding a platform here is not enough on its own: the goos/goarch pair must
// also be produced by .goreleaser.yaml, the npm name must be listed in the
// PACKAGES table inside *every* npm/*/bin/cli.js, and the new package needs its
// own trusted publisher on npmjs.com before the release workflow can push it.

import path from "node:path";

export const SCOPE = "@gyeonghokim";
export const NAME = "gerrit-mcp-server";
export const CLI_NAME = "gerrit-cli";

/**
 * The binaries inside every platform package, by goreleaser build id.
 *
 * Two frontends ship as two executables in one platform package rather than as
 * two sets of platform packages: the second frontend then costs one wrapper
 * instead of five more packages, five more trusted publishers on npmjs.com and
 * five more things to keep in step.
 */
export const BINARIES = [NAME, CLI_NAME];

/** @type {{npm: string, goos: string, goarch: string, os: string, cpu: string}[]} */
export const PLATFORMS = [
  { npm: "linux-x64", goos: "linux", goarch: "amd64", os: "linux", cpu: "x64" },
  { npm: "linux-arm64", goos: "linux", goarch: "arm64", os: "linux", cpu: "arm64" },
  { npm: "darwin-x64", goos: "darwin", goarch: "amd64", os: "darwin", cpu: "x64" },
  { npm: "darwin-arm64", goos: "darwin", goarch: "arm64", os: "darwin", cpu: "arm64" },
  { npm: "win32-x64", goos: "windows", goarch: "amd64", os: "win32", cpu: "x64" },
];

/** The wrapper package for the MCP server. */
export const WRAPPER = `${SCOPE}/${NAME}`;

/**
 * The wrapper packages users install, one per frontend.
 *
 * Both resolve to the same five platform packages. `dir` is the source tree
 * under npm/ and the output directory under dist/npm/; `binary` is which of
 * BINARIES that wrapper runs.
 */
export const WRAPPERS = [
  { name: `${SCOPE}/${NAME}`, dir: NAME, binary: NAME },
  { name: `${SCOPE}/${CLI_NAME}`, dir: CLI_NAME, binary: CLI_NAME },
];

/**
 * The package name for a platform entry.
 *
 * Platform packages are named after the server even though they carry both
 * binaries. Renaming them would orphan every installed wrapper that pins them.
 */
export function packageName(platform) {
  return `${SCOPE}/${NAME}-${platform.npm}`;
}

/** The filename of one binary inside a platform package. */
export function binaryName(platform, binary) {
  return platform.os === "win32" ? `${binary}.exe` : binary;
}

/** Strip a leading "v" so git tags and npm versions agree. */
export function normalizeVersion(raw) {
  if (typeof raw !== "string" || raw.length === 0) {
    throw new Error("a version argument is required, for example 1.2.3");
  }

  return raw.startsWith("v") ? raw.slice(1) : raw;
}

/**
 * Resolve the version to publish.
 *
 * Defaults to the version goreleaser recorded in dist/metadata.json, which is
 * the same value it compiled into the binaries. That keeps the npm package
 * version and the version the binary reports from ever drifting apart -- a
 * mismatch users would only discover after installing.
 *
 * @param {string|undefined} raw explicit version, usually the git tag
 * @param {string} root repository root
 * @returns {Promise<string>} version without a leading "v"
 */
export async function resolveVersion(raw, root) {
  if (typeof raw === "string" && raw.length > 0) {
    return normalizeVersion(raw);
  }

  const { readFile } = await import("node:fs/promises");
  const metadata = path.join(root, "dist", "metadata.json");

  let recorded;
  try {
    recorded = JSON.parse(await readFile(metadata, "utf8")).version;
  } catch {
    throw new Error(
      `no version given and ${metadata} is not readable. ` +
        `Run \`just build-all\` first, or pass a version explicitly.`,
    );
  }

  return normalizeVersion(recorded);
}
