package render

import (
	"strings"

	"github.com/GyeongHoKim/gerrit-mcp-server/internal/gerrit"
)

// Reviewers renders reviewer suggestions, people and groups alike.
//
// Groups keep their member count: adding a group of twelve is a different act
// from adding one person, and the count is what says so.
func Reviewers(suggestions []gerrit.SuggestedReviewerInfo) string {
	if len(suggestions) == 0 {
		return "No reviewer suggestions.\n"
	}

	var out strings.Builder

	writeCount(&out, len(suggestions), "suggestion")
	out.WriteString(".\n\n")

	for i := range suggestions {
		out.WriteString("  ")
		writeSuggestion(&out, &suggestions[i])
		out.WriteString("\n")
	}

	return out.String()
}

// writeSuggestion appends one suggestion, which is either a person or a group.
func writeSuggestion(out *strings.Builder, suggestion *gerrit.SuggestedReviewerInfo) {
	if group := suggestion.Group; group != nil {
		out.WriteString("group ")
		out.WriteString(group.Name)
		out.WriteString(" (")
		writeCount(out, suggestion.Count, "member")
		out.WriteString(")")

		return
	}

	if account := suggestion.Account; account != nil {
		out.WriteString(account.Display())

		// The address is what disambiguates two people with the same name.
		if account.Email != "" && account.Email != account.Display() {
			out.WriteString(" <")
			out.WriteString(account.Email)
			out.WriteString(">")
		}

		return
	}

	out.WriteString("unknown suggestion")
}

// ReviewerAdded confirms who was added to a change.
func ReviewerAdded(changeID string, result *gerrit.ReviewerResult) string {
	var out strings.Builder

	for _, group := range []struct {
		label    string
		accounts []gerrit.AccountInfo
	}{
		{label: "reviewer", accounts: result.Reviewers},
		{label: "CC", accounts: result.CCs},
	} {
		for i := range group.accounts {
			out.WriteString("Added ")
			out.WriteString(group.accounts[i].Display())
			out.WriteString(" as ")
			out.WriteString(group.label)
			out.WriteString(" on change ")
			out.WriteString(changeID)
			out.WriteString(".\n")
		}
	}

	if out.Len() == 0 {
		// Gerrit accepted the request but named nobody, which happens when
		// the account was already on the change.
		return "Nobody was added to change " + changeID + "; they may already be on it.\n"
	}

	return out.String()
}
