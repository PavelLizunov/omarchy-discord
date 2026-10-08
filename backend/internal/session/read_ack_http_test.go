package session

import (
	"context"
	"io"
	"net/http"
	"strings"
	"testing"

	"github.com/diamondburned/arikawa/v3/utils/httputil/httpdriver"
)

type ackTransport func(*http.Request) (*http.Response, error)

func (f ackTransport) RoundTrip(r *http.Request) (*http.Response, error) { return f(r) }

func TestAckUsesDiscordRESTBeforePublishingRead(t *testing.T) {
	m, n := readyManager(t)
	fillCache(m, n, chGeneral, guildOmar, 0, 5)
	drain(m)
	status := http.StatusForbidden
	calls := 0
	n.Client.Client.Client = httpdriver.WrapClient(http.Client{Transport: ackTransport(func(r *http.Request) (*http.Response, error) {
		calls++
		if r.Method != "POST" || !strings.HasSuffix(r.URL.Path, "/channels/300000000000000002/messages/600000000000000004/ack") {
			t.Fatalf("unexpected REST request %s %s", r.Method, r.URL.Path)
		}
		body, _ := io.ReadAll(r.Body)
		if strings.TrimSpace(string(body)) != "{\"token\":null}" {
			t.Fatalf("unexpected ack body %q", body)
		}
		return &http.Response{StatusCode: status, Header: http.Header{"Content-Type": []string{"application/json"}}, Body: io.NopCloser(strings.NewReader(`{"token":null}`)), Request: r}, nil
	})})
	m.rest = liveREST()
	before := n.ReadState.ReadState(chGeneral).LastMessageID
	call := func() error {
		_, e := m.Handle(context.Background(), req(t, `{"v":1,"id":1,"command":"ack","channel_id":"300000000000000002","message_id":"600000000000000004"}`))
		if e != nil {
			return e
		}
		return nil
	}
	if e := call(); e == nil {
		t.Fatal("HTTP403 reported success")
	}
	if got := n.ReadState.ReadState(chGeneral).LastMessageID; got != before {
		t.Fatalf("failed REST changed read boundary: %s", got)
	}
	status = http.StatusOK
	if e := call(); e != nil {
		t.Fatal(e)
	}
	if got := n.ReadState.ReadState(chGeneral).LastMessageID; got != msgBase+4 {
		t.Fatalf("REST success did not advance boundary: %s", got)
	}
	if calls != 2 {
		t.Fatalf("unbounded/duplicate REST calls %d", calls)
	}
}
