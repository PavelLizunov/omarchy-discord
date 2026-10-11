package voice

import (
	"encoding/binary"
	"net"
	"testing"
	"time"
)

func TestReceiveObservationIncludesIgnoredDatagrams(t *testing.T) {
	reader, writer := net.Pipe()
	defer reader.Close()
	u := &udpConnImpl{conn: reader, receiveBuffer: make([]byte, 64)}
	if count, last := u.ReceiveObservation(); count != 0 || last != 0 {
		t.Fatal("unobserved transport advertised as receiving")
	}
	go func() {
		defer writer.Close()
		writer.Write([]byte{1})
		for i := uint32(0); i < 40; i++ {
			p := make([]byte, 12)
			p[0] = 0x80
			p[1] = 101
			binary.BigEndian.PutUint32(p[8:], i)
			writer.Write(p)
		}
	}()
	if _, err := u.ReadPacket(); err == nil {
		t.Fatal("closed synthetic transport must return an error")
	}
	count, last := u.ReceiveObservation()
	if count != 41 || last <= 0 || time.Now().UnixMilli()-last > 1000 {
		t.Fatalf("ignored raw delivery was lost: count=%d last=%d", count, last)
	}
	headers := u.PacketHeaders()
	if len(headers) != 32 || headers["101:0"] != 1 {
		t.Fatalf("header bound or ignored payload visibility failed: %v", headers)
	}
	headers["101:0"] = 999
	if u.PacketHeaders()["101:0"] != 1 {
		t.Fatal("diagnostic snapshot shares owner state")
	}
}
