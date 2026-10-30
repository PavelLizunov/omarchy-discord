package session

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/mattcalayo/omarchy-discord/backend/internal/protocol"
	"github.com/mattcalayo/omarchy-discord/backend/internal/voice"
)

func TestVoiceActionsDoNotWaitForScreenRetirement(t *testing.T) {
	for _, command := range []string{
		`{"v":1,"id":1,"command":"voice_leave"}`,
		`{"v":1,"id":2,"command":"voice_join","guild_id":"200000000000000001","channel_id":"300000000000000004"}`,
	} {
		for _, status := range []string{"selecting", "connecting", "waiting", "sharing", "stopping"} {
			t.Run(command+status, func(t *testing.T) {
				m, _, fv := voiceManager(t)
				ctx, cancel := context.WithCancel(context.Background())
				defer cancel()
				retirement := make(chan struct{})
				defer close(retirement)
				m.mu.Lock()
				s := m.video.share
				s.mu.Lock()
				s.cancel = cancel
				s.done = retirement
				s.status = status
				s.mu.Unlock()
				m.mu.Unlock()
				result := make(chan *protocol.Error, 1)
				go func() { _, e := m.Handle(context.Background(), req(t, command)); result <- e }()
				select {
				case e := <-result:
					if e != nil {
						t.Fatal(e)
					}
				case <-time.After(time.Second):
					t.Fatal("voice command waited for media retirement")
				}
				if ctx.Err() == nil {
					t.Fatal("capture was not cancelled")
				}
				if fv.took() == "" {
					t.Fatal("voice engine was not reached")
				}
			})
		}
	}
}

func TestScreenStoppingOverridesBroadcastSnapshot(t *testing.T) {
	s := &screenShare{status: "stopping", broadcast: &voice.Broadcast{}}
	if got := s.snapshot().Status; got != "stopping" {
		t.Fatal(got)
	}
	m := &Manager{video: &videoModule{share: s}}
	got, e := voiceReq(t, m, `{"v":1,"id":1,"command":"screen_share","action":"status"}`)
	if e != nil || got.(voice.BroadcastSnapshot).Status != "stopping" {
		t.Fatal(got, e)
	}
}

func TestReauthRetiresScreenCapture(t *testing.T) {
	m, n, _ := voiceManager(t)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	done := make(chan struct{})
	m.video.share.cancel = cancel
	m.video.share.done = done
	go func() { <-ctx.Done(); close(done) }()
	m.reauth(n, errors.New("session expired"))
	if ctx.Err() == nil || m.video != nil {
		t.Fatal("reauth retained capture")
	}
}
