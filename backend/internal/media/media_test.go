package media

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/mattcalayo/omarchy-discord/backend/internal/protocol"
)

// recorder collects the media_ready events a cache emits.
type recorder struct {
	ch chan protocol.MediaReadyEvent

	mu   sync.Mutex
	seen []protocol.MediaReadyEvent
}

func newRecorder() *recorder {
	return &recorder{ch: make(chan protocol.MediaReadyEvent, 64)}
}

func (r *recorder) emit(ev any) {
	e, ok := ev.(protocol.MediaReadyEvent)
	if !ok {
		panic(fmt.Sprintf("emit: unexpected event %T", ev))
	}
	r.mu.Lock()
	r.seen = append(r.seen, e)
	r.mu.Unlock()
	r.ch <- e
}

func (r *recorder) count() int {
	r.mu.Lock()
	defer r.mu.Unlock()
	return len(r.seen)
}

// wait returns the next event, failing the test if none arrives.
func (r *recorder) wait(t *testing.T) protocol.MediaReadyEvent {
	t.Helper()
	select {
	case ev := <-r.ch:
		return ev
	case <-time.After(5 * time.Second):
		t.Fatal("timed out waiting for media_ready")
		return protocol.MediaReadyEvent{}
	}
}

// none asserts that no further event arrives shortly.
func (r *recorder) none(t *testing.T) {
	t.Helper()
	select {
	case ev := <-r.ch:
		t.Fatalf("unexpected media_ready: %+v", ev)
	case <-time.After(150 * time.Millisecond):
	}
}

// newCache builds a cache pointed at srv (nil for allowlist-only tests).
func newCache(t *testing.T, srv *httptest.Server, o Options) (*Cache, *recorder) {
	t.Helper()
	rec := newRecorder()
	o.Emit = rec.emit
	if o.Dir == "" {
		o.Dir = t.TempDir()
	}
	if srv != nil {
		u, err := url.Parse(srv.URL)
		if err != nil {
			t.Fatalf("parse server url: %v", err)
		}
		if o.Hosts == nil {
			o.Hosts = []string{u.Host}
		}
		o.AllowHTTP = true
	}
	c, err := New(o)
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	return c, rec
}

// dirNames lists the cache directory.
func dirNames(t *testing.T, dir string) []string {
	t.Helper()
	ents, err := os.ReadDir(dir)
	if err != nil {
		t.Fatalf("read dir: %v", err)
	}
	names := make([]string, 0, len(ents))
	for _, e := range ents {
		names = append(names, e.Name())
	}
	return names
}

func TestFetchRejectsDisallowedURLs(t *testing.T) {
	c, rec := newCache(t, nil, Options{})
	ctx := context.Background()

	for _, tc := range []struct {
		name, url, code string
	}{
		{"foreign host", "https://evil.example/a.png", protocol.CodeMediaError},
		{"http on allowed host", "http://cdn.discordapp.com/a.png", protocol.CodeMediaError},
		{"empty", "", protocol.CodeInvalidArgument},
		{"unparseable", "https://cdn.discordapp.com/%zz", protocol.CodeInvalidArgument},
	} {
		res, perr := c.Fetch(ctx, tc.url, 0)
		if perr == nil {
			t.Fatalf("%s: want error, got %+v", tc.name, res)
		}
		if perr.Code != tc.code {
			t.Fatalf("%s: code = %q, want %q", tc.name, perr.Code, tc.code)
		}
		if res.Cached || res.Path != "" {
			t.Fatalf("%s: want zero result, got %+v", tc.name, res)
		}
	}
	rec.none(t)
	if n := rec.count(); n != 0 {
		t.Fatalf("emitted %d events, want 0", n)
	}
}

