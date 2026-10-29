package voice

import (
	"bytes"
	"context"
	"encoding/binary"
	"log/slog"
	"net"
	"testing"
	"time"

	"github.com/disgoorg/godave"
	"github.com/disgoorg/snowflake/v2"
	"github.com/pion/rtcp"
)

func TestAudioPassthroughPreservesLargeFrames(t *testing.T) {
	for _, mode := range AllEncryptionModes {
		t.Run(string(mode), func(t *testing.T) {
			left, right := net.Pipe()
			defer left.Close()
			defer right.Close()
			left.SetDeadline(time.Now().Add(2 * time.Second))
			right.SetDeadline(time.Now().Add(2 * time.Second))
			newConn := func(conn net.Conn) *udpConnImpl {
				u := NewUDPConn(godave.NewNoopSession(slog.Default(), "123", nil), func(uint32) snowflake.ID { return 123 }).(*udpConnImpl)
				u.conn = conn
				u.header[0], u.header[1] = 0x80, RTPPayloadType
				binary.BigEndian.PutUint32(u.header[8:], 42)
				if err := u.SetSecretKey(mode, bytes.Repeat([]byte{42}, 32)); err != nil {
					t.Fatal(err)
				}
				return u
			}
			tx, rx := newConn(left), newConn(right)
			for _, size := range []int{1400, 3, 800} {
				payload := bytes.Repeat([]byte{0x55}, size)
				done := make(chan error, 1)
				go func() { _, err := tx.Write(payload); done <- err }()
				packet, err := rx.ReadPacket()
				if err != nil {
					t.Fatal(err)
				}
				if !bytes.Equal(packet.Opus, payload) {
					t.Fatalf("passthrough truncated: got %d, want %d", len(packet.Opus), size)
				}
				if err := <-done; err != nil {
					t.Fatal(err)
				}
			}
		})
	}
}

func TestTransportFeedbackKeepsFirstArrivalAndNegativeDelta(t *testing.T) {
	f := transportFeedback{}
	at := time.Unix(1700000000, 0)
	f.add(10, at, 42)
	f.add(12, at.Add(time.Millisecond), 42)
	f.add(10, at.Add(2*time.Millisecond), 42)
	f.add(11, at.Add(10*time.Millisecond), 42)
	wire := f.add(13, at.Add(60*time.Millisecond), 42)
	var report rtcp.TransportLayerCC
	if err := report.Unmarshal(wire); err != nil {
		t.Fatal(err)
	}
	if len(report.RecvDeltas) != 4 || report.RecvDeltas[1].Delta != 10000 || report.RecvDeltas[2].Delta != -9000 {
		t.Fatalf("arrival timing changed: %v", report.RecvDeltas)
	}
}

func TestDisconnectRemovesAllOwnedSources(t *testing.T) {
	c := &connImpl{ssrcs: map[uint32]snowflake.ID{10: 1, 11: 1, 12: 1, 20: 2}}
	c.handleMessage(nil, OpcodeClientDisconnect, 0, GatewayMessageDataClientDisconnect{UserID: 1})
	if len(c.ssrcs) != 1 || c.ssrcs[20] != 2 {
		t.Fatal("departed sources retained or another user's sources removed", c.ssrcs)
	}
}

type senderLifecycleConn struct {
	Conn
	calls int
}

func (c *senderLifecycleConn) DAVE() godave.Session {
	return godave.NewNoopSession(slog.Default(), "123", nil)
}
func (c *senderLifecycleConn) SetSpeaking(ctx context.Context, _ SpeakingFlags) error {
	c.calls++
	return context.Canceled
}

type fixedFrameProvider struct{}

func (fixedFrameProvider) ProvideOpusFrame() ([]byte, error) { return []byte{1, 2, 3}, nil }
func (fixedFrameProvider) Close()                            {}

func TestFailedSpeakingDoesNotSendOrLatchState(t *testing.T) {
	conn := &senderLifecycleConn{}
	s := NewAudioSender(slog.Default(), fixedFrameProvider{}, conn).(*defaultAudioSender)
	defer s.Close()
	s.send()
	s.send()
	if conn.calls != 2 || s.sentSpeakingStart {
		t.Fatal("failed speaking command was latched")
	}
}
