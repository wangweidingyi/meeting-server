package mqtt

import (
	"testing"
	"time"

	paho "github.com/eclipse/paho.mqtt.golang"
)

func TestBuildPahoClientOptionsKeepsOrderedCallbacksForControlMessages(t *testing.T) {
	options := buildPahoClientOptions(PahoClientOptions{
		BrokerURL: "tcp://127.0.0.1:1883",
		ClientID:  "meeting-server",
		Username:  "user-1",
		Password:  "pass-1",
		KeepAlive: 30 * time.Second,
	})

	reader := paho.NewOptionsReader(options)

	if !reader.Order() {
		t.Fatal("expected paho client options to preserve message order")
	}
	if !reader.AutoReconnect() {
		t.Fatal("expected auto reconnect to stay enabled")
	}
	if !reader.ConnectRetry() {
		t.Fatal("expected connect retry to stay enabled")
	}
	if reader.ClientID() != "meeting-server" {
		t.Fatalf("unexpected client id %q", reader.ClientID())
	}
}