func TestFetchMissThenHit(t *testing.T) {
	var hits atomic.Int64
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		hits.Add(1)
		w.Header().Set("Content-Type", "image/png")
		w.Write([]byte("pngbytes"))
	}))
	defer srv.Close()

	c, rec := newCache(t, srv, Options{})
	ctx := context.Background()
	u := srv.URL + "/attachments/1/2/a.png"

	res, perr := c.Fetch(ctx, u, 0)
	if perr != nil {
		t.Fatalf("Fetch: %v", perr)
	}
	if res.Cached {
		t.Fatal("first fetch reported a hit")
	}

	ev := rec.wait(t)
	if !ev.OK || ev.Error != "" {
		t.Fatalf("media_ready = %+v, want ok", ev)
	}
	if ev.URL != u {
		t.Fatalf("event url = %q, want %q", ev.URL, u)
	}
	if filepath.Ext(ev.Path) != ".png" {
		t.Fatalf("path = %q, want a .png", ev.Path)
	}
	if _, err := os.Stat(ev.Path); err != nil {
		t.Fatalf("stat downloaded file: %v", err)
	}

	res2, perr := c.Fetch(ctx, u, 0)
	if perr != nil {
		t.Fatalf("second Fetch: %v", perr)
	}
	if !res2.Cached || res2.Path != ev.Path {
		t.Fatalf("second fetch = %+v, want hit on %q", res2, ev.Path)
	}
	rec.none(t)
	if got := hits.Load(); got != 1 {
		t.Fatalf("server hits = %d, want 1", got)
	}
}

func TestFetchCoalescesConcurrentMisses(t *testing.T) {
	var hits atomic.Int64
	release := make(chan struct{})
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		hits.Add(1)
		<-release
		w.Header().Set("Content-Type", "image/png")
		w.Write([]byte("pngbytes"))
	}))
	defer srv.Close()

	c, rec := newCache(t, srv, Options{})
	u := srv.URL + "/attachments/1/2/a.png"

	var wg sync.WaitGroup
	for i := 0; i < 5; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			res, perr := c.Fetch(context.Background(), u, 0)
			if perr != nil {
				t.Errorf("Fetch: %v", perr)
				return
			}
			if res.Cached {
				t.Errorf("fetch reported a hit while the download was blocked")
			}
		}()
	}
	wg.Wait()
	close(release)

	ev := rec.wait(t)
	if !ev.OK {
		t.Fatalf("media_ready = %+v, want ok", ev)
	}
	rec.none(t)
	if n := rec.count(); n != 1 {
		t.Fatalf("emitted %d events, want 1", n)
	}
	if got := hits.Load(); got != 1 {
		t.Fatalf("server hits = %d, want 1", got)
	}
}

func TestExtensionFromContentTypeAndPath(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/x/photo":
			w.Header().Set("Content-Type", "image/jpeg; charset=binary")
		default:
			w.Header().Set("Content-Type", "application/octet-stream")
		}
		w.Write([]byte("body"))
	}))
	defer srv.Close()

	c, rec := newCache(t, srv, Options{})
	for _, tc := range []struct{ path, want string }{
		{"/x/photo", ".jpg"},
		{"/x/file.webp", ".webp"},
		{"/x/noext", ""},
	} {
		if _, perr := c.Fetch(context.Background(), srv.URL+tc.path, 0); perr != nil {
			t.Fatalf("%s: Fetch: %v", tc.path, perr)
		}
		ev := rec.wait(t)
		if !ev.OK {
			t.Fatalf("%s: media_ready = %+v, want ok", tc.path, ev)
		}
		if got := filepath.Ext(ev.Path); got != tc.want {
			t.Fatalf("%s: ext = %q, want %q", tc.path, got, tc.want)
		}
	}
}

