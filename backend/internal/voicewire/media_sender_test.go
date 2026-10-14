package voice

import (
	"context"
	"log/slog"
	"testing"
	"time"

	"github.com/disgoorg/godave"
)

type waitingMediaGateway struct {
	Gateway
	opened, closed int
}

func (g *waitingMediaGateway) Open(context.Context, State) error { g.opened++; return nil }
func (g *waitingMediaGateway) Close()                            { g.closed++ }
func TestMediaSenderCancellationClosesOnlyStream(t *testing.T) {
	ds := &fixtureVideoCipher{Session: godave.NewNoopSession(slog.Default(), "123", nil)}
	g := &waitingMediaGateway{}
	u := &mediaUDP{}
	removed := 0
	c := &connImpl{dave: ds, gateway: g, udp: u, removeConnFunc: func() { removed++ }}
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Millisecond)
	defer cancel()
	err := c.OpenMediaSender(ctx, State{GuildID: 1, ChannelID: 2, UserID: 123, SessionID: "fixture", Token: "fixture", Endpoint: "fixture.discord.media"})
	if err == nil || !c.videoSend || !c.mediaOnly || g.opened != 1 || g.closed != 1 || u.closed != 1 || removed != 1 {
		t.Fatal("sender cancellation leaked or used voice", err, g.opened, g.closed, u.closed, removed)
	}
}
