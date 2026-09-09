package render

import (
	"regexp"
	"strconv"
	"strings"

	"github.com/GyeongHoKim/gerrit-mcp-server/internal/gerrit"
)

// accountPlaceholder is how Gerrit 3.x writes an account into a message body.
var accountPlaceholder = regexp.MustCompile(`<GERRIT_ACCOUNT_(\d+)>`)

// systemAuthor names the author of a message Gerrit wrote itself, which
// arrives with no account at all.
const systemAuthor = "Gerrit"

// Messages renders a change's log in the order Gerrit sends it, oldest first.
func Messages(messages []gerrit.ChangeMessage) string {
	if len(messages) == 0 {
		return "No change messages.\n"
	}

	var out strings.Builder

	writeCount(&out, len(messages), "change message")
	out.WriteString(".\n")

	for i := range messages {
		writeMessage(&out, &messages[i])
	}

	return out.String()
}

// writeMessage appends one message: a header line, then the indented body.
func writeMessage(out *strings.Builder, message *gerrit.ChangeMessage) {
	out.WriteString("\n[")
	out.WriteString(message.ID)
	out.WriteString("] ")

	if message.Author != nil {
		out.WriteString(message.Author.Display())
	} else {
		out.WriteString(systemAuthor)
	}

	if !message.Date.IsZero() {
		out.WriteString(" · ")
		out.WriteString(message.Date.Format(minuteLayout))
		out.WriteString(" UTC")
	}

	if message.RevisionNumber > 0 {
		out.WriteString(" · ps")
		out.WriteString(strconv.Itoa(message.RevisionNumber))
	}

	if message.Tag != "" {
		out.WriteString(" [")
		out.WriteString(message.Tag)
		out.WriteString("]")
	}

	out.WriteString("\n")
	out.WriteString(indentBody(resolveAccounts(message)))
	out.WriteString("\n")
}

// resolveAccounts puts names back where Gerrit left <GERRIT_ACCOUNT_N>.
//
// A placeholder with no entry in the list is left as it came. Dropping it
// would turn "X and Y looked at this" into "X and looked at this", which
// reads as a sentence and is wrong.
func resolveAccounts(message *gerrit.ChangeMessage) string {
	if len(message.AccountsInMessage) == 0 {
		return message.Message
	}

	names := make(map[string]string, len(message.AccountsInMessage))
	for i := range message.AccountsInMessage {
		account := &message.AccountsInMessage[i]
		names[strconv.Itoa(account.AccountID)] = account.Display()
	}

	return accountPlaceholder.ReplaceAllStringFunc(message.Message, func(placeholder string) string {
		id := accountPlaceholder.FindStringSubmatch(placeholder)[1]
		if name, ok := names[id]; ok {
			return name
		}

		return placeholder
	})
}

// indentBody prefixes every non-empty line with four spaces.
//
// Empty lines stay empty rather than becoming four spaces, so the golden
// files that pin this output survive an editor that strips trailing
// whitespace.
func indentBody(body string) string {
	lines := strings.Split(body, "\n")

	for i, line := range lines {
		if line != "" {
			lines[i] = "    " + line
		}
	}

	return strings.Join(lines, "\n")
}
