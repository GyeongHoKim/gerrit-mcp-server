package render

import (
	"strconv"
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

	out.WriteString(strconv.Itoa(len(suggestions)))
	out.WriteString(" suggestion")

	if len(suggestions) != 1 {
		out.WriteString("s")
	}

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
		out.WriteString(strconv.Itoa(suggestion.Count))
		out.WriteString(" member")

		if suggestion.Count != 1 {
			out.WriteString("s")
		}

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
