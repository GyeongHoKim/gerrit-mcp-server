package render

// TopicSet confirms a change's new topic.
func TopicSet(changeID, topic string) string {
	if topic == "" {
		return "Cleared the topic on change " + changeID + ".\n"
	}

	return "Set the topic on change " + changeID + " to " + topic + ".\n"
}
