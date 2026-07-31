package render

// TopicSet confirms a change's new topic.
func TopicSet(changeID, topic string) string {
	if topic == "" {
		return "Cleared the topic on change " + changeID + ".\n"
	}

	return "Set the topic on change " + changeID + " to " + topic + ".\n"
}

// ReadyForReviewSet confirms a change is asking for review.
func ReadyForReviewSet(_ string) string {
	return ""
}

// WorkInProgressSet confirms a change is no longer asking for review.
func WorkInProgressSet(_ string) string {
	return ""
}
