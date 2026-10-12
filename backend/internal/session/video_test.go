package session

import (
	"context"
	"sync"
	"testing"
	"time"

	"github.com/diamondburned/arikawa/v3/gateway"
	"github.com/diamondburned/arikawa/v3/utils/ws"
	dsnowflake "github.com/disgoorg/snowflake/v2"
	"github.com/mattcalayo/omarchy-discord/backend/internal/voice"
)

func TestStreamHandshakeIdentityAndEndpoint(t *testing.T) {
	c := streamCreateEvent{Key: "guild:2:3:4", Server: "500", Channel: "600"}
	s := streamServerEvent{Key: c.Key, Endpoint: "wss://rtc.discord.media:443", Token: "fixture"}
	st, e := streamState(c, s, 700, "fixture-session")
	if e != nil || st.GuildID != 500 || st.ChannelID != 600 || st.MediaGroupID != 499 || !st.MediaOnly || st.SessionID != "fixture-session" {
		t.Fatalf("identity: %+v %v", st, e)
	}
	for _, endpoint := range []string{"http://rtc.discord.media", "localhost:80", "127.0.0.1:443", "rtc.discord.media.evil.test:443", "rtc.discord.gg/path", "user@rtc.discord.gg:443", "rtc.discord.media\n"} {
		bad := s
		bad.Endpoint = endpoint
		if _, e := streamState(c, bad, 700, "fixture-session"); e == nil {
			t.Fatal(endpoint)
		}
	}
	bad := c
	bad.Key = "guild:2:3:5"
	if _, e := streamState(bad, s, 700, "fixture-session"); e == nil {
		t.Fatal("cross-stream secrets accepted")
	}
	bad = c
	bad.Server = "1"
	if _, e := streamState(bad, s, 700, "fixture-session"); e == nil {
		t.Fatal("MLS group underflow")
	}
	if _, e := streamState(c, s, 700, ""); e == nil {
		t.Fatal("missing parent session")
	}
	for _, name := range []ws.EventType{"STREAM_CREATE", "STREAM_SERVER_UPDATE", "STREAM_DELETE"} {
		constructor := gateway.OpUnmarshalers.Lookup(0, name)
		if constructor == nil || constructor().EventType() != name {
			t.Fatal("gateway codec registration", name)
		}
	}
}
func TestVideoSameKeyRetirementBeforeFreshEvents(t *testing.T) {
	v := &videoModule{key: "guild:2:3:4"}
	var sawBoundary bool
	v.leave = func(key string) {
		v.mu.Lock()
		sawBoundary = v.key == "" && v.retiring == key && v.retired != nil
		v.acknowledge(key)
		v.mu.Unlock()
	}
	v.op.Lock()
	v.reset()
	v.op.Unlock()
	if !sawBoundary || v.retiring != "" {
		t.Fatal("fresh handshake can reuse unretired key")
	}
}
func TestVideoNewRevisionCancelsHandshakeAndStaleStop(t *testing.T) {
	v := &videoModule{}
	if !v.acquire(100) {
		t.Fatal("first action rejected")
	}
	ctx, cancel := context.WithCancel(context.Background())
	v.mu.Lock()
	v.cancel = cancel
	v.mu.Unlock()
	done := make(chan bool, 1)
	go func() {
		fresh := v.acquire(101)
		if fresh {
			v.reset()
			v.op.Unlock()
		}
		done <- fresh
	}()
	select {
	case <-ctx.Done():
	case <-time.After(time.Second):
		t.Fatal("superseded handshake not cancelled")
	}
	v.op.Unlock()
	if !<-done {
		t.Fatal("new stop lost")
	}
	if v.acquire(100) {
		v.op.Unlock()
		t.Fatal("stale close replaced newer watch")
	}
	var wg sync.WaitGroup
	for i := 0; i < 10; i++ {
		wg.Add(1)
		go func() { defer wg.Done(); v.close() }()
	}
	wg.Wait()
}

type videoVoiceProbe struct {
	fakeVoice
	watches int
}

func (v *videoVoiceProbe) Cameras() voice.CameraSnapshot {
	return voice.CameraSnapshot{Streams: []voice.CameraStream{{UserID: "42", SSRC: 123}}}
}
func (v *videoVoiceProbe) WatchCamera(context.Context, dsnowflake.ID, uint64) error {
	v.watches++
	return nil
}
func TestVideoCameraAdmissionBothCommands(t *testing.T) {
	m, _, _ := voiceManager(t)
	v := &videoVoiceProbe{}
	v.state = voice.State{Status: voice.StatusConnected, GuildID: guildOmar, ChannelID: chVoice}
	m.voice = v
	for _, command := range []string{"video_watch", "voice_watch_camera"} {
		_, e := voiceReq(t, m, `{"v":1,"id":601,"command":"`+command+`","kind":"camera","user_id":"42","revision":2000}`)
		if e == nil {
			t.Fatal("stale announced nonparticipant admitted", command)
		}
	}
	if v.watches != 0 {
		t.Fatal("rejection subscribed camera")
	}
}
func TestVideoRejectsOutOfRoomBeforeGateway(t *testing.T) {
	m, _, fv := voiceManager(t)
	fv.state.Status = "connected"
	_, e := voiceReq(t, m, `{"v":1,"id":501,"command":"video_watch","kind":"screen","user_id":"42","revision":1000}`)
	if e == nil {
		t.Fatal("out-of-room watcher admitted")
	}
	if fv.took() != "" {
		t.Fatal("stream action modified voice", fv.took())
	}
	_, e = voiceReq(t, m, `{"v":1,"id":502,"command":"video_watch","kind":"publish","user_id":"42","revision":1001}`)
	if e == nil {
		t.Fatal("publishing admitted")
	}
}
