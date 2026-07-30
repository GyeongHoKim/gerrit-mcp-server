#!/usr/bin/env node
// Downloads the Gerrit REST API reference into doc/ (gitignored).
//
//   node scripts/fetch-gerrit-docs.mjs [branch]
//
// Gerrit publishes no OpenAPI description, so this AsciiDoc source is the
// authoritative definition of the API this server wraps. It is fetched on
// demand rather than vendored: it is ~700 KB of Apache-2.0 licensed material
// that would otherwise sit in an Elastic-2.0 repository forever.

import { mkdir, writeFile } from "node:fs/promises";
import path from "node:path";
import process from "node:process";

const ROOT = path.resolve(import.meta.dirname, "..");
const BRANCH = process.argv[2] ?? "stable-3.14";
const BASE = "https://gerrit.googlesource.com/gerrit/+";

const DOCUMENTS = [
  "rest-api",
  "rest-api-access",
  "rest-api-accounts",
  "rest-api-changes",
  "rest-api-config",
  "rest-api-groups",
  "rest-api-projects",
];

async function fetchDocument(name) {
  // Gitiles serves raw files base64 encoded when asked for format=TEXT.
  const url = `${BASE}/refs/heads/${BRANCH}/Documentation/${name}.txt?format=TEXT`;
  const response = await fetch(url);

  if (!response.ok) {
    throw new Error(`GET ${url} returned ${response.status} ${response.statusText}`);
  }

  return Buffer.from(await response.text(), "base64").toString("utf8");
}

async function main() {
  const target = path.join(ROOT, "doc", `rest-api-${BRANCH.replace(/^stable-/u, "")}`);
  await mkdir(target, { recursive: true });

  const results = await Promise.all(
    DOCUMENTS.map(async (name) => {
      const content = await fetchDocument(name);
      await writeFile(path.join(target, `${name}.txt`), content);

      return { name, bytes: content.length };
    }),
  );

  for (const { name, bytes } of results) {
    process.stderr.write(`  ${name}.txt  ${bytes} bytes\n`);
  }

  process.stderr.write(
    `fetched ${results.length} documents from ${BRANCH} into ${path.relative(ROOT, target)}\n`,
  );
}

await main();
