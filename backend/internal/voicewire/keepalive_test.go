package voice

import (
	"context"
	"encoding/binary"
	"log/slog"
	"net"
	"testing"
	"time"

	"github.com/disgoorg/godave"
)

func TestKeepAliveWithoutMicrophoneAudio(t *testing.T) {
	client, server := net.Pipe()
	defer client.Close()
	defer server.Close()
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	u := &udpConnImpl{config: defaultUDPConnConfig()}
	done := make(chan struct{})
	go func() { defer close(done); u.keepAlive(ctx, client, time.Millisecond) }()
	for sequence := uint32(0); sequence < 3; sequence++ {
		_ = server.SetReadDeadline(time.Now().Add(time.Second))
		var packet [8]byte
		n, err := server.Read(packet[:])
		if err != nil || n != 8 {
			t.Fatalf("ping: n=%d err=%v", n, err)
		}
		if binary.BigEndian.Uint32(packet[:4]) != 0x1337CAFE || binary.BigEndian.Uint32(packet[4:]) != sequence {
			t.Fatalf("invalid transport ping: %x", packet)
		}
	}
	cancel()
	select {
	case <-done:
	case <-time.After(time.Second):
		t.Fatal("keepalive survived cancellation")
	}
}

func TestCloseStopsKeepAlive(t *testing.T) {
	client, server := net.Pipe()
	defer server.Close()
	ctx, cancel := context.WithCancel(context.Background())
	u := &udpConnImpl{conn: client, keepAliveCancel: cancel, config: udpConnConfig{Logger: slog.Default()}}
	done := make(chan struct{})
	go func() { defer close(done); u.keepAlive(ctx, client, time.Hour) }()
	if err := u.Close(); err != nil {
		t.Fatal(err)
	}
	select {
	case <-done:
	case <-time.After(time.Second):
		t.Fatal("Close leaked keepalive")
	}
}

func TestUDPOpenOwnsKeepAliveAndFiltersReplies(t *testing.T) {
	server, err := net.ListenUDP("udp", &net.UDPAddr{IP: net.IPv4(127, 0, 0, 1)})
	if err != nil {
		t.Fatal(err)
	}
	defer server.Close()
	discovery := make(chan error, 1)
	go func() {
		var packet [74]byte
		_ = server.SetReadDeadline(time.Now().Add(time.Second))
		n, peer, err := server.ReadFromUDP(packet[:])
		if err == nil && n == 74 {
			binary.BigEndian.PutUint16(packet[:2], 2)
			copy(packet[8:72], "127.0.0.1")
			binary.BigEndian.PutUint16(packet[72:], uint16(peer.Port))
			_, err = server.WriteToUDP(packet[:], peer)
		}
		discovery <- err
	}()
	u := NewUDPConn(godave.NewNoopSession(slog.Default(), "123", nil), nil).(*udpConnImpl)
	defer u.Close()
	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()
	if _, _, err := u.Open(ctx, "127.0.0.1", server.LocalAddr().(*net.UDPAddr).Port, 42); err != nil {
		t.Fatal(err)
	}
	if err := <-discovery; err != nil {
		t.Fatal(err)
	}
	if u.keepAliveCancel == nil {
		t.Fatal("Open did not start transport keepalive")
	}
	_ = server.SetReadDeadline(time.Now().Add(6 * time.Second))
	var ping [8]byte
	n, peer, err := server.ReadFromUDP(ping[:])
	if err != nil || n != 8 || binary.BigEndian.Uint32(ping[:4]) != 0x1337CAFE {
		t.Fatalf("no ping after discovery: %x %v", ping, err)
	}
	binary.BigEndian.PutUint32(ping[:4], 0x1337F00D)
	if _, err = server.WriteToUDP(ping[:], peer); err != nil {
		t.Fatal(err)
	}
	_ = u.SetReadDeadline(time.Now().Add(30 * time.Millisecond))
	if p, err := u.ReadPacket(); p != nil || err == nil {
		t.Fatal("ping reply was decoded as media")
	}
}
