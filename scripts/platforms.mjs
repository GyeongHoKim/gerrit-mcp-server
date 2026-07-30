// The single source of truth for which platforms we ship.
//
// Adding a platform here is not enough on its own: the goos/goarch pair must
// also be produced by .goreleaser.yaml, the npm name must be listed in
// npm/bin/cli.js, and the new package needs its own trusted publisher on
// npmjs.com before the release workflow can push it.

import path from "node:path";

export const SCOPE = "@gyeonghokim";
export const NAME = "gerrit-mcp-server";

/** @type {{npm: string, goos: string, goarch: string, os: string, cpu: string}[]} */
export const PLATFORMS = [
  { npm: "linux-x64", goos: "linux", goarch: "amd64", os: "linux", cpu: "x64" },
  { npm: "linux-arm64", goos: "linux", goarch: "arm64", os: "linux", cpu: "arm64" },
  { npm: "darwin-x64", goos: "darwin", goarch: "amd64", os: "darwin", cpu: "x64" },
  { npm: "darwin-arm64", goos: "darwin", goarch: "arm64", os: "darwin", cpu: "arm64" },
  { npm: "win32-x64", goos: "windows", goarch: "amd64", os: "win32", cpu: "x64" },
];

/** The wrapper package users actually install. */
export const WRAPPER = `${SCOPE}/${NAME}`;

/** The package name for a platform entry. */
export function packageName(platform) {
  return `${SCOPE}/${NAME}-${platform.npm}`;
}

/** The binary filename inside a platform package. */
export function binaryName(platform) {
  return platform.os === "win32" ? `${NAME}.exe` : NAME;
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
