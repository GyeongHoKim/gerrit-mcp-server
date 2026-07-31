package render

import (
	"strconv"
	"strings"

	"github.com/GyeongHoKim/gerrit-mcp-server/internal/gerrit"
)

// TopicSet confirms a change's new topic.
func TopicSet(changeID, topic string) string {
	if topic == "" {
		return "Cleared the topic on change " + changeID + ".\n"
	}

	return "Set the topic on change " + changeID + " to " + topic + ".\n"
}

// ReadyForReviewSet confirms a change is asking for review.
func ReadyForReviewSet(changeID string) string {
	return "Change " + changeID + " is ready for review; its reviewers have been notified.\n"
}

// WorkInProgressSet confirms a change is no longer asking for review.
func WorkInProgressSet(changeID string) string {
	return "Change " + changeID + " is marked work-in-progress and no longer asks for review.\n"
}

// ChangeAbandoned confirms a change is no longer being worked on.
func ChangeAbandoned(change *gerrit.ChangeInfo) string {
	return "Abandoned change " + strconv.Itoa(change.Number) + ": " + change.Subject + "\n"
}

// ChangeReverted names the change created to undo another.
func ChangeReverted(changeID string, revert *gerrit.ChangeInfo) string {
	var out strings.Builder

	out.WriteString("Created change ")
	out.WriteString(strconv.Itoa(revert.Number))
	out.WriteString(" to revert ")
	out.WriteString(changeID)
	out.WriteString(".\n\n")
	writeChange(&out, revert)
	// The revert is a proposal, not an undo: saying so stops an agent
	// concluding the original is already gone.
	out.WriteString("\nThe revert still has to be reviewed and submitted.\n")

	return out.String()
}
