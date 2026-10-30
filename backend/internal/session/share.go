package session

import (
	"context"
	"fmt"
	"sync"
	"time"

	"github.com/diamondburned/arikawa/v3/discord"
	"github.com/diamondburned/arikawa/v3/utils/ws"
	"github.com/diamondburned/ningen/v3"
	dsnowflake "github.com/disgoorg/snowflake/v2"
	"github.com/mattcalayo/omarchy-discord/backend/internal/protocol"
	"github.com/mattcalayo/omarchy-discord/backend/internal/voice"
)

type screenShare struct {
	mu        sync.Mutex
	cancel    context.CancelFunc
	done      chan struct{}
	key       string
	created   *streamCreateEvent
	server    *streamServerEvent
	signal    chan struct{}
	broadcast *voice.Broadcast
	status    string
	err       string
	revision  uint64
}

func (s *screenShare) snapshot() voice.BroadcastSnapshot {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.broadcast != nil && s.status != "stopping" {
		return s.broadcast.Snapshot()
	}
	status := s.status
	if status == "" {
		status = "idle"
	}
	return voice.BroadcastSnapshot{Status: status, Error: s.err}
}
func (s *screenShare) stop() {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.cancel != nil {
		s.status = "stopping"
		s.cancel()
	}
}
func (s *screenShare) close() {
	s.stop()
	s.mu.Lock()
	done := s.done
	s.mu.Unlock()
	if done != nil {
		<-done
	}
}
func (s *screenShare) notify() {
	select {
	case s.signal <- struct{}{}:
	default:
	}
}
func (s *screenShare) create(e *streamCreateEvent) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.key == e.Key {
		copy := *e
		s.created = &copy
		s.notify()
	}
}
func (s *screenShare) update(e *streamServerEvent) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.key == e.Key {
		copy := *e
		s.server = &copy
		s.notify()
	}
}
func (s *screenShare) deleted(e *streamDeleteEvent) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.key == e.Key && s.cancel != nil {
		s.cancel()
	}
}

type shareCommand struct {
	Code      ws.OpCode `json:"-"`
	Type      string    `json:"type,omitempty"`
	GuildID   string    `json:"guild_id,omitempty"`
	ChannelID string    `json:"channel_id,omitempty"`
	Key       string    `json:"stream_key,omitempty"`
	Paused    *bool     `json:"paused,omitempty"`
}

func (s *shareCommand) Op() ws.OpCode         { return s.Code }
func (*shareCommand) EventType() ws.EventType { return "" }

