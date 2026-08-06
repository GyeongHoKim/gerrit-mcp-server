// Runs the npm CLI the same way on every platform.
//
// npm ships as npm.cmd on Windows, which spawnSync will not find without
// either the extension or a shell.
//
// Naming it explicitly is not enough on its own any more: since the fix for
// CVE-2024-27980, Node refuses to spawn a .cmd or .bat file unless a shell is
// asked for, and fails with EINVAL instead. So Windows gets `shell: true`, and
// with it cmd.exe's quoting rules -- hence quoteForShell below. Every argument
// here is a path this repository generated, never anything a caller supplied,
// so quoting is about paths containing spaces rather than about injection.

import { spawnSync } from "node:child_process";

const NPM = process.platform === "win32" ? "npm.cmd" : "npm";
const NEEDS_SHELL = process.platform === "win32";

/**
 * Quote one argument for cmd.exe, which splits on spaces and does not
 * understand single quotes.
 *
 * cmd.exe expands %VAR% inside double quotes too, and a percent sign cannot be
 * escaped on the command line -- ^ is literal once a quote is open. An argument
 * carrying one is therefore refused rather than passed through mangled into a
 * path that is not the one this asked for. Every argument here is a path this
 * repository generated, so the fix is a build directory without a percent sign
 * in it.
 *
 * @param {string} argument one npm argument
 * @returns {string} the argument, quoted if it needs to be
 */
function quoteForShell(argument) {
  if (!NEEDS_SHELL) {
    return argument;
  }

  if (argument.includes("%")) {
    throw new Error(
      `cmd.exe would expand the percent sign in ${argument}; run this from a path without one`,
    );
  }

  if (!/[\s&|<>^"]/.test(argument)) {
    return argument;
  }

  return `"${argument.replaceAll('"', '\\"')}"`;
}

/**
 * Run npm with the given arguments, throwing if it fails.
 *
 * @param {string[]} args npm arguments
 * @param {string} cwd working directory
 * @param {"inherit"|"pipe"} stdout how to handle npm's stdout
 * @returns {string} captured stdout when piped, otherwise an empty string
 */
export function npm(args, cwd, stdout = "inherit") {
  const options = { cwd, encoding: "utf8", stdio: ["ignore", stdout, "inherit"] };

  // With a shell, the whole command goes in as one already-quoted string.
  // Passing an argument array alongside shell: true is deprecated, because
  // Node would concatenate it without quoting anything.
  const result = NEEDS_SHELL
    ? spawnSync([NPM, ...args.map(quoteForShell)].join(" "), { ...options, shell: true })
    : spawnSync(NPM, args, options);

  if (result.error !== undefined) {
    throw new Error(`could not run ${NPM}: ${result.error.message}`);
  }

  if (result.status !== 0) {
    throw new Error(`${NPM} ${args.join(" ")} failed with exit code ${result.status}`);
  }

  return result.stdout ?? "";
}
