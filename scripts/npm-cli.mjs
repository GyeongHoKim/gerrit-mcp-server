// Runs the npm CLI the same way on every platform.
//
// npm ships as npm.cmd on Windows, which spawnSync will not find without
// either the extension or a shell. Naming it explicitly avoids shell quoting
// rules differing between cmd.exe and sh.

import { spawnSync } from "node:child_process";

const NPM = process.platform === "win32" ? "npm.cmd" : "npm";

/**
 * Run npm with the given arguments, throwing if it fails.
 *
 * @param {string[]} args npm arguments
 * @param {string} cwd working directory
 * @param {"inherit"|"pipe"} stdout how to handle npm's stdout
 * @returns {string} captured stdout when piped, otherwise an empty string
 */
export function npm(args, cwd, stdout = "inherit") {
  const result = spawnSync(NPM, args, {
    cwd,
    encoding: "utf8",
    stdio: ["ignore", stdout, "inherit"],
  });

  if (result.error !== undefined) {
    throw new Error(`could not run ${NPM}: ${result.error.message}`);
  }

  if (result.status !== 0) {
    throw new Error(`${NPM} ${args.join(" ")} failed with exit code ${result.status}`);
  }

  return result.stdout ?? "";
}
