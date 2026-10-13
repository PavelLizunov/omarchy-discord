package session

import (
	"context"
	"errors"
	"fmt"
	"net"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/diamondburned/arikawa/v3/discord"
	"github.com/diamondburned/ningen/v3"
	dsnowflake "github.com/disgoorg/snowflake/v2"
	"github.com/mattcalayo/omarchy-discord/backend/internal/protocol"
	"github.com/mattcalayo/omarchy-discord/backend/internal/voice"
	dvoice "github.com/mattcalayo/omarchy-discord/backend/internal/voicewire"
)

// videoModule serializes viewer actions separately from the voice engine.
// Handshake secrets are retained only while opening the selected transport.
type videoModule struct {
	mu       sync.Mutex
	op       sync.Mutex
	key      string
	created  *streamCreateEvent
	server   *streamServerEvent
	signal   chan struct{}
	screen   *voice.Screen
	leave    func(string)
	revision uint64
	cancel   context.CancelFunc
	retiring string
	retired  chan struct{}
}
type videoChoice struct {
	UserID string `json:"user_id"`
	Kind   string `json:"kind"`
}
type videoSnapshot struct {
	Choices []videoChoice `json:"choices"`
	Kind    string        `json:"kind"`
	voice.CameraSnapshot
	Screen *voice.ScreenObservation `json:"screen,omitempty"`
}

func (m *Manager) installVideoHandlers(n *ningen.State) {
	owner := &videoModule{leave: func(key string) {
		ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
		defer cancel()
		_ = n.SendGateway(ctx, &streamCommand{Code: 19, Key: key})
	}}
	m.video = owner
	n.AddSyncHandler(func(e *streamCreateEvent) {
		owner.mu.Lock()
		defer owner.mu.Unlock()
		if e.Key == owner.key {
			c := *e
			owner.created = &c
			owner.notify()
		}
	})
	n.AddSyncHandler(func(e *streamServerEvent) {
		owner.mu.Lock()
		defer owner.mu.Unlock()
		if e.Key == owner.key {
			c := *e
			owner.server = &c
			owner.notify()
		}
	})
	n.AddSyncHandler(func(e *streamDeleteEvent) {
		owner.mu.Lock()
		owner.acknowledge(e.Key)
		var s *voice.Screen
		if e.Key == owner.key {
			owner.revision++
			if owner.cancel != nil {
				owner.cancel()
			}
			s = owner.screen
			owner.screen = nil
			owner.key = ""
			owner.created = nil
			owner.server = nil
		}
		owner.mu.Unlock()
		if s != nil {
			go s.Close()
		}
	})
}

// Called under mu: the gateway dispatch lane orders delete before a fresh pair.
func (v *videoModule) acknowledge(key string) {
	if key == v.retiring && v.retired != nil {
		close(v.retired)
		v.retired = nil
		v.retiring = ""
	}
}
func (v *videoModule) notify() {
	if v.signal != nil {
		select {
		case v.signal <- struct{}{}:
		default:
		}
	}
}

// acquire supersedes an in-flight watch before waiting for its bounded cleanup.
func (v *videoModule) acquire(revision uint64) bool {
	v.mu.Lock()
	if revision == 0 || revision <= v.revision {
		v.mu.Unlock()
		return false
	}
	v.revision = revision
	if v.cancel != nil {
		v.cancel()
	}
	v.mu.Unlock()
	v.op.Lock()
	v.mu.Lock()
	fresh := v.revision == revision
	v.mu.Unlock()
	if !fresh {
		v.op.Unlock()
	}
	return fresh
}

// reset requires op; callbacks do not acquire op or perform blocking cleanup.
func (v *videoModule) reset() {
	v.mu.Lock()
	key := v.key
	s := v.screen
	v.screen = nil
	v.key = ""
	v.created = nil
	v.server = nil
	v.cancel = nil
	var ack chan struct{}
	if key != "" && v.leave != nil {
		ack = make(chan struct{})
		v.retiring = key
		v.retired = ack
	}
	v.mu.Unlock()
	if s != nil {
		s.Close()
	}
	if key != "" && v.leave != nil {
		v.leave(key)
		select {
		case <-ack:
		case <-time.After(3 * time.Second):
		}
	}
}

