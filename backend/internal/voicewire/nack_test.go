package voice

import (
	"bytes"
	"encoding/binary"
	"net"
	"testing"
	"time"
)

func TestNACKAuthenticatedWireAndBound(t *testing.T) {
	for _, mode := range AllEncryptionModes {
		t.Run(string(mode), func(t *testing.T) {
			r, w := net.Pipe()
			defer r.Close()
			defer w.Close()
			r.SetDeadline(time.Now().Add(time.Second))
			w.SetDeadline(time.Now().Add(time.Second))
			key := bytes.Repeat([]byte{42}, 32)
			a, _ := NewAEADEncrypter(mode, key)
			u := &udpConnImpl{conn: r, encrypter: a, ssrc: 99}
			done := make(chan error, 1)
			go func() {
				seqs := make([]uint16, 20)
				for i := range seqs {
					seqs[i] = uint16(i + 10)
				}
				done <- u.SendNACK(42, seqs)
			}()
			b := make([]byte, 256)
			n, e := w.Read(b)
			if e != nil {
				t.Fatal(e)
			}
			decrypt, _ := NewAEADEncrypter(mode, key)
			body, e := decrypt.Decrypt(8, b[:n])
			if e != nil {
				t.Fatal(e)
			}
			if b[0] != 0x81 || b[1] != 205 || len(body) != 4+16*4 || binary.BigEndian.Uint32(body[:4]) != 42 || binary.BigEndian.Uint16(body[4:6]) != 10 {
				t.Fatal("invalid NACK", b[:8], body)
			}
			if e := <-done; e != nil {
				t.Fatal(e)
			}
		})
	}
}
