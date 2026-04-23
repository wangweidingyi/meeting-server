package protocol

func ControlTopic(clientID, sessionID string) string {
	return "meetings/" + clientID + "/session/" + sessionID + "/control"
}

func ControlSubscriptionTopic() string {
	return "meetings/+/session/+/control"
}

func ControlReplyTopic(clientID, sessionID string) string {
	return "meetings/" + clientID + "/session/" + sessionID + "/control/reply"
}

func EventsTopic(clientID, sessionID string) string {
	return "meetings/" + clientID + "/session/" + sessionID + "/events"
}

func SttTopic(clientID, sessionID string) string {
	return "meetings/" + clientID + "/session/" + sessionID + "/stt"
}

func SummaryTopic(clientID, sessionID string) string {
	return "meetings/" + clientID + "/session/" + sessionID + "/summary"
}

func ActionItemsTopic(clientID, sessionID string) string {
	return "meetings/" + clientID + "/session/" + sessionID + "/action-items"
}
