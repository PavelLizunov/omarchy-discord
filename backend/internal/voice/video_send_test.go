package voice

import (
	"context"
	"errors"
	"log/slog"
	"testing"

	"github.com/disgoorg/godave"
	"github.com/disgoorg/snowflake/v2"
	dvoice "github.com/mattcalayo/omarchy-discord/backend/internal/voicewire"
	davesession "github.com/thomas-vilte/dave-go/session"
)

// Transport guard must reject the actual production audio-only DAVE adapter.
func TestVideoWriterRejectsAudioOnlyAdapter(t *testing.T) {
	ds := davesession.New("123", nil)
	defer ds.Close()
	u := dvoice.NewUDPConn(ds, func(uint32) snowflake.ID { return 123 })
	factory := u.(interface {
		NewVideoWriter(uint32) (*dvoice.VideoWriter, error)
	})
	if _, err := factory.NewVideoWriter(42); err == nil {
		t.Fatal("Opus adapter silently accepted H264")
	}
	conn := dvoice.NewConn(1, 123, func(context.Context, snowflake.ID, *snowflake.ID, bool, bool) error { return nil }, func() {}, dvoice.WithConnDaveSessionCreateFunc(func(_ *slog.Logger, _ godave.UserID, _ godave.Callbacks) godave.Session { return ds }))
	if err := conn.(interface {
		OpenMediaSender(context.Context, dvoice.State) error
	}).OpenMediaSender(context.Background(), dvoice.State{}); err == nil {
		t.Fatal("audio adapter opened video network")
	}
	conn.Close(context.Background())
}

type unavailableVideoSession struct {
	godave.Session
	ready bool
}

func (s *unavailableVideoSession) Ready() bool { return s.ready }
func (s *unavailableVideoSession) EncryptH264(uint32, []byte) ([]byte, error) {
	return nil, errors.New("fixture encryption rejection")
}

func TestVideoWriterCancellationAndEncryptionFailure(t *testing.T) {
	ds := &unavailableVideoSession{Session: godave.NewNoopSession(slog.Default(), "123", nil)}
	u := dvoice.NewUDPConn(ds, func(uint32) snowflake.ID { return 123 })
	writer, err := u.(interface {
		NewVideoWriter(uint32) (*dvoice.VideoWriter, error)
	}).NewVideoWriter(42)
	if err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if !errors.Is(writer.Send(ctx, []byte{0, 0, 0, 1, 0x65, 1}, 9000), context.Canceled) {
		t.Fatal("cancel ignored")
	}
	if err := writer.Send(context.Background(), []byte{0, 0, 0, 1, 0x65, 1}, 9000); err == nil {
		t.Fatal("unready epoch allowed")
	}
}
