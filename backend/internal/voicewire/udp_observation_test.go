package voice

import (
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
		writer.Write(make([]byte, 12))
	}()
	if _, err := u.ReadPacket(); err == nil {
		t.Fatal("closed synthetic transport must return an error")
	}
	count, last := u.ReceiveObservation()
	if count != 2 || last <= 0 || time.Now().UnixMilli()-last > 1000 {
		t.Fatalf("ignored raw delivery was lost: count=%d last=%d", count, last)
	}
}
