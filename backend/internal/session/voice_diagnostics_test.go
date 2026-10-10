package session

import (
	"context"
	"testing"

	"github.com/mattcalayo/omarchy-discord/backend/internal/protocol"
	"github.com/mattcalayo/omarchy-discord/backend/internal/voice"
)

type diagnosticVoice struct {
	fakeVoice
	tests int
}

func (v *diagnosticVoice) Diagnostics() voice.Diagnostics {
	return voice.Diagnostics{Active: true, SentAgeMS: -1}
}
func (v *diagnosticVoice) TestOutput() error { v.tests++; return nil }
func TestVoiceDiagnosticsCommands(t *testing.T) {
	m := New(&fakeKeyring{})
	run := func(command string) (any, *protocol.Error) {
		r, e := protocol.DecodeRequest([]byte(`{"v":1,"id":1,"command":"` + command + `"}`))
		if e != nil {
			t.Fatal(e)
		}
		return m.Handle(context.Background(), r)
	}
	if _, e := run("voice_diagnostics"); e == nil {
		t.Fatal("missing session accepted")
	}
	v := &diagnosticVoice{}
	m.voice = v
	result, e := run("voice_diagnostics")
	if e != nil || !result.(voice.Diagnostics).Active || v.tests != 0 {
		t.Fatal(result, e)
	}
	if _, e := run("voice_test_output"); e != nil || v.tests != 1 {
		t.Fatal(e)
	}
	if v.took() != "" {
		t.Fatal("diagnostics changed call flags or connection")
	}
}