// invalidate is nonblocking and cannot close a later selection when queued.
func (v *videoModule) invalidate() {
	v.mu.Lock()
	v.revision++
	if v.cancel != nil {
		v.cancel()
	}
	s := v.screen
	v.screen = nil
	v.key = ""
	v.created = nil
	v.server = nil
	v.mu.Unlock()
	if s != nil {
		go s.Close()
	}
}
func (v *videoModule) close() {
	v.mu.Lock()
	v.revision++
	if v.cancel != nil {
		v.cancel()
	}
	v.mu.Unlock()
	v.op.Lock()
	defer v.op.Unlock()
	v.reset()
}
func (m *Manager) videoSnapshot() (any, *protocol.Error) {
	v, e := m.liveVoice()
	if e != nil {
		return nil, e
	}
	n, e := m.cachedSession()
	if e != nil {
		return nil, e
	}
	camera, ok := v.(interface{ Cameras() voice.CameraSnapshot })
	if !ok {
		return nil, protocol.Errorf(protocol.CodeGatewayUnavailable, "video receive unavailable")
	}
	r := videoSnapshot{Choices: []videoChoice{}, Kind: "camera", CameraSnapshot: camera.Cameras()}
	for _, s := range r.Streams {
		r.Choices = append(r.Choices, videoChoice{UserID: s.UserID, Kind: "camera"})
	}
	st := v.State()
	states, _ := n.Offline().Cabinet.VoiceStates(st.GuildID)
	for _, u := range states {
		if st.Status == voice.StatusConnected && u.ChannelID == st.ChannelID && u.SelfStream {
			r.Choices = append(r.Choices, videoChoice{UserID: u.UserID.String(), Kind: "screen"})
		}
	}
	m.mu.Lock()
	mod := m.video
	m.mu.Unlock()
	if mod != nil {
		mod.mu.Lock()
		s := mod.screen
		mod.mu.Unlock()
		if sink, ok := v.(interface{ SetScreenAudio(*voice.Screen) }); ok {
			sink.SetScreenAudio(s)
		}
		if s != nil {
			r.Kind = "screen"
			snapshot := s.Snapshot()
			r.CameraSnapshot = snapshot.CameraSnapshot
			r.Screen = &snapshot.ScreenObservation
		}
	}
	sort.Slice(r.Choices, func(i, j int) bool {
		return r.Choices[i].UserID+r.Choices[i].Kind < r.Choices[j].UserID+r.Choices[j].Kind
	})
	return r, nil
}
func (m *Manager) watchVideo(ctx context.Context, req *protocol.Request) (any, *protocol.Error) {
	var p struct {
		Kind     string `json:"kind"`
		UserID   string `json:"user_id"`
		Revision uint64 `json:"revision"`
	}
	if e := req.Params(&p); e != nil {
		return nil, e
	}
	if p.Kind != "camera" && p.Kind != "screen" && p.Kind != "stop" {
		return nil, protocol.Errorf(protocol.CodeInvalidArgument, "choose camera or screen")
	}
	m.mu.Lock()
	mod := m.video
	m.mu.Unlock()
	if mod == nil {
		return nil, protocol.Errorf(protocol.CodeGatewayUnavailable, "video module unavailable")
	}
	var id dsnowflake.ID
	if p.Kind != "stop" {
		sf, e := parseSnowflake(p.UserID, "user_id")
		if e != nil {
			return nil, e
		}
		id = dsnowflake.ID(sf)
	}
	if !mod.acquire(p.Revision) {
		return nil, protocol.Errorf(protocol.CodeInvalidArgument, "video request superseded")
	}
	defer mod.op.Unlock()
	mod.reset()
	if v, e := m.liveVoice(); e == nil {
		if sink, ok := v.(interface{ SetScreenAudio(*voice.Screen) }); ok {
			sink.SetScreenAudio(nil)
		}
	}
	bound, cancel := context.WithTimeout(ctx, 15*time.Second)
	defer cancel()
	mod.mu.Lock()
	if mod.revision != p.Revision {
		mod.mu.Unlock()
		return nil, protocol.Errorf(protocol.CodeInvalidArgument, "video request superseded")
	}
	mod.cancel = cancel
	mod.mu.Unlock()
	defer func() { mod.mu.Lock(); mod.cancel = nil; mod.mu.Unlock() }()
	ctx = bound
	v, e := m.liveVoice()
	if e != nil {
		return nil, e
	}
	receiver, ok := v.(interface {
		WatchCamera(context.Context, dsnowflake.ID, uint64) error
	})
	if !ok {
		return nil, protocol.Errorf(protocol.CodeGatewayUnavailable, "camera unavailable")
	}
	if p.Kind == "camera" || p.Kind == "stop" {
		if id != 0 {
			if e := m.cameraParticipant(id); e != nil {
				return nil, e
			}
		}
		if err := receiver.WatchCamera(ctx, id, p.Revision); err != nil {
			return nil, protocol.Errorf(protocol.CodeDiscordError, "%v", err)
		}
		return m.videoSnapshot()
	}
	st := v.State()
	if st.Status != voice.StatusConnected {
		return nil, protocol.Errorf(protocol.CodeGatewayUnavailable, "join voice before watching screen")
	}
	n, e := m.cachedSession()
	if e != nil {
		return nil, e
	}
	vs, err := n.Offline().Cabinet.VoiceState(st.GuildID, discord.UserID(id))
	if err != nil || vs.ChannelID != st.ChannelID || !vs.SelfStream {
		return nil, protocol.Errorf(protocol.CodeInvalidArgument, "participant has no screen stream in this room")
	}
	if !n.Offline().HasPermissions(st.ChannelID, discord.PermissionViewChannel|discord.PermissionConnect) {
		return nil, protocol.Errorf(protocol.CodeForbidden, "no permission to watch this room")
	}
	self, err := n.Cabinet.Me()
	if err != nil {
		return nil, protocol.Errorf(protocol.CodeGatewayUnavailable, "session unavailable")
	}
	if discord.UserID(id) == self.ID {
		return nil, protocol.Errorf(protocol.CodeInvalidArgument, "cannot watch your own screen")
	}
	own, err := n.Cabinet.VoiceState(st.GuildID, self.ID)
	if err != nil || own.SessionID == "" {
		return nil, protocol.Errorf(protocol.CodeGatewayUnavailable, "voice session identity unavailable")
	}
	if err := receiver.WatchCamera(ctx, 0, p.Revision); err != nil {
		return nil, protocol.Errorf(protocol.CodeDiscordError, "stop camera before screen: %v", err)
	}
	key := fmt.Sprintf("guild:%s:%s:%s", st.GuildID, st.ChannelID, id)
	mod.mu.Lock()
	if mod.retiring == key {
		mod.mu.Unlock()
		return nil, protocol.Errorf(protocol.CodeGatewayUnavailable, "previous stream disconnect not acknowledged; wait before retrying")
	}
	if mod.revision != p.Revision || bound.Err() != nil {
		mod.mu.Unlock()
		return nil, protocol.Errorf(protocol.CodeInvalidArgument, "video request superseded")
	}
	mod.key = key
	mod.signal = make(chan struct{}, 1)
	mod.cancel = cancel
	mod.mu.Unlock()
	defer func() {
		mod.mu.Lock()
		mod.cancel = nil
		mod.created = nil
		mod.server = nil
		opened := mod.screen != nil
		mod.mu.Unlock()
		if !opened {
			mod.reset()
		}
	}()
	if err = n.SendGateway(bound, &streamCommand{Code: 20, Key: key}); err != nil {
		return nil, protocol.Errorf(protocol.CodeDiscordError, "stream watch request failed")
	}
	for {
		mod.mu.Lock()
		create, server := mod.created, mod.server
		mod.mu.Unlock()
		if create != nil && server != nil {
			state, err := streamState(*create, *server, dsnowflake.ID(self.ID), own.SessionID)
			if err != nil {
				return nil, protocol.Errorf(protocol.CodeInvalidArgument, "%v", err)
			}
			screen, err := voice.OpenScreen(bound, state, id, nil)
			if err != nil {
				return nil, protocol.Errorf(protocol.CodeDiscordError, "stream connection failed; select again")
			}
			mod.mu.Lock()
			fresh := mod.revision == p.Revision && mod.key == key && bound.Err() == nil
			if fresh {
				mod.screen = screen
			}
			mod.mu.Unlock()
			if !fresh {
				screen.Close()
				return nil, protocol.Errorf(protocol.CodeInvalidArgument, "video request superseded")
			}
			return m.videoSnapshot()
		}
		select {
		case <-bound.Done():
			return nil, protocol.Errorf(protocol.CodeDiscordError, "stream handshake unavailable or timed out")
		case <-mod.signal:
		}
	}
}
func streamState(c streamCreateEvent, s streamServerEvent, self dsnowflake.ID, session string) (dvoice.State, error) {
	parse := func(s string) (dsnowflake.ID, error) {
		n, e := strconv.ParseUint(s, 10, 64)
		if e != nil || n < 2 {
			return 0, errors.New("invalid stream RTC identity")
		}
		return dsnowflake.ID(n), nil
	}
	server, e := parse(c.Server)
	if e != nil {
		return dvoice.State{}, e
	}
	channel, e := parse(c.Channel)
	if e != nil {
		return dvoice.State{}, e
	}
	host := strings.TrimPrefix(s.Endpoint, "wss://")
	name, _, e := net.SplitHostPort(host)
	if e != nil {
		name = host
	}
	if strings.ContainsAny(host, "/@?#\\\r\n") || !(strings.HasSuffix(name, ".discord.media") || strings.HasSuffix(name, ".discord.gg")) {
		return dvoice.State{}, errors.New("untrusted stream RTC endpoint")
	}
	if s.Token == "" || session == "" || self == 0 || c.Key == "" || c.Key != s.Key {
		return dvoice.State{}, errors.New("stream handshake mismatch")
	}
	// discord-native-voice stream_client.py at e2ace1d uses rtc_server_id-1
	// for Go Live MLS, not the enclosing guild voice channel.
	return dvoice.State{GuildID: server, ChannelID: channel, MediaGroupID: server - 1, MediaOnly: true, UserID: self, SessionID: session, Token: s.Token, Endpoint: host}, nil
}
