package voice

import (
	"bytes"
	"log/slog"
	"net"
	"testing"
	"time"

	"github.com/disgoorg/godave"
	"github.com/disgoorg/snowflake/v2"
	"github.com/pion/rtcp"
	"github.com/pion/rtp"
)

func TestVideoTransportAndAuthenticatedFeedback(t *testing.T) {
	for _, mode := range AllEncryptionModes {
		t.Run(string(mode), func(t *testing.T) {
			left, right := net.Pipe()
			defer left.Close()
			defer right.Close()
			left.SetDeadline(time.Now().Add(2 * time.Second))
			right.SetDeadline(time.Now().Add(2 * time.Second))
			makeConn := func(conn net.Conn) *udpConnImpl {
				u := NewUDPConn(godave.NewNoopSession(slog.Default(), "123", nil), func(uint32) snowflake.ID { return 123 }).(*udpConnImpl)
				u.conn = conn
				if err := u.SetSecretKey(mode, bytes.Repeat([]byte{42}, 32)); err != nil {
					t.Fatal(err)
				}
				return u
			}
			tx, rx := makeConn(left), makeConn(right)
			original := &rtp.Packet{Header: rtp.Header{Version: 2, SSRC: 42, SequenceNumber: 12, Timestamp: 90000, PayloadType: H264PayloadType, Marker: true}, Payload: []byte{0x65, 0x88, 0x84}}
			if err := original.SetExtension(5, []byte{0, 1}); err != nil {
				t.Fatal(err)
			}
			done := make(chan error, 1)
			go func() { done <- tx.WriteVideoPacket(original) }()
			received, err := rx.ReadPacket()
			if err != nil {
				t.Fatal(err)
			}
			if !bytes.Equal(received.Opus, original.Payload) || received.SSRC != 42 || !received.Marker {
				t.Fatal("video header or payload changed")
			}
			if err := <-done; err != nil {
				t.Fatal(err)
			}
			wire, err := (&rtcp.PictureLossIndication{SenderSSRC: 7, MediaSSRC: 42}).Marshal()
			if err != nil {
				t.Fatal(err)
			}
			go func() { rx.sendFeedback(wire, "test"); done <- nil }()
			feedback, err := tx.ReadVideoFeedback()
			if err != nil {
				t.Fatal(err)
			}
			if len(feedback) != 1 {
				t.Fatal("feedback count")
			}
			if pli, ok := feedback[0].(*rtcp.PictureLossIndication); !ok || pli.MediaSSRC != 42 {
				t.Fatal("invalid feedback")
			}
			<-done
		})
	}
}
