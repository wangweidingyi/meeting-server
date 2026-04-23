package protocol

import "testing"

func TestControlTopic(t *testing.T) {
	got := ControlTopic("client-a", "session-1")
	want := "meetings/client-a/session/session-1/control"

	if got != want {
		t.Fatalf("got %s want %s", got, want)
	}
}

func TestControlSubscriptionTopic(t *testing.T) {
	got := ControlSubscriptionTopic()
	want := "meetings/+/session/+/control"

	if got != want {
		t.Fatalf("got %s want %s", got, want)
	}
}

func TestDownstreamTopics(t *testing.T) {
	cases := map[string]string{
		ControlReplyTopic("client-a", "session-1"): "meetings/client-a/session/session-1/control/reply",
		EventsTopic("client-a", "session-1"):       "meetings/client-a/session/session-1/events",
		SttTopic("client-a", "session-1"):          "meetings/client-a/session/session-1/stt",
		SummaryTopic("client-a", "session-1"):      "meetings/client-a/session/session-1/summary",
		ActionItemsTopic("client-a", "session-1"):  "meetings/client-a/session/session-1/action-items",
	}

	for got, want := range cases {
		if got != want {
			t.Fatalf("got %s want %s", got, want)
		}
	}
}
