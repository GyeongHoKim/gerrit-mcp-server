package gerrit

import "strings"

// parseFooters extracts the git trailers from a commit message.
//
// Gerrit parses these itself and hands them back from GET /changes/{id}/message,
// but that endpoint arrived in 3.0. Parsing here is what lets one code path
// serve every supported version -- see [Client.GetCommitMessage].
//
// Only the final paragraph is read, and only when the message has more than
// one. A whole commit message of "Bug: 42" is a subject, and reporting a
// subject as an issue reference is worse than missing it. That is a deliberate
// divergence from JGit's getFooterLines, which this otherwise follows.
func parseFooters(message string) map[string]string {
	block := footerBlock(message)
	if block == "" {
		return nil
	}

	footers := map[string]string{}
	last := ""

	for raw := range strings.SplitSeq(block, "\n") {
		line := strings.TrimRight(raw, " \t")

		if parsed, ok := splitFooter(line); ok {
			last = parsed.key

			addFooter(footers, parsed.key, parsed.value)

			continue
		}

		// A line that is not a trailer is skipped rather than disqualifying
		// the whole block, because "(cherry picked from commit abc123)" sits
		// alongside Change-Id in essentially every Gerrit repository. An
		// indented one continues the trailer above it, which is how a long
		// value is wrapped.
		if last != "" && isContinuation(raw) {
			footers[last] = strings.TrimSpace(footers[last] + " " + strings.TrimSpace(line))
		}
	}

	if len(footers) == 0 {
		return nil
	}

	return footers
}

// footerBlock returns the last paragraph of a message, or the empty string
// when there is only one paragraph and therefore no trailers to find.
func footerBlock(message string) string {
	// Line endings are normalised first. A message that reached us with CRLF
	// would otherwise never match the blank line that separates paragraphs.
	trimmed := strings.TrimRight(strings.ReplaceAll(message, "\r\n", "\n"), "\n")

	index := strings.LastIndex(trimmed, "\n\n")
	if index < 0 {
		return ""
	}

	// Past the second newline. Any further blank lines are left in the block
	// and skipped as non-trailer lines by the caller.
	return trimmed[index+2:]
}

// footer is one parsed trailer line. A struct rather than two strings back,
// so that a caller cannot take the key for the value.
type footer struct {
	key   string
	value string
}

// splitFooter reads one trailer line, reporting whether it is one at all.
//
// The split is on the first colon only, so "Bug: http://tracker/1" keeps its
// value rather than losing everything past the scheme.
func splitFooter(line string) (footer, bool) {
	key, value, found := strings.Cut(line, ":")
	if !found || !isFooterKey(key) {
		return footer{}, false
	}

	return footer{key: canonicalFooterKey(key), value: strings.TrimSpace(value)}, true
}

// isFooterKey reports JGit's trailer key charset: one or more ASCII letters,
// digits and hyphens, and nothing else.
func isFooterKey(key string) bool {
	if key == "" {
		return false
	}

	for _, r := range key {
		letter := (r >= 'a' && r <= 'z') || (r >= 'A' && r <= 'Z')
		digit := r >= '0' && r <= '9'

		if !letter && !digit && r != '-' {
			return false
		}
	}

	return true
}

// canonicalFooterKey normalises a trailer key to the capitalisation the
// convention uses, so that "bug", "BUG" and "Bug" are one footer.
//
// [bugFooters] and Gerrit's own footers map are both spelled this way, so
// normalising on the way in is what lets [CommitMessageInfo.Bugs] keep its
// exact-case lookups. Slicing by byte is safe because [isFooterKey] has
// already rejected everything outside ASCII.
func canonicalFooterKey(key string) string {
	parts := strings.Split(key, "-")

	for i, part := range parts {
		if part == "" {
			continue
		}

		parts[i] = strings.ToUpper(part[:1]) + strings.ToLower(part[1:])
	}

	return strings.Join(parts, "-")
}

// addFooter records a trailer, joining a repeated key rather than replacing
// it.
//
// Gerrit's own /message endpoint documents that it keeps only the last of a
// repeated footer. Joining is strictly more useful here and costs nothing:
// [CommitMessageInfo.Bugs] already splits a footer value on commas, so two
// Bug lines yield two ids instead of one.
func addFooter(footers map[string]string, key, value string) {
	if existing := footers[key]; existing != "" {
		footers[key] = existing + ", " + value

		return
	}

	footers[key] = value
}

// isContinuation reports a line that carries on the trailer above it, which
// git spells as leading whitespace.
func isContinuation(line string) bool {
	return strings.HasPrefix(line, " ") || strings.HasPrefix(line, "\t")
}
