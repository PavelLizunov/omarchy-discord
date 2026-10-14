package voice

import (
	"encoding/binary"
	"log/slog"
	"net"
	"testing"
	"time"

	"github.com/disgoorg/godave"
)

func TestCameraReceiveStageRejections(t *testing.T) {
	for _, tc := range []struct {
		name   string
		header byte
		body   []byte
		stage  string
	}{
		{"wrong_version", 0x40, nil, "filtered"},
		{"short_csrc", 0x81, nil, "short_csrc"},
		{"short_extension", 0x90, nil, "short_extension"},
		{"short_ciphertext", 0x80, nil, "short_ciphertext"},
		{"invalid_extension", 0x90, []byte{0xbe, 0xde, 0, 1}, "invalid_extension"},
		{"empty", 0x80, nil, "empty"},
		{"padding_only_255", 0xa0, nil, "padding_only"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			reader, writer := net.Pipe()
			defer reader.Close()
			reader.SetReadDeadline(time.Now().Add(time.Second))
			u := NewUDPConn(godave.NewNoopSession(slog.Default(), "123", nil), nil).(*udpConnImpl)
			u.conn = reader
			key := make([]byte, 32)
			u.SetSecretKey(EncryptionModeAEADAES256GCMRTPSize, key)
			enc, _ := NewAEADEncrypter(EncryptionModeAEADAES256GCMRTPSize, key)
			var h [12]byte
			h[0] = tc.header
			h[1] = H264PayloadType
			binary.BigEndian.PutUint32(h[8:], 42)
			packet := append(h[:], tc.body...)
			if tc.stage == "empty" {
				packet = enc.Encrypt(h, nil)
			}
			if tc.stage == "padding_only" {
				padding := make([]byte, 255)
				padding[254] = 255
				packet = enc.Encrypt(h, padding)
			}
			if tc.stage == "invalid_extension" {
				packet = enc.encryptHeader(append(h[:], tc.body...), []byte{1})
			}
			go func() { defer writer.Close(); writer.Write(packet) }()
			_, _ = u.ReadPacket()
			s := u.CameraReceiveStages()
			if s["raw"] != 1 || s[tc.stage] != 1 || s["delivered"] != 0 {
				t.Fatalf("stage not distinguished: %v", s)
			}
			if tc.stage == "padding_only" && (s["nonempty_decryption"] != 1 || s["padding_length_255"] != 1) {
				t.Fatal(s)
			}
		})
	}
}
