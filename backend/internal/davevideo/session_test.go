//go:build libdave

package davevideo

import (
	"bytes"
	"context"
	"crypto/elliptic"
	"encoding/binary"
	dvoice "github.com/mattcalayo/omarchy-discord/backend/internal/voicewire"
	"log/slog"
	"net"
	"os/exec"
	"sync"
	"testing"
	"time"

	"github.com/disgoorg/godave"
	"github.com/disgoorg/snowflake/v2"
	"github.com/thomas-vilte/dave-go/frame"
)

type callbacks struct{ key, commit []byte }

func (c *callbacks) SendMLSKeyPackage(b []byte) error { c.key = append([]byte(nil), b...); return nil }
func (c *callbacks) SendMLSCommitWelcome(b []byte) error {
	c.commit = append([]byte(nil), b...)
	return nil
}
func (*callbacks) SendReadyForTransition(uint16) error   { return nil }
func (*callbacks) SendInvalidCommitWelcome(uint16) error { return nil }
func syntheticSession(t *testing.T) *Session {
	t.Helper()
	cb := &callbacks{}
	s := New(slog.Default(), "123", cb).(*Session)
	t.Cleanup(func() { s.Close() })
	s.SetChannelID(42)
	s.AddUser(godave.UserID("123"))
	s.OnSelectProtocolAck(1)
	if len(cb.key) == 0 {
		t.Fatal("missing native key package")
	}
	curve := elliptic.P256()
	pub := elliptic.Marshal(curve, curve.Params().Gx, curve.Params().Gy)
	external := append([]byte{0x40, 65}, pub...)
	external = append(external, 0, 1, 4, 0, 1, 1, 0)
	s.OnDaveMLSExternalSenderPackage(external)
	s.OnDaveMLSProposals([]byte{0, 0})
	if len(cb.commit) == 0 {
		t.Fatal("no native MLS synthetic commit")
	}
	s.OnDaveMLSPrepareCommitTransition(0, cb.commit)
	if !s.Ready() {
		t.Fatal("native MLS epoch not active")
	}
	return s
}
func TestOfficialLibdaveH264Encryption(t *testing.T) {
	s := syntheticSession(t)
	plain := append([]byte{0, 0, 0, 1, 0x65}, bytes.Repeat([]byte{0x55}, 2500)...)
	nonces := map[uint32]bool{}
	s.AssignSsrcToCodec(99, godave.CodecOpus)
	for i := 0; i < 50; i++ {
		if i%5 == 0 {
			s.OnDavePrepareTransition(0, 1)
			s.OnDavePrepareTransition(7, 1)
			s.OnDaveExecuteTransition(7)
			s.OnDaveExecuteTransition(7)
		}
		audio := make([]byte, s.MaxEncryptedFrameSize(3))
		n, err := s.Encrypt(99, []byte{0xf8, 0xff, 0xfe}, audio)
		if err != nil {
			t.Fatal(err)
		}
		a, err := frame.Parse(audio[:n])
		if err != nil {
			t.Fatal(err)
		}
		if nonces[a.TruncatedNonce] {
			t.Fatal("audio/video nonce reused")
		}
		nonces[a.TruncatedNonce] = true
		encrypted, err := s.EncryptH264(42, plain)
		if err != nil {
			t.Fatal(err)
		}
		if bytes.Equal(encrypted, plain) {
			t.Fatal("plaintext passthrough")
		}
		parsed, err := frame.Parse(encrypted)
		if err != nil {
			t.Fatal(err)
		}
		if nonces[parsed.TruncatedNonce] {
			t.Fatal("native nonce reused")
		}
		nonces[parsed.TruncatedNonce] = true
		decoded, err := s.DecryptVideo("123", encrypted)
		if err != nil {
			t.Fatal(err)
		}
		if !bytes.Equal(decoded, plain) {
			t.Fatal("native H264 authenticated roundtrip altered frame")
		}
	}
	encrypted, _ := s.EncryptH264(42, plain)
	encrypted[10] ^= 1
	if _, err := s.DecryptVideo("123", encrypted); err == nil {
		t.Fatal("tamper accepted")
	}
	s.OnSelectProtocolAck(1)
	if s.Ready() {
		t.Fatal("old sending ratchet survived new group")
	}
	if _, err := s.EncryptH264(42, plain); err == nil {
		t.Fatal("unready epoch sent video")
	}
}
func TestOfficialLibdaveEncodedH264RTP(t *testing.T) {
	s := syntheticSession(t)
	plain, err := exec.Command("ffmpeg", "-hide_banner", "-loglevel", "error", "-f", "lavfi", "-i", "testsrc2=size=160x90:rate=1", "-frames:v", "1", "-pix_fmt", "yuv420p", "-c:v", "libx264", "-threads", "1", "-tune", "zerolatency", "-x264-params", "aud=1", "-f", "h264", "pipe:1").Output()
	if err != nil {
		t.Fatal(err)
	}
	for _, mode := range dvoice.AllEncryptionModes {
		rx, err := net.ListenUDP("udp", &net.UDPAddr{IP: net.IPv4(127, 0, 0, 1)})
		if err != nil {
			t.Fatal(err)
		}
		t.Cleanup(func() { rx.Close() })
		u := dvoice.NewUDPConn(s, func(uint32) snowflake.ID { return 123 })
		t.Cleanup(func() { u.Close() })
		done := make(chan error, 1)
		go func() {
			buf := make([]byte, 74)
			n, addr, e := rx.ReadFromUDP(buf)
			if e == nil && n == 74 {
				buf[0], buf[1] = 0, 2
				copy(buf[8:], "127.0.0.1")
				binary.BigEndian.PutUint16(buf[72:], 12345)
				_, e = rx.WriteToUDP(buf, addr)
			}
			done <- e
		}()
		if _, _, err = u.Open(context.Background(), "127.0.0.1", rx.LocalAddr().(*net.UDPAddr).Port, 1); err != nil {
			t.Fatal(err)
		}
		if err = <-done; err != nil {
			t.Fatal(err)
		}
		key := bytes.Repeat([]byte{0x24}, 32)
		if err = u.SetSecretKey(mode, key); err != nil {
			t.Fatal(err)
		}
		writer, err := u.(interface {
			NewVideoWriter(uint32) (*dvoice.VideoWriter, error)
		}).NewVideoWriter(42)
		if err != nil {
			t.Fatal(err)
		}
		if err = writer.Send(context.Background(), plain, 9000); err != nil {
			t.Fatal(err)
		}
		decrypt, err := dvoice.NewAEADEncrypter(mode, key)
		if err != nil {
			t.Fatal(err)
		}
		rx.SetReadDeadline(time.Now().Add(time.Second))
		buf := make([]byte, 1400)
		var assembled []byte
		for {
			n, _, err := rx.ReadFromUDP(buf)
			if err != nil {
				t.Fatal(err)
			}
			payload, err := decrypt.Decrypt(12, buf[:n])
			if err != nil {
				t.Fatal(err)
			}
			if payload[0]&31 == 28 {
				if payload[1]&128 != 0 {
					assembled = append(assembled, 0, 0, 0, 1, (payload[0]&0xe0)|(payload[1]&31))
				}
				assembled = append(assembled, payload[2:]...)
			} else {
				assembled = append(assembled, 0, 0, 0, 1)
				assembled = append(assembled, payload...)
			}
			if buf[1]&128 != 0 {
				break
			}
		}
		decoded, err := s.DecryptVideo("123", assembled)
		if err != nil {
			t.Fatal(err)
		}
		// libdave normalizes 3-byte Annex B boundaries to 4-byte ones before auth.
		cmd := exec.Command("ffmpeg", "-hide_banner", "-loglevel", "error", "-f", "h264", "-i", "pipe:0", "-frames:v", "1", "-f", "null", "-")
		cmd.Stdin = bytes.NewReader(decoded)
		if out, err := cmd.CombinedOutput(); err != nil {
			t.Fatalf("native encrypted video no longer decodes: %v %s", err, out)
		}
		u.Close()
		rx.Close()
	}
}