func (m *Manager) shareScreen(ctx context.Context, req *protocol.Request) (any, *protocol.Error) {
	var p struct {
		Action   string `json:"action"`
		Revision uint64 `json:"revision"`
		Height   int    `json:"height"`
		FPS      int    `json:"fps"`
	}
	if e := req.Params(&p); e != nil {
		return nil, e
	}
	m.mu.Lock()
	mod := m.video
	m.mu.Unlock()
	if mod == nil || mod.share == nil {
		return nil, protocol.Errorf(protocol.CodeGatewayUnavailable, "screen sharing unavailable")
	}
	s := mod.share
	if p.Action == "status" {
		return s.snapshot(), nil
	}
	if p.Action == "stop" {
		s.mu.Lock()
		if p.Revision == 0 || p.Revision <= s.revision {
			s.mu.Unlock()
			return nil, protocol.Errorf(protocol.CodeInvalidArgument, "screen request superseded")
		}
		s.revision = p.Revision
		if s.cancel != nil {
			s.status = "stopping"
			s.cancel()
		}
		s.mu.Unlock()
		return s.snapshot(), nil
	}
	if p.Action != "start" {
		return nil, protocol.Errorf(protocol.CodeInvalidArgument, "choose start or stop")
	}
	if p.Height == 0 {
		p.Height = 720
	}
	if p.FPS == 0 {
		p.FPS = 30
	}
	if (p.Height != 360 && p.Height != 720 && p.Height != 1080) || (p.FPS != 15 && p.FPS != 30) {
		return nil, protocol.Errorf(protocol.CodeInvalidArgument, "unsupported screen quality")
	}
	v, e := m.liveVoice()
	if e != nil {
		return nil, e
	}
	st := v.State()
	if st.Status != voice.StatusConnected {
		return nil, protocol.Errorf(protocol.CodeGatewayUnavailable, "join voice before sharing your screen")
	}
	n, e := m.cachedSession()
	if e != nil {
		return nil, e
	}
	self, err := n.Cabinet.Me()
	if err != nil {
		return nil, protocol.Errorf(protocol.CodeGatewayUnavailable, "account unavailable")
	}
	own, err := n.Cabinet.VoiceState(st.GuildID, self.ID)
	if err != nil || own.SessionID == "" || own.ChannelID != st.ChannelID {
		return nil, protocol.Errorf(protocol.CodeGatewayUnavailable, "voice session unavailable")
	}
	if own.SelfStream {
		return nil, protocol.Errorf(protocol.CodeInvalidArgument, "this account already has an active screen share")
	}
	if !n.Offline().HasPermissions(st.ChannelID, discord.PermissionViewChannel|discord.PermissionConnect|discord.PermissionStream) {
		return nil, protocol.Errorf(protocol.CodeForbidden, "no permission to share in this room")
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.video != mod || m.voiceState.Status != voice.StatusConnected || m.voiceState.ChannelID != st.ChannelID {
		return nil, protocol.Errorf(protocol.CodeGatewayUnavailable, "voice session changed")
	}
	s.mu.Lock()
	if p.Revision == 0 || p.Revision <= s.revision {
		s.mu.Unlock()
		return nil, protocol.Errorf(protocol.CodeInvalidArgument, "screen request superseded")
	}
	s.revision = p.Revision
	if s.done != nil {
		s.mu.Unlock()
		return nil, protocol.Errorf(protocol.CodeInvalidArgument, "screen sharing is already starting or active")
	}
	run, cancel := context.WithCancel(context.Background())
	s.cancel = cancel
	s.done = make(chan struct{})
	s.signal = make(chan struct{}, 1)
	s.status = "selecting"
	s.err = ""
	s.mu.Unlock()
	go s.run(run, n, st, dsnowflake.ID(self.ID), own.SessionID, p.Height, p.FPS)
	return s.snapshot(), nil
}
func (s *screenShare) run(ctx context.Context, n *ningen.State, st voice.State, self dsnowflake.ID, sessionID string, height, fps int) {
	var key string
	var capture *voice.Capture
	var broadcast *voice.Broadcast
	var failure string
	defer func() {
		if broadcast != nil {
			broadcast.Close()
		} else if capture != nil {
			capture.Close()
		}
		if key != "" {
			bound, cancel := context.WithTimeout(context.Background(), 3*time.Second)
			_ = n.SendGateway(bound, &streamCommand{Code: 19, Key: key})
			cancel()
		}
		s.mu.Lock()
		defer s.mu.Unlock()
		s.broadcast = nil
		s.key = ""
		s.created = nil
		s.server = nil
		s.cancel = nil
		s.status = "idle"
		if failure != "" {
			s.status = "error"
			s.err = failure
		}
		close(s.done)
		s.done = nil
	}()
	var err error
	capture, err = voice.StartCapture(ctx, height, fps)
	if err != nil {
		failure = err.Error()
		return
	}
	selection, cancel := context.WithTimeout(ctx, 125*time.Second)
	first, err := capture.Next(selection)
	cancel()
	if err != nil {
		if ctx.Err() == nil {
			failure = "Screen selection cancelled or capture unavailable"
		}
		return
	}
	own, err := n.Cabinet.VoiceState(st.GuildID, discord.UserID(self))
	if ctx.Err() != nil {
		return
	}
	if err != nil || own.ChannelID != st.ChannelID || own.SessionID != sessionID || own.SelfStream {
		failure = "Voice or screen session changed during source selection"
		return
	}
	// No stream is advertised before the portal has supplied an encoded frame.
	key = fmt.Sprintf("guild:%s:%s:%s", st.GuildID, st.ChannelID, self)
	s.mu.Lock()
	s.key = key
	s.status = "connecting"
	s.mu.Unlock()
	handshake, cancel := context.WithTimeout(ctx, 20*time.Second)
	defer cancel()
	if err = n.SendGateway(handshake, &shareCommand{Code: 18, Type: "guild", GuildID: st.GuildID.String(), ChannelID: st.ChannelID.String()}); err != nil {
		failure = "Discord screen request failed"
		return
	}
	paused := false
	if err = n.SendGateway(handshake, &shareCommand{Code: 22, Key: key, Paused: &paused}); err != nil {
		failure = "Discord screen activation failed"
		return
	}
	for {
		s.mu.Lock()
		create, server := s.created, s.server
		s.mu.Unlock()
		if create != nil && server != nil {
			state, err := streamState(*create, *server, self, sessionID)
			if err != nil {
				failure = "Invalid screen handshake"
				return
			}
			broadcast, err = voice.OpenBroadcast(ctx, state, capture, first, height, fps)
			if err != nil {
				if ctx.Err() == nil {
					failure = "Screen transport could not start"
				}
				return
			}
			s.mu.Lock()
			s.broadcast = broadcast
			s.status = "sharing"
			s.created = nil
			s.server = nil
			s.mu.Unlock()
			ticker := time.NewTicker(time.Second)
			defer ticker.Stop()
			for {
				select {
				case <-ctx.Done():
					return
				case <-ticker.C:
					snap := broadcast.Snapshot()
					if snap.Status == "error" || snap.Status == "stopped" {
						failure = snap.Error
						return
					}
				}
			}
		}
		select {
		case <-handshake.Done():
			if ctx.Err() == nil {
				failure = "Discord screen handshake timed out"
			}
			return
		case <-s.signal:
		}
	}
}
