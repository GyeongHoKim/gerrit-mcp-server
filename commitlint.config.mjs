// Commit message rules for gerrit-mcp-server.
//
// Enforced locally by the lefthook commit-msg hook and again in CI, so that a
// bypassed hook still fails the pull request.

export default {
  extends: ["@commitlint/config-conventional"],
  rules: {
    "type-enum": [
      2,
      "always",
      [
        "feat", // user-visible capability
        "fix", // bug fix
        "perf", // performance work
        "refactor", // behaviour-preserving change
        "docs", // documentation only
        "test", // tests only
        "build", // build system, toolchain, dependencies
        "ci", // workflows and automation
        "chore", // housekeeping with no product impact
        "revert", // reverts a previous commit
      ],
    ],

    // A scope is mandatory: it makes the changelog readable and forces the
    // author to say which part of the system moved.
    //
    // The value itself is free-form. There is deliberately no scope-enum: an
    // allow-list goes stale the moment someone adds a package, and the failure
    // mode is a rejected commit for a scope that is perfectly reasonable.
    // Lower-case is still enforced so that "gerrit" and "Gerrit" do not both
    // end up in the changelog.
    //
    // Scopes in common use: gerrit, mcp, tools, config, auth, npm, release,
    // ci, deps, lint, docs. Prefer an existing one over inventing a synonym.
    "scope-empty": [2, "never"],
    "scope-case": [2, "always", "lower-case"],

    "subject-empty": [2, "never"],
    "subject-case": [2, "always", "lower-case"],
    "subject-full-stop": [2, "never", "."],

    "header-max-length": [2, "always", 72],
    "body-leading-blank": [2, "always"],
    "body-max-line-length": [2, "always", 100],
    "footer-leading-blank": [2, "always"],
  },
};
