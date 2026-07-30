#!/usr/bin/env node
// Fails unless the npm CLI is at least the given version.
//
//   node scripts/require-npm-version.mjs 11.5.1
//
// Trusted publishing needs npm >= 11.5.1. An older CLI does not error -- it
// quietly falls back to looking for a token that the release workflow never
// provides, and the publish fails much later with a confusing 404.

import process from "node:process";

import { npm } from "./npm-cli.mjs";

function compare(a, b) {
  const left = a.split(".").map(Number);
  const right = b.split(".").map(Number);

  for (let i = 0; i < Math.max(left.length, right.length); i += 1) {
    const difference = (left[i] ?? 0) - (right[i] ?? 0);

    if (difference !== 0) {
      return difference;
    }
  }

  return 0;
}

const required = process.argv[2];

if (required === undefined) {
  throw new Error("usage: require-npm-version.mjs <minimum-version>");
}

const actual = npm(["--version"], process.cwd(), "pipe").trim();

if (compare(actual, required) < 0) {
  process.stderr.write(
    `npm ${actual} is too old: trusted publishing requires >= ${required}.\n` +
      `Pin a newer Node in mise.toml, or run \`npm install -g npm@latest\`.\n`,
  );
  process.exit(1);
}

process.stderr.write(`npm ${actual} satisfies >= ${required}\n`);
