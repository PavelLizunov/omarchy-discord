package session

import (
	"context"
	"net/http"
	"net/http/httptest"
	"net/url"
	"testing"
	"time"

	"github.com/mattcalayo/omarchy-discord/backend/internal/media"
	"github.com/mattcalayo/omarchy-discord/backend/internal/protocol"
)

func TestFetchMediaAndSetConfig(t *testing.T) {
	m := New(&fakeKeyring{})
	call := func(line string) (any, *protocol.Error) { return m.Handle(context.Background(), req(t, line)) }
	if _, e := call(`{"v":1,"id":1,"command":"fetch_media","url":"https://cdn.discordapp.com/a.png"}`); e == nil || e.Code != protocol.CodeMediaError {
		t.Fatalf("unconfigured: %v", e)
	}
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "image/png")
		w.Write([]byte("png"))
	}))
	defer srv.Close()
	u, _ := url.Parse(srv.URL)
	events := make(chan any, 8)
	cache, err := media.New(media.Options{Dir: t.TempDir(), Emit: func(ev any) { events <- ev }, Hosts: []string{u.Host}, AllowHTTP: true})
	if err != nil {
		t.Fatal(err)
	}
	m.Configure(t.TempDir(), cache)

	if _, e := call(`{"v":1,"id":1,"command":"fetch_media","url":"https://example.com/a.png"}`); e == nil || e.Code != protocol.CodeMediaError {
		t.Fatalf("foreign host: %v", e)
	}
	if _, e := call(`{"v":1,"id":1,"command":"fetch_media","url":""}`); e == nil || e.Code != protocol.CodeInvalidArgument {
		t.Fatalf("empty url: %v", e)
	}
	res, e := call(`{"v":1,"id":1,"command":"fetch_media","url":"` + srv.URL + `/a.png","size":64}`)
	if e != nil || res.(protocol.FetchMediaResult).Cached {
		t.Fatalf("miss: %v %+v", e, res)
	}
	select {
	case ev := <-events:
		if mr := ev.(protocol.MediaReadyEvent); !mr.OK || mr.URL != srv.URL+"/a.png" {
			t.Fatalf("%+v", mr)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("no media_ready")
	}
	res, e = call(`{"v":1,"id":1,"command":"fetch_media","url":"` + srv.URL + `/a.png","size":64}`)
	if e != nil || !res.(protocol.FetchMediaResult).Cached || res.(protocol.FetchMediaResult).Path == "" {
		t.Fatalf("hit: %v %+v", e, res)
	}

	if _, e := call(`{"v":1,"id":2,"command":"set_config","media_cache_mb":0}`); e == nil || e.Code != protocol.CodeInvalidArgument {
		t.Fatalf("bad cap: %v", e)
	}
	if _, e := call(`{"v":1,"id":2,"command":"set_config","media_cache_mb":128}`); e != nil {
		t.Fatal(e)
	}
	if _, e := call(`{"v":1,"id":2,"command":"set_config"}`); e != nil {
		t.Fatal(e)
	}
}
