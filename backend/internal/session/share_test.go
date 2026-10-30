package session

import (
	"context"
	"testing"

	"github.com/mattcalayo/omarchy-discord/backend/internal/protocol"
)

func TestScreenStopSupersedesLateActions(t *testing.T) {
	s := &screenShare{}
	m := &Manager{video: &videoModule{share: s}}
	_, e := voiceReq(t, m, `{"v":1,"id":1,"command":"screen_share","action":"stop","revision":200}`)
	if e != nil {
		t.Fatal(e)
	}
	_, e = voiceReq(t, m, `{"v":1,"id":2,"command":"screen_share","action":"stop","revision":100}`)
	if e == nil || e.Code != protocol.CodeInvalidArgument {
		t.Fatal("stale request accepted", e)
	}
	ctx, cancel := context.WithCancel(context.Background())
	s.cancel = cancel
	s.status = "selecting"
	_, e = voiceReq(t, m, `{"v":1,"id":3,"command":"screen_share","action":"stop","revision":300}`)
	if e != nil || ctx.Err() == nil {
		t.Fatal("selection was not cancelled", e)
	}
	if s.revision != 300 {
		t.Fatal("request ordering was lost")
	}
}
