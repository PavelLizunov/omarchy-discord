package voice

import (
	"context"
	"log/slog"
	"testing"

	"github.com/disgoorg/godave"
	"github.com/disgoorg/snowflake/v2"
)

type mediaGateway struct {
	Gateway
	closed int
}

func (g *mediaGateway) Close() { g.closed++ }

type mediaUDP struct {
	UDPConn
	closed int
}

func (u *mediaUDP) Close() error { u.closed++; return nil }
func TestMediaCloseAndLossNeverUpdateVoiceState(t *testing.T) {
	calls := 0
	removed := 0
	g := &mediaGateway{}
	u := &mediaUDP{}
	c := &connImpl{mediaOnly: true, gateway: g, udp: u, dave: godave.NewNoopSession(slog.Default(), "123", nil), removeConnFunc: func() { removed++ }, voiceStateUpdateFunc: func(context.Context, snowflake.ID, *snowflake.ID, bool, bool) error { calls++; return nil }}
	c.handleGatewayClose(g, nil)
	c.Close(context.Background())
	if calls != 0 || removed != 1 || g.closed != 1 || u.closed != 2 {
		t.Fatal("media modified voice lifecycle", calls, removed, g.closed, u.closed)
	}
	if mediaGroup(State{ChannelID: 10}) != 10 || mediaGroup(State{ChannelID: 10, MediaGroupID: 20}) != 20 {
		t.Fatal("DAVE group isolation")
	}
	if err := c.OpenMedia(context.Background(), State{}); err == nil {
		t.Fatal("incomplete stream admitted")
	}
}
