package voice

import (
	"bytes"
	"context"
	"encoding/binary"
	"errors"
	"log/slog"
	"net"
	"testing"
	"time"

	"github.com/disgoorg/godave"
	"github.com/thomas-vilte/dave-go/frame"
)

// Fixed synthetic media key, not an MLS session or a production crypto adapter.
// It exercises complete-frame authentication before transport fragmentation.
type fixtureVideoCipher struct {
	godave.Session
	key   []byte
	nonce uint32
	fail  bool
}

func (d *fixtureVideoCipher) EncryptH264(ssrc uint32, plain []byte) ([]byte, error) {
	if d.fail {
		return nil, errors.New("fixture refused")
	}
	n := d.nonce
	d.nonce++
	return frame.Encrypt(frame.EncryptParams{Plaintext: plain, Key: d.key, TruncatedNonce: n, UnencryptedRanges: []frame.Range{{Offset: 0, Length: 5}}})
}

func TestVideoWriterAuthenticatedTransport(t *testing.T) {
	for _, mode := range AllEncryptionModes {
		t.Run(string(mode), func(t *testing.T) {
			rx, err := net.ListenUDP("udp", &net.UDPAddr{IP: net.IPv4(127, 0, 0, 1)})
			if err != nil {
				t.Fatal(err)
			}
			defer rx.Close()
			tx, err := net.DialUDP("udp", nil, rx.LocalAddr().(*net.UDPAddr))
			if err != nil {
				t.Fatal(err)
			}
			defer tx.Close()
			ds := &fixtureVideoCipher{Session: godave.NewNoopSession(slog.Default(), "123", nil), key: bytes.Repeat([]byte{0x42}, 16)}
			u := &udpConnImpl{conn: tx, daveSession: ds, ssrc: 1}
			transportKey := bytes.Repeat([]byte{0x24}, 32)
			if err = u.SetSecretKey(mode, transportKey); err != nil {
				t.Fatal(err)
			}
			writer, err := u.NewVideoWriter(42)
			if err != nil {
				t.Fatal(err)
			}
			if _, err = u.NewVideoWriter(1); err == nil {
				t.Fatal("audio SSRC reused")
			}
			plain := append([]byte{0, 0, 0, 1, 0x65}, bytes.Repeat([]byte{0x55}, 2500)...)
			if err = writer.Send(context.Background(), plain, 9000); err != nil {
				t.Fatal(err)
			}
			decrypt, err := NewAEADEncrypter(mode, transportKey)
			if err != nil {
				t.Fatal(err)
			}
			rx.SetReadDeadline(time.Now().Add(time.Second))
			var rebuilt []byte
			count := 0
			buf := make([]byte, 1400)
			for {
				n, _, err := rx.ReadFromUDP(buf)
				if err != nil {
					t.Fatal(err)
				}
				if n > 1200 {
					t.Fatal("wire MTU exceeded")
				}
				wire := buf[:n]
				if wire[1]&127 != 105 || binary.BigEndian.Uint32(wire[8:12]) != 42 || binary.BigEndian.Uint32(wire[4:8]) != 9000 {
					t.Fatal("RTP identity")
				}
				payload, err := decrypt.Decrypt(12, wire)
				if err != nil {
					t.Fatal(err)
				}
				if payload[0]&31 != 28 {
					t.Fatal("expected FU-A")
				}
				if payload[1]&128 != 0 {
					rebuilt = append(rebuilt, 0, 0, 0, 1, (payload[0]&0xe0)|(payload[1]&31))
				}
				rebuilt = append(rebuilt, payload[2:]...)
				count++
				if wire[1]&128 != 0 {
					break
				}
			}
			if count < 3 {
				t.Fatal("fragmentation absent")
			}
			decoded, _, err := frame.Decrypt(frame.DecryptParams{Ciphertext: rebuilt, Key: ds.key})
			if err != nil {
				t.Fatal(err)
			}
			if !bytes.Equal(decoded, plain) {
				t.Fatal("DAVE roundtrip altered frame")
			}
			rebuilt[10] ^= 1
			if _, _, err = frame.Decrypt(frame.DecryptParams{Ciphertext: rebuilt, Key: ds.key}); err == nil {
				t.Fatal("tamper accepted")
			}
			ds.fail = true
			if err = writer.Send(context.Background(), plain, 12000); err == nil {
				t.Fatal("failed crypto transmitted")
			}
			rx.SetReadDeadline(time.Now().Add(20 * time.Millisecond))
			if _, _, err = rx.ReadFromUDP(buf); err == nil {
				t.Fatal("plaintext fallback sent")
			}
		})
	}
}