func TestSizeHint(t *testing.T) {
	var mu sync.Mutex
	seen := map[string][]string{}
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		mu.Lock()
		seen[r.URL.Path] = append(seen[r.URL.Path], r.URL.RawQuery)
		mu.Unlock()
		w.Header().Set("Content-Type", "image/png")
		w.Write([]byte("pngbytes"))
	}))
	defer srv.Close()

	queries := func(path string) []string {
		mu.Lock()
		defer mu.Unlock()
		return append([]string(nil), seen[path]...)
	}

	c, rec := newCache(t, srv, Options{})
	ctx := context.Background()

	avatar := srv.URL + "/avatars/1/a.png"
	if _, perr := c.Fetch(ctx, avatar, 64); perr != nil {
		t.Fatalf("Fetch avatar: %v", perr)
	}
	ev64 := rec.wait(t)
	if !ev64.OK {
		t.Fatalf("avatar media_ready = %+v, want ok", ev64)
	}
	if ev64.URL != avatar {
		t.Fatalf("event url = %q, want the original %q", ev64.URL, avatar)
	}
	if got := queries("/avatars/1/a.png"); len(got) != 1 || got[0] != "size=64" {
		t.Fatalf("avatar queries = %q, want [size=64]", got)
	}

	attach := srv.URL + "/attachments/1/2/a.png"
	if _, perr := c.Fetch(ctx, attach, 64); perr != nil {
		t.Fatalf("Fetch attachment: %v", perr)
	}
	if ev := rec.wait(t); !ev.OK {
		t.Fatalf("attachment media_ready = %+v, want ok", ev)
	}
	if got := queries("/attachments/1/2/a.png"); len(got) != 1 || got[0] != "" {
		t.Fatalf("attachment queries = %q, want [\"\"]", got)
	}

	withQuery := srv.URL + "/avatars/1/b.png?foo=1"
	if _, perr := c.Fetch(ctx, withQuery, 64); perr != nil {
		t.Fatalf("Fetch avatar with query: %v", perr)
	}
	if ev := rec.wait(t); !ev.OK {
		t.Fatalf("queried avatar media_ready = %+v, want ok", ev)
	}
	if got := queries("/avatars/1/b.png"); len(got) != 1 || got[0] != "foo=1" {
		t.Fatalf("queried avatar queries = %q, want [foo=1]", got)
	}

	// The size is part of the cache key: 64 is a hit, 128 is a fresh miss.
	if res, perr := c.Fetch(ctx, avatar, 64); perr != nil || !res.Cached || res.Path != ev64.Path {
		t.Fatalf("re-fetch size 64 = %+v (%v), want hit on %q", res, perr, ev64.Path)
	}
	if res, perr := c.Fetch(ctx, avatar, 128); perr != nil || res.Cached {
		t.Fatalf("fetch size 128 = %+v (%v), want a miss", res, perr)
	}
	ev128 := rec.wait(t)
	if !ev128.OK || ev128.Path == ev64.Path {
		t.Fatalf("size 128 media_ready = %+v, want a distinct cache entry", ev128)
	}
	if got := queries("/avatars/1/a.png"); len(got) != 2 || got[1] != "size=128" {
		t.Fatalf("avatar queries = %q, want a second size=128 request", got)
	}
}

func TestRedirectOffAllowlistFails(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Redirect(w, r, "http://127.0.0.1:1/x", http.StatusFound)
	}))
	defer srv.Close()

	c, rec := newCache(t, srv, Options{})
	if _, perr := c.Fetch(context.Background(), srv.URL+"/attachments/1/2/a.png", 0); perr != nil {
		t.Fatalf("Fetch: %v", perr)
	}
	ev := rec.wait(t)
	if ev.OK {
		t.Fatalf("media_ready = %+v, want a failure", ev)
	}
	if ev.Error == "" {
		t.Fatal("failure event carries no error text")
	}
	if names := dirNames(t, c.Dir()); len(names) != 0 {
		t.Fatalf("cache dir holds %q, want it empty", names)
	}
}

func TestHTTPErrorFails(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Error(w, "nope", http.StatusNotFound)
	}))
	defer srv.Close()

	c, rec := newCache(t, srv, Options{})
	if _, perr := c.Fetch(context.Background(), srv.URL+"/attachments/1/2/a.png", 0); perr != nil {
		t.Fatalf("Fetch: %v", perr)
	}
	ev := rec.wait(t)
	if ev.OK {
		t.Fatalf("media_ready = %+v, want a failure", ev)
	}
	if !strings.Contains(ev.Error, "404") {
		t.Fatalf("error = %q, want it to mention 404", ev.Error)
	}
	if names := dirNames(t, c.Dir()); len(names) != 0 {
		t.Fatalf("cache dir holds %q, want it empty", names)
	}
}

