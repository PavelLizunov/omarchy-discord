package voice

import (
	"bytes"
	"encoding/binary"
	"log/slog"
	"net"
	"testing"
	"time"

	"github.com/disgoorg/godave"
	"github.com/disgoorg/snowflake/v2"
)

func TestAnnouncedRTXRestoresOriginalSequence(t *testing.T) {
	for _, mode := range AllEncryptionModes {
		t.Run(string(mode), func(t *testing.T) {
			r, w := net.Pipe()
			defer r.Close()
			defer w.Close()
			r.SetDeadline(time.Now().Add(time.Second))
			w.SetDeadline(time.Now().Add(time.Second))
			u := NewUDPConn(godave.NewNoopSession(slog.Default(), "123", nil), func(uint32) snowflake.ID { return 123 }).(*udpConnImpl)
			u.conn = r
			u.AnnounceVideo(GatewayMessageDataVideo{UserID: 123, Streams: []VideoStream{{SSRC: 42, RTXSSRC: 43}}})
			key := bytes.Repeat([]byte{42}, 32)
			if e := u.SetSecretKey(mode, key); e != nil {
				t.Fatal(e)
			}
			enc, _ := NewAEADEncrypter(mode, key)
			go func() {
				h := [12]byte{0x80, H264PayloadType + 1 | 128}
				binary.BigEndian.PutUint16(h[2:4], 77)
				binary.BigEndian.PutUint32(h[4:8], 9000)
				binary.BigEndian.PutUint32(h[8:], 43)
				w.Write(enc.Encrypt(h, []byte{0x12, 0x34, 0x65, 1, 2, 3}))
			}()
			p, e := u.ReadPacket()
			if e != nil {
				t.Fatal(e)
			}
			if p.Type != H264PayloadType || p.SSRC != 42 || p.Sequence != 0x1234 || !p.Marker || !bytes.Equal(p.Opus, []byte{0x65, 1, 2, 3}) {
				t.Fatal("RTX not depacketized", p)
			}
		})
	}
}

func TestRTXOwnershipRemovedBeforeReuse(t *testing.T) {
	u := &udpConnImpl{}
	u.AnnounceVideo(GatewayMessageDataVideo{UserID: 123, Streams: []VideoStream{{SSRC: 42, RTXSSRC: 43}}})
	if u.rtxSources[43] != 42 {
		t.Fatal("mapping absent")
	}
	u.AnnounceVideo(GatewayMessageDataVideo{UserID: 123})
	if u.rtxSources[43] != 0 {
		t.Fatal("departed RTX retained")
	}
}

func TestRTXOwnershipReassignmentAndDuplicateBound(t *testing.T) {
	u := &udpConnImpl{}
	var streams []VideoStream
	for i := 0; i < 2000; i++ {
		streams = append(streams, VideoStream{SSRC: 42, RTXSSRC: 43})
	}
	u.AnnounceVideo(GatewayMessageDataVideo{UserID: 123, Streams: streams})
	if len(u.rtxOwners) != 1 || len(u.rtxSources) != 1 {
		t.Fatal("duplicate metadata unbounded")
	}
	u.AnnounceVideo(GatewayMessageDataVideo{UserID: 124, Streams: []VideoStream{{SSRC: 84, RTXSSRC: 43}}})
	u.AnnounceVideo(GatewayMessageDataVideo{UserID: 123})
	if u.rtxSources[43] != 84 {
		t.Fatal("old owner removed new mapping")
	}
}
