package voice

import (
	"encoding/json"
	"testing"
)

func TestVoiceResumeAcknowledgesDocumentedSequence(t *testing.T) {
	for _, field := range []string{"seq", "s"} {
		var event GatewayMessage
		if err := json.Unmarshal([]byte(`{"op":9,"d":null,"`+field+`":42}`), &event); err != nil {
			t.Fatal(err)
		}
		if event.Seq != 42 {
			t.Fatalf("%s sequence ignored", field)
		}
	}
	wire, err := json.Marshal(GatewayMessageDataResume{SeqAck: 42})
	if err != nil {
		t.Fatal(err)
	}
	var fields map[string]any
	if err = json.Unmarshal(wire, &fields); err != nil {
		t.Fatal(err)
	}
	if fields["seq_ack"] != float64(42) || fields["seq"] != nil {
		t.Fatalf("invalid v8 resume acknowledgment: %s", wire)
	}
}
