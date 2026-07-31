package render

import (
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
func ChangeAbandoned(_ *gerrit.ChangeInfo) string {
	return ""
}

// ChangeReverted names the change created to undo another.
func ChangeReverted(_ string, _ *gerrit.ChangeInfo) string {
	return ""
}
