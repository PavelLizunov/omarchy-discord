package voice

import (
	"context"
	"errors"
	"log/slog"
	"sync"
	"time"

	"github.com/disgoorg/snowflake/v2"
	dvoice "github.com/mattcalayo/omarchy-discord/backend/internal/voicewire"
	davesession "github.com/thomas-vilte/dave-go/session"
)

// Screen owns one independent receiving transport and decoder. It never joins
// voice, captures local devices, publishes video or writes received media.
type Screen struct {
	mu        sync.Mutex
	conn      dvoice.Conn
	video     *camera
	cancel    context.CancelFunc
	done      chan struct{}
	closeOnce sync.Once
	closed    bool
	err       string
}

func OpenScreen(ctx context.Context, state dvoice.State, user snowflake.ID, log *slog.Logger) (*Screen, error) {
	return openScreen(ctx, state, user, log)
}
func openScreen(ctx context.Context, state dvoice.State, user snowflake.ID, log *slog.Logger, extra ...dvoice.ConnConfigOpt) (*Screen, error) {
	if log == nil {
		log = slog.Default()
	}
	s := &Screen{video: newCamera(), done: make(chan struct{})}
	opts := []dvoice.ConnConfigOpt{dvoice.WithConnLogger(log), dvoice.WithConnDaveSessionCreateFunc(davesession.CreateFunc()), dvoice.WithConnGatewayConfigOpts(dvoice.WithGatewayCloseObserver(func(error) { s.fail("Screen connection lost. Select it again.") })), dvoice.WithConnEventHandlerFunc(func(g dvoice.Gateway, op dvoice.Opcode, seq int, data dvoice.GatewayMessageData) {
		if d, ok := data.(dvoice.GatewayMessageDataVideo); ok {
			s.video.announce(d)
		}
	})}
	opts = append(opts, extra...)
	c := dvoice.NewConn(state.GuildID, state.UserID, nil, func() {}, opts...)
	s.conn = c
	opener, ok := c.(interface {
		OpenMedia(context.Context, dvoice.State) error
	})
	if !ok {
		return nil, errors.New("stream transport unavailable")
	}
	if err := opener.OpenMedia(ctx, state); err != nil {
		s.Close()
		return nil, err
	}
	if err := c.Gateway().Send(ctx, dvoice.OpcodeVideo, dvoice.GatewayMessageDataVideo{AudioSSRC: c.Gateway().SSRC(), Streams: []dvoice.VideoStream{}}); err != nil {
		s.Close()
		return nil, err
	}
	if err := c.SetSpeaking(ctx, 0); err != nil {
		s.Close()
		return nil, err
	}
	// Opcode 12 can arrive after Session Description. Wait for an authenticated
	// group and the selected sender; never fabricate an SSRC or bypass DAVE.
	for {
		s.mu.Lock()
		failed := s.err != ""
		s.mu.Unlock()
		if failed {
			s.Close()
			return nil, errors.New("screen connection lost")
		}
		s.video.mu.Lock()
		announced := s.video.streams[user] != 0
		s.video.mu.Unlock()
		if announced && c.DAVE().Ready() {
			break
		}
		select {
		case <-ctx.Done():
			s.Close()
			return nil, errors.New("stream sender or encryption not ready")
		case <-time.After(20 * time.Millisecond):
		}
	}
	ssrc, err := s.video.selectUser(user)
	if err != nil {
		s.Close()
		return nil, err
	}
	if err := c.Gateway().Send(ctx, dvoice.OpcodeMediaSinkWants, s.video.sinkWants(ssrc)); err != nil {
		s.Close()
		return nil, err
	}
	run, cancel := context.WithCancel(context.Background())
	s.mu.Lock()
	s.cancel = cancel
	failed := s.err != ""
	s.mu.Unlock()
	go func() {
		defer close(s.done)
		for run.Err() == nil {
			if !c.DAVE().Ready() {
				select {
				case <-run.Done():
					return
				case <-time.After(20 * time.Millisecond):
				}
				continue
			}
			_ = c.UDP().SetReadDeadline(time.Now().Add(time.Second))
			p, err := c.UDP().ReadPacket()
			if err != nil {
				if run.Err() != nil {
					return
				}
				if c.Gateway().Status() == dvoice.StatusDisconnected {
					s.fail("Screen connection lost. Select it again.")
					return
				}
				select {
				case <-run.Done():
					return
				case <-time.After(20 * time.Millisecond):
				}
				continue
			}
			if p.Type == dvoice.H264PayloadType {
				s.video.receive(p, c.DAVE())
				if transport, ok := c.UDP().(interface{ SendNACK(uint32, []uint16) error }); ok {
					ssrc, missing := s.video.missingPackets()
					if len(missing) > 0 {
						_ = transport.SendNACK(ssrc, missing)
					}
				}
			}
		}
	}()
	if failed {
		s.Close()
		return nil, errors.New("screen connection lost")
	}
	if pli, ok := c.UDP().(interface{ SendPLI(uint32) error }); ok {
		_ = pli.SendPLI(ssrc)
	}
	return s, nil
}

type ScreenSnapshot struct {
	CameraSnapshot
	ScreenObservation
}
type ScreenObservation struct {
	PacketHeaders   map[string]uint64 `json:"packet_headers"`
	ReceiveStages   map[string]uint64 `json:"receive_stages"`
	EncryptionReady bool              `json:"encryption_ready"`
}

func (s *Screen) Snapshot() ScreenSnapshot {
	s.mu.Lock()
	defer s.mu.Unlock()
	r := ScreenSnapshot{CameraSnapshot: s.video.snapshot()}
	if s.err != "" {
		r.Error = s.err
		r.Image = ""
	}
	if s.closed {
		return r
	}
	if s.conn != nil {
		r.EncryptionReady = s.conn.DAVE().Ready()
		if observer, ok := s.conn.UDP().(interface {
			PacketHeaders() map[string]uint64
			CameraReceiveStages() map[string]uint64
		}); ok {
			r.PacketHeaders = observer.PacketHeaders()
			r.ReceiveStages = observer.CameraReceiveStages()
		}
	}
	return r
}
func (s *Screen) fail(message string) {
	s.mu.Lock()
	s.err = message
	started := s.cancel != nil
	s.mu.Unlock()
	if started {
		go s.Close()
	}
}
func (s *Screen) Close() { s.closeOnce.Do(s.close) }
func (s *Screen) close() {
	s.mu.Lock()
	c, cancel := s.conn, s.cancel
	s.conn = nil
	s.cancel = nil
	s.closed = true
	s.mu.Unlock()
	if cancel != nil {
		cancel()
	}
	if c != nil {
		_ = c.UDP().Close()
	}
	if cancel != nil {
		<-s.done
	}
	if c != nil {
		ctx, stop := context.WithTimeout(context.Background(), 3*time.Second)
		c.Close(ctx)
		stop()
	}
	s.video.close()
}