func TestOfficialLibdaveFailClosedBoundaries(t *testing.T) {
	s := syntheticSession(t)
	if _, err := s.EncryptH264(42, make([]byte, (4<<20)+1)); err == nil {
		t.Fatal("oversized frame accepted")
	}
	s.SetChannelID(43)
	if s.Ready() {
		t.Fatal("room change retained ready sender")
	}
	s.OnSelectProtocolAck(0)
	if s.Ready() {
		t.Fatal("plaintext protocol enabled sender")
	}
	s.OnDavePrepareTransition(999, 1)
	s.OnDaveExecuteTransition(999)
	if _, err := s.EncryptH264(42, []byte{0, 0, 0, 1, 0x65, 1}); err == nil {
		t.Fatal("old native epoch revived after disabled protocol")
	}
}

func TestOfficialLibdaveCloseConcurrent(t *testing.T) {
	s := syntheticSession(t)
	var wg sync.WaitGroup
	for i := 0; i < 4; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for j := 0; j < 10; j++ {
				s.Ready()
				s.EncryptH264(42, []byte{0, 0, 0, 1, 0x65, 0x55})
			}
		}()
	}
	s.Close()
	wg.Wait()
	s.Close()
	if s.Ready() {
		t.Fatal("closed session ready")
	}
	if _, err := s.EncryptH264(42, []byte{0, 0, 0, 1, 0x65, 0x55}); err == nil {
		t.Fatal("closed session encrypted")
	}
}
