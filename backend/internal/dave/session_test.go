package dave

import (
	"bytes"
	"crypto/ecdh"
	"crypto/rand"
	"log/slog"
	"testing"

	"github.com/disgoorg/godave"
	"github.com/disgoorg/godave/libdave"
	"github.com/pion/rtp/codecs"
)

type callbacks struct{}

func (callbacks) SendMLSKeyPackage([]byte) error        { return nil }
func (callbacks) SendMLSCommitWelcome([]byte) error     { return nil }
func (callbacks) SendReadyForTransition(uint16) error   { return nil }
func (callbacks) SendInvalidCommitWelcome(uint16) error { return nil }

func TestSessionAdmissionAndVideoPassthrough(t *testing.T) {
	s := NewSession(slog.Default(), "123456789", callbacks{}).(*session)
	defer s.Close()
	s.SetChannelID(987654321)
	s.AssignH264(10)
	frame := append([]byte{0, 0, 0, 1, 0x65}, bytes.Repeat([]byte{0x88}, 1000)...)
	if _, err := s.EncryptVideo(10, frame); err == nil {
		t.Fatal("accepted video before protocol negotiation")
	}
	s.OnSelectProtocolAck(0)
	if !s.Ready() {
		t.Fatal("server-selected passthrough not ready")
	}
	encrypted, err := s.EncryptVideo(10, frame)
	if err != nil || !bytes.Equal(frame, encrypted) {
		t.Fatalf("passthrough: %v", err)
	}
	out := make([]byte, len(frame))
	if _, err := s.DecryptVideo("unknown", frame, out); err == nil {
		t.Fatal("unknown participant admitted")
	}
	s.AddUser("2")
	n, err := s.DecryptVideo("2", encrypted, out)
	if err != nil || !bytes.Equal(out[:n], frame) {
		t.Fatalf("decrypt: %v", err)
	}
	s.OnDaveMLSProposals(nil)
	s.OnDaveMLSExternalSenderPackage(nil)
	s.OnDaveMLSPrepareCommitTransition(1, nil)
	s.OnDaveMLSWelcome(1, nil)
	s.Close()
	s.Close()
	if s.Ready() {
		t.Fatal("closed session ready")
	}
	if _, err := s.EncryptVideo(10, frame); err == nil {
		t.Fatal("closed session encrypted")
	}
	s.AddUser("3")
	s.AssignSsrcToCodec(1, godave.CodecOpus)
}

func TestNativeH264Encryption(t *testing.T) {
	s := NewSession(slog.Default(), "123456789", callbacks{}).(*session)
	defer s.Close()
	s.SetChannelID(987654321)
	s.OnSelectProtocolAck(1)
	key, err := ecdh.P256().GenerateKey(rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	// RFC 9420 ExternalSender: 65-byte P-256 key, BasicCredential uint64(1).
	external := append([]byte{0x40, 65}, key.PublicKey().Bytes()...)
	external = append(external, 0, 1, 8, 0, 0, 0, 0, 0, 0, 0, 1)
	s.OnDaveMLSExternalSenderPackage(external)
	// Local fixture: let libdave commit an empty proposal batch to establish an
	// epoch. Production only executes commits acknowledged by the gateway.
	commit := s.session.ProcessProposals([]byte{0, 0}, []string{"123456789"})
	if len(commit) == 0 {
		t.Fatal("native fixture commit unavailable")
	}
	s.OnDaveMLSPrepareCommitTransition(0, commit)
	if !s.Ready() {
		t.Fatal("initial native epoch unavailable")
	}
	s.AssignH264(10)
	d := libdave.NewDecryptor()
	d.TransitionToKeyRatchet(s.session.GetKeyRatchet("123456789"))
	frame := append([]byte{0, 0, 0, 1, 0x65, 0x88, 0x84}, bytes.Repeat([]byte{0x88}, 2000)...)
	encrypted, err := s.EncryptVideo(10, frame)
	if err != nil {
		t.Fatal(err)
	}
	if bytes.Equal(frame, encrypted) {
		t.Fatal("video was not encrypted")
	}
	// The authenticated frame must survive real RTP fragmentation/reassembly.
	payloader := &codecs.H264Payloader{DisableStapA: true}
	parser := &codecs.H264Packet{}
	var assembled []byte
	for _, payload := range payloader.Payload(1150, encrypted) {
		nal, err := parser.Unmarshal(payload)
		if err != nil {
			t.Fatal(err)
		}
		assembled = append(assembled, nal...)
	}
	encrypted = assembled
	out := make([]byte, d.GetMaxPlaintextByteSize(libdave.MediaTypeVideo, len(encrypted)))
	n, err := d.Decrypt(libdave.MediaTypeVideo, encrypted, out)
	if err != nil || !bytes.Equal(frame, out[:n]) {
		t.Fatalf("native H264 roundtrip: %v", err)
	}
}