func TestLRUEviction(t *testing.T) {
	body := make([]byte, 1024)
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "image/png")
		w.Write(body)
	}))
	defer srv.Close()

	c, rec := newCache(t, srv, Options{})
	c.setCapBytes(2560) // 2.5 KiB: three 1 KiB files do not fit.
	ctx := context.Background()

	fetch := func(path string) protocol.MediaReadyEvent {
		t.Helper()
		if _, perr := c.Fetch(ctx, srv.URL+path, 0); perr != nil {
			t.Fatalf("Fetch %s: %v", path, perr)
		}
		ev := rec.wait(t)
		if !ev.OK {
			t.Fatalf("media_ready for %s = %+v, want ok", path, ev)
		}
		return ev
	}
	age := func(path string, d time.Duration) {
		t.Helper()
		at := time.Now().Add(-d)
		if err := os.Chtimes(path, at, at); err != nil {
			t.Fatalf("chtimes: %v", err)
		}
	}

	first := fetch("/attachments/1/1/a.png")
	age(first.Path, 2*time.Hour)
	second := fetch("/attachments/1/2/b.png")
	age(second.Path, time.Hour)
	third := fetch("/attachments/1/3/c.png")

	if _, err := os.Stat(first.Path); !os.IsNotExist(err) {
		t.Fatalf("oldest file still present (stat err %v)", err)
	}
	for _, ev := range []protocol.MediaReadyEvent{second, third} {
		if _, err := os.Stat(ev.Path); err != nil {
			t.Fatalf("newer file %q evicted: %v", ev.Path, err)
		}
	}

	res, perr := c.Fetch(ctx, srv.URL+"/attachments/1/1/a.png", 0)
	if perr != nil {
		t.Fatalf("re-fetch: %v", perr)
	}
	if res.Cached {
		t.Fatal("evicted url still reports a hit")
	}
}

func TestStartupIndexAdoptsFilesAndDropsTemps(t *testing.T) {
	dir := t.TempDir()
	u := "https://cdn.discordapp.com/attachments/1/2/a.png"
	sum := sha256.Sum256([]byte(u + "\x00" + strconv.Itoa(0)))
	name := hex.EncodeToString(sum[:]) + ".png"
	if err := os.WriteFile(filepath.Join(dir, name), []byte("pngbytes"), 0o600); err != nil {
		t.Fatalf("seed cache file: %v", err)
	}
	if err := os.WriteFile(filepath.Join(dir, ".tmp-abc"), []byte("junk"), 0o600); err != nil {
		t.Fatalf("seed temp file: %v", err)
	}

	c, rec := newCache(t, nil, Options{Dir: dir})
	res, perr := c.Fetch(context.Background(), u, 0)
	if perr != nil {
		t.Fatalf("Fetch: %v", perr)
	}
	if !res.Cached || res.Path != filepath.Join(c.Dir(), name) {
		t.Fatalf("fetch = %+v, want a hit on the seeded file", res)
	}
	if _, err := os.Stat(filepath.Join(dir, ".tmp-abc")); !os.IsNotExist(err) {
		t.Fatalf("stray temp file survived (stat err %v)", err)
	}
	rec.none(t)
}

func TestConcurrencyIsBounded(t *testing.T) {
	var (
		inflight atomic.Int64
		mu       sync.Mutex
		peakSeen int64
	)
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		cur := inflight.Add(1)
		mu.Lock()
		if cur > peakSeen {
			peakSeen = cur
		}
		mu.Unlock()
		time.Sleep(5 * time.Millisecond)
		inflight.Add(-1)
		w.Header().Set("Content-Type", "image/png")
		w.Write([]byte("pngbytes"))
	}))
	defer srv.Close()

	const n = 20
	c, rec := newCache(t, srv, Options{Concurrency: 2})
	for i := 0; i < n; i++ {
		u := fmt.Sprintf("%s/attachments/1/%d/a.png", srv.URL, i)
		if _, perr := c.Fetch(context.Background(), u, 0); perr != nil {
			t.Fatalf("Fetch %d: %v", i, perr)
		}
	}
	for i := 0; i < n; i++ {
		if ev := rec.wait(t); !ev.OK {
			t.Fatalf("media_ready %d = %+v, want ok", i, ev)
		}
	}
	mu.Lock()
	peak := peakSeen
	mu.Unlock()
	if peak > 2 {
		t.Fatalf("peak in-flight requests = %d, want at most 2", peak)
	}
}
