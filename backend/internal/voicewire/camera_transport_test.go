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

type cameraDaveProbe struct {
	godave.Session
	calls int
}

func (d *cameraDaveProbe) Decrypt(id godave.UserID, in, out []byte) (int, error) {
	d.calls++
	return d.Session.Decrypt(id, in, out)
}

// Real encrypted RTP101 fragments must reach the camera without per-packet
// DAVE authentication. Opus still takes the existing authenticated audio path.
func TestCameraTransport101AndOpusDemultiplexing(t *testing.T) {
	if H264PayloadType != 101 {
		t.Fatal("H264 advertisement must match observed RTP101")
	}
	for _, mode := range AllEncryptionModes {
		t.Run(string(mode), func(t *testing.T) {
			reader, writer := net.Pipe()
			defer reader.Close()
			reader.SetReadDeadline(time.Now().Add(time.Second))
			d := &cameraDaveProbe{Session: godave.NewNoopSession(slog.Default(), "123", nil)}
			u := NewUDPConn(d, func(uint32) snowflake.ID { return 123 }).(*udpConnImpl)
			u.conn = reader
			key := bytes.Repeat([]byte{42}, 32)
			if err := u.SetSecretKey(mode, key); err != nil {
				t.Fatal(err)
			}
			enc, err := NewAEADEncrypter(mode, key)
			if err != nil {
				t.Fatal(err)
			}
			camera := []byte{0x65, 1, 2, 3, 4}
			audio := []byte{0xf8, 0xff, 0xfe}
			go func() {
				defer writer.Close()
				for _, pt := range []byte{109, H264PayloadType, RTPPayloadType} {
					var h [12]byte
					h[0] = 0x80
					h[1] = pt | 0x80
					binary.BigEndian.PutUint16(h[2:], 7)
					binary.BigEndian.PutUint32(h[8:], 42)
					payload := camera
					if pt == RTPPayloadType {
						payload = audio
					}
					writer.Write(enc.Encrypt(h, payload))
				}
			}()
			p, err := u.ReadPacket()
			if err != nil {
				t.Fatal(err)
			}
			if p.Type != 101 || p.SSRC != 42 || !p.Marker || !bytes.Equal(p.Opus, camera) || d.calls != 0 {
				t.Fatalf("camera dropped or authenticated before assembly: %+v calls%d", p, d.calls)
			}
			p, err = u.ReadPacket()
			if err != nil {
				t.Fatal(err)
			}
			if p.Type != RTPPayloadType || !bytes.Equal(p.Opus, audio) || d.calls != 1 {
				t.Fatalf("audio path changed: %+v calls%d", p, d.calls)
			}
			if u.PacketHeaders()["101:42"] != 1 || u.PacketHeaders()["109:42"] != 1 {
				t.Fatal("raw observation missing")
			}
		})
	}
}
