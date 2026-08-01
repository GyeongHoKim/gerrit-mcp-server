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

// ChangeCreated names a newly opened change.
func ChangeCreated(change *gerrit.ChangeInfo) string {
	var out strings.Builder

	out.WriteString("Created change ")
	out.WriteString(strconv.Itoa(change.Number))
	out.WriteString(".\n\n")
	writeChange(&out, change)
	out.WriteString("\nThe change is empty until an edit is published to it.\n")

	return out.String()
}

// SubmissionReverted names the changes created to undo a submission.
func SubmissionReverted(changeID string, reverts *gerrit.RevertSubmissionInfo) string {
	changes := reverts.RevertChanges
	if len(changes) == 0 {
		return "Gerrit created no reverts for the submission containing change " + changeID + ".\n"
	}

	var out strings.Builder

	out.WriteString("Created ")
	writeCount(&out, len(changes), "change")
	out.WriteString(" reverting the submission containing ")
	out.WriteString(changeID)
	out.WriteString(".\n")

	for i := range changes {
		out.WriteString("\n")
		writeChange(&out, &changes[i])
	}

	out.WriteString("\nEvery revert still has to be reviewed and submitted.\n")

	return out.String()
}
