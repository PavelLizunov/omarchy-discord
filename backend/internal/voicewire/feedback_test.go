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

func TestTransportFeedbackWireAndWrap(t *testing.T) {
	for _, sequence := range []uint16{100, 65534} {
		f := transportFeedback{}
		at := time.Unix(1700000000, 0)
		if f.add(sequence, at, 42) != nil {
			t.Fatal("unbounded per-packet feedback")
		}
		f.add(sequence+1, at.Add(time.Millisecond), 42)
		p := f.add(sequence+3, at.Add(60*time.Millisecond), 42)
		if len(p) == 0 || p[0] != 0x8f || p[1] != 205 || int(binary.BigEndian.Uint16(p[2:4])+1)*4 != len(p) {
			t.Fatal("invalid RTCP", p)
		}
		if binary.BigEndian.Uint32(p[4:8]) != 42 || binary.BigEndian.Uint32(p[8:12]) != 0 || binary.BigEndian.Uint16(p[12:14]) != sequence || binary.BigEndian.Uint16(p[14:16]) != 4 {
			t.Fatal("feedback identity", p)
		}
		chunk := binary.BigEndian.Uint16(p[20:22])
		statuses := []uint16{(chunk >> 12) & 3, (chunk >> 10) & 3, (chunk >> 8) & 3, (chunk >> 6) & 3}
		if statuses[0] != 1 || statuses[1] != 1 || statuses[2] != 0 || statuses[3] != 1 {
			t.Fatal("missing packet not represented", statuses)
		}
		if p[23] != 4 || p[24] != 236 {
			t.Fatal("arrival delta units", p)
		}
	}
	f := transportFeedback{}
	at := time.Unix(1700000000, 0)
	f.add(10, at, 42)
	f.add(11, at.Add(60*time.Millisecond), 42)
	if p := f.add(10, at.Add(61*time.Millisecond), 42); p != nil {
		t.Fatal("late duplicate reopened interval")
	}
	p := f.add(13, at.Add(120*time.Millisecond), 42)
	if binary.BigEndian.Uint16(p[12:14]) != 12 || binary.BigEndian.Uint16(p[14:16]) != 2 {
		t.Fatal("report gap lost", p)
	}
	if binary.BigEndian.Uint16(p[20:22])>>10&3 != 1 {
		t.Fatal("gap status invalid", p)
	}
	if _, ok := transportSequence(0xbede, []byte{0x51, 1}); ok {
		t.Fatal("truncated extension")
	}
	if _, ok := transportSequence(0x1000, []byte{0x51, 1, 2}); ok {
		t.Fatal("wrong profile")
	}
	seq, ok := transportSequence(0xbede, []byte{0, 0x20, 4, 0x51, 1, 2, 0})
	if !ok || seq != 258 {
		t.Fatal(seq, ok)
	}
}
func TestReceiverReportSequenceLossAndWrap(t *testing.T) {
	r := receiverReport{}
	at := time.Unix(1700000000, 0)
	p := &Packet{SSRC: 42, Sequence: 65534, Timestamp: 9000}
	if r.add(p, at, 99) != nil {
		t.Fatal("report too frequent")
	}
	p.Sequence = 0
	p.Timestamp = 99000
	wire := r.add(p, at.Add(time.Second), 99)
	if len(wire) != 32 || wire[0] != 0x81 || wire[1] != 201 || binary.BigEndian.Uint32(wire[16:20]) != 65536 {
		t.Fatal("bad receiver report", wire)
	}
	if loss := binary.BigEndian.Uint32(wire[12:16]); loss != uint32(85<<24|1) {
		t.Fatal("loss/wrap", loss)
	}
	if binary.BigEndian.Uint32(wire[8:12]) != 42 || binary.BigEndian.Uint32(wire[4:8]) != 99 {
		t.Fatal("wrong SSRC")
	}
}
func TestPaddingProbeAcknowledgedButNotDelivered(t *testing.T) {
	for _, mode := range AllEncryptionModes {
		t.Run(string(mode), func(t *testing.T) {
			reader, writer := net.Pipe()
			defer reader.Close()
			defer writer.Close()
			reader.SetDeadline(time.Now().Add(3 * time.Second))
			writer.SetDeadline(time.Now().Add(3 * time.Second))
			key := bytes.Repeat([]byte{42}, 32)
			ds := godave.NewNoopSession(slog.Default(), "123", nil)
			u := NewUDPConn(ds, func(uint32) snowflake.ID { return 123 }).(*udpConnImpl)
			u.conn = reader
			u.ssrc = 99
			if err := u.SetSecretKey(mode, key); err != nil {
				t.Fatal(err)
			}
			enc, _ := NewAEADEncrypter(mode, key)
			feedback := make(chan []byte, 1)
			errs := make(chan error, 1)
			go func() {
				for i := 0; i < 2; i++ {
					h := [16]byte{0xb0, H264PayloadType}
					binary.BigEndian.PutUint32(h[8:12], 42)
					h[12] = 0xbe
					h[13] = 0xde
					h[15] = 1
					p := append([]byte{0x51, 0, byte(i), 0}, bytes.Repeat([]byte{255}, 255)...)
					_, e := writer.Write(enc.encryptHeader(h[:], p))
					if e != nil {
						errs <- e
						return
					}
					if i == 0 {
						time.Sleep(60 * time.Millisecond)
					}
				}
				buf := make([]byte, 512)
				n, e := writer.Read(buf)
				if e != nil {
					errs <- e
					return
				}
				feedback <- append([]byte(nil), buf[:n]...)
				h := [12]byte{0x80, H264PayloadType}
				binary.BigEndian.PutUint32(h[8:], 42)
				_, e = writer.Write(enc.Encrypt(h, []byte{0x65, 1, 2}))
				errs <- e
			}()
			for i := 0; i < 2; i++ {
				p, e := u.ReadPacket()
				if e != nil || len(p.Opus) != 0 {
					t.Fatal("padding became media", p, e)
				}
			}
			p, e := u.ReadPacket()
			if e != nil {
				t.Fatal(e)
			}
			if !bytes.Equal(p.Opus, []byte{0x65, 1, 2}) {
				t.Fatal("padding delivered as media", p)
			}
			wire := <-feedback
			decrypt, _ := NewAEADEncrypter(mode, key)
			body, e := decrypt.Decrypt(8, wire)
			if e != nil {
				t.Fatal("feedback not authenticated", e)
			}
			if wire[0] != 0x8f || wire[1] != 205 || binary.BigEndian.Uint16(body[6:8]) != 2 {
				t.Fatal("wrong feedback packet", wire[:8], body)
			}
			if e := <-errs; e != nil {
				t.Fatal(e)
			}
			stages := u.CameraReceiveStages()
			if stages["padding_only"] != 2 || stages["feedback_sent"] != 1 || stages["delivered"] != 1 {
				t.Fatal(stages)
			}
		})
	}
}

func TestTransportFeedbackOverflowStillReports(t *testing.T) {
	f := transportFeedback{}
	at := time.Unix(1700000000, 0)
	reports := 0
	for i := 0; i < 2000; i++ {
		p := f.add(uint16(i), at.Add(time.Duration(i)*90*time.Microsecond), 42)
		if p != nil {
			reports++
			if len(p) > 1600 {
				t.Fatal("oversize report")
			}
		}
	}
	if reports < 3 {
		t.Fatal("feedback starved", reports)
	}
}
func TestReceiverReportTimestampWrap(t *testing.T) {
	r := receiverReport{}
	at := time.Unix(1700000000, 0)
	p := &Packet{Sequence: 1, Timestamp: 0xfffffff0}
	r.add(p, at, 99)
	p.Sequence = 2
	p.Timestamp = 0x4a
	r.add(p, at.Add(time.Millisecond), 99)
	if r.jitter != 0 {
		t.Fatal("timestamp wrap corrupted jitter", r.jitter)
	}
}
