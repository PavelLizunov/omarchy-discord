package voice

import (
	"testing"

	dvoice "github.com/mattcalayo/omarchy-discord/backend/internal/voicewire"
)

func TestCameraDispatchObservationWithoutSelection(t *testing.T) {
	v := newCamera()
	v.receive(&dvoice.Packet{Type: dvoice.H264PayloadType, SSRC: 42}, nil)
	s := v.snapshot()
	if s.Dispatched != 1 || s.SSRCMismatch != 1 || s.Packets != 0 || s.Sequence != 0 {
		t.Fatalf("dispatch must remain distinct from selected packets and frames: %+v", s)
	}
}
