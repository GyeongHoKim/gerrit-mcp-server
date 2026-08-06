#!/usr/bin/env node
// Checks the agent skills under skills/.
//
//   node scripts/check-skills.mjs
//
// A skill is installed by tooling that parses its frontmatter, so malformed
// frontmatter does not fail loudly -- the skill is skipped, or installed under
// the wrong name, and nobody finds out until an agent that should have used it
// does not. That failure is invisible from inside this repository, which is
// what makes it worth a check here.

import { readFile, readdir, stat } from "node:fs/promises";
import path from "node:path";
import process from "node:process";

const ROOT = path.resolve(import.meta.dirname, "..");
const SKILLS = path.join(ROOT, "skills");

// The description is the only text an agent carries before the skill triggers,
// so it decides whether the skill is ever used. Too short cannot describe the
// tool; too long stops being a summary and costs context in every session.
const DESCRIPTION_MIN = 80;
const DESCRIPTION_MAX = 1024;

/**
 * Split the YAML frontmatter off a SKILL.md.
 *
 * Only the two scalar keys that matter are read. A real YAML parser would be a
 * dependency for six lines of fixed shape.
 *
 * @param {string} body the file contents
 * @returns {Record<string, string>} the frontmatter keys
 */
function frontmatter(body) {
  const match = /^---\r?\n([\s\S]*?)\r?\n---\r?\n/.exec(body);

  if (match === null) {
    throw new Error("has no --- frontmatter block at the top");
  }

  const fields = {};
  let key = null;

  for (const line of match[1].split(/\r?\n/)) {
    const started = /^([a-z-]+):\s*(.*)$/.exec(line);

    if (started !== null) {
      key = started[1];
      fields[key] = started[2].trim();
      continue;
    }

    // A folded continuation line, which is how a long description is written.
    if (key !== null && line.trim() !== "") {
      fields[key] = `${fields[key]} ${line.trim()}`.trim();
    }
  }

  return fields;
}

async function checkSkill(name, problems) {
  const directory = path.join(SKILLS, name);
  const manifest = path.join(directory, "SKILL.md");

  let body;
  try {
    body = await readFile(manifest, "utf8");
  } catch {
    problems.push(`${name} has no SKILL.md`);

    return;
  }

  let fields;
  try {
    fields = frontmatter(body);
  } catch (error) {
    problems.push(`${name}/SKILL.md ${error.message}`);

    return;
  }

  // The installer takes the directory name from `name`, so a mismatch installs
  // the skill somewhere nothing looks for it.
  if (fields.name !== name) {
    problems.push(`${name}/SKILL.md declares name: ${fields.name ?? "nothing"}, want ${name}`);
  }

  const description = fields.description ?? "";

  if (description.length < DESCRIPTION_MIN) {
    problems.push(
      `${name}/SKILL.md has a ${description.length}-character description, want at least ` +
        `${DESCRIPTION_MIN}: it is the only text an agent sees before deciding to use the skill`,
    );
  }

  if (description.length > DESCRIPTION_MAX) {
    problems.push(
      `${name}/SKILL.md has a ${description.length}-character description, want at most ` +
        `${DESCRIPTION_MAX}`,
    );
  }

  await checkLinks(name, directory, body, problems);
}

/**
 * Check that every relative link in a skill points at a file that exists.
 *
 * A skill that points an agent at a reference which is not there wastes the
 * turn that followed the link.
 */
async function checkLinks(name, directory, body, problems) {
  for (const [, target] of body.matchAll(/\]\((?!https?:)([^)#]+)/g)) {
    try {
      await stat(path.join(directory, target));
    } catch {
      problems.push(`${name}/SKILL.md links to ${target}, which does not exist`);
    }
  }
}

async function main() {
  let entries;
  try {
    entries = await readdir(SKILLS, { withFileTypes: true });
  } catch {
    throw new Error(`${SKILLS} not found`);
  }

  const names = entries.filter((entry) => entry.isDirectory()).map((entry) => entry.name);

  if (names.length === 0) {
    throw new Error(`${SKILLS} holds no skills`);
  }

  const problems = [];

  for (const name of names) {
    await checkSkill(name, problems);
  }

  if (problems.length > 0) {
    throw new Error(`the skills are not installable:\n  ${problems.join("\n  ")}`);
  }

  process.stderr.write(`${names.length} skill(s) check out: ${names.join(", ")}\n`);
}

await main();
