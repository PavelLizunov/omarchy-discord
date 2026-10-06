package session

import (
	"bytes"
	"context"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/diamondburned/arikawa/v3/api"
	"github.com/diamondburned/arikawa/v3/discord"
	"github.com/diamondburned/arikawa/v3/utils/httputil/httpdriver"
	"github.com/diamondburned/ningen/v3"

	"github.com/mattcalayo/omarchy-discord/backend/internal/protocol"
	"github.com/mattcalayo/omarchy-discord/backend/internal/socket"
)

func uploadHarness(t *testing.T) (*Manager, *fakeClient, *[]string, func(line string) (any, *protocol.Error)) {
	t.Helper()
	m, _ := readyManager(t)
	var names []string
	f := &fakeREST{}
	m.rest = f.ops()
	m.rest.send = func(_ context.Context, _ *ningen.State, _ discord.ChannelID, d api.SendMessageData) (*discord.Message, error) {
		for _, file := range d.Files {
			names = append(names, file.Name)
			if _, err := io.Copy(io.Discard, file.Reader); err != nil {
				return nil, err
			}
		}
		if f.err != nil {
			return nil, f.err
		}
		return &discord.Message{ID: 778, Nonce: d.Nonce}, nil
	}
	m.stagedDir = filepath.Join(t.TempDir(), "staged")
	os.MkdirAll(m.stagedDir, 0o700)
	client := newFakeClient()
	client.OpenChannel("300000000000000002")
	ctx := socket.WithClient(context.Background(), client)
	return m, client, &names, func(line string) (any, *protocol.Error) { return m.Handle(ctx, req(t, line)) }
}

func writeFile(t *testing.T, path string, size int) string {
	t.Helper()
	if err := os.WriteFile(path, bytes.Repeat([]byte("a"), size), 0o600); err != nil {
		t.Fatal(err)
	}
	return path
}

func progressEvents(c *fakeClient) []protocol.UploadProgressEvent {
	var out []protocol.UploadProgressEvent
	for _, ev := range c.events() {
		if p, ok := ev.(protocol.UploadProgressEvent); ok {
			out = append(out, p)
		}
	}
	return out
}

func TestUploadStagedCleanupAndProgress(t *testing.T) {
	m, client, names, call := uploadHarness(t)
	staged := writeFile(t, filepath.Join(m.stagedDir, "shot-1.png"), 4096)
	outside := writeFile(t, filepath.Join(t.TempDir(), "keep.png"), 100)
	res, e := call(`{"v":1,"id":44,"command":"upload",` + general + `,"paths":["` + staged + `","` + outside + `"],"content":"look"}`)
	if e != nil {
		t.Fatal(e)
	}
	r := res.(protocol.SendResult)
	if r.MessageID != "778" || len(r.Nonce) != 16 {
		t.Fatalf("result %+v", r)
	}
	if strings.Join(*names, ",") != "shot-1.png,keep.png" {
		t.Fatalf("names %v", *names)
	}
	if _, err := os.Stat(staged); !os.IsNotExist(err) {
		t.Fatal("staged file not removed")
	}
	if _, err := os.Stat(outside); err != nil {
		t.Fatal("file outside the staged dir was removed")
	}
	evs := progressEvents(client)
	if len(evs) < 2 {
		t.Fatalf("progress events %+v", evs)
	}
	var finals int
	for _, ev := range evs {
		if ev.UploadID != 44 {
			t.Fatalf("upload id %d", ev.UploadID)
		}
		if ev.BytesSent == ev.BytesTotal {
			finals++
		}
	}
	if finals != 2 || evs[len(evs)-1].Filename != "keep.png" || evs[len(evs)-1].BytesTotal != 100 {
		t.Fatalf("finals %d, events %+v", finals, evs)
	}
}

func TestUploadProgressCadence(t *testing.T) {
	now := time.Unix(1_000_000, 0)
	var got [][2]int64
	pr := &progressReader{r: bytes.NewReader(make([]byte, 1000)), total: 1000, now: func() time.Time { return now },
		report: func(sent, total int64) { got = append(got, [2]int64{sent, total}) }}
	buf := make([]byte, 100)
	for i := 0; i < 5; i++ {
		pr.Read(buf)
		now = now.Add(10 * time.Millisecond)
	}
	now = now.Add(100 * time.Millisecond)
	pr.Read(buf)
	for i := 0; i < 4; i++ {
		pr.Read(buf)
	}
	if _, err := pr.Read(buf); err != io.EOF {
		t.Fatalf("want EOF, got %v", err)
	}
	want := [][2]int64{{100, 1000}, {600, 1000}, {1000, 1000}}
	if len(got) != len(want) {
		t.Fatalf("events %v", got)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("events %v want %v", got, want)
		}
	}
}

func TestUploadTooLargeAndValidation(t *testing.T) {
	m, client, names, call := uploadHarness(t)
	big := writeFile(t, filepath.Join(m.stagedDir, "big.bin"), api.UploadSizeLimit/2+1)
	big2 := writeFile(t, filepath.Join(m.stagedDir, "big2.bin"), api.UploadSizeLimit/2+1)
	if _, e := call(`{"v":1,"id":1,"command":"upload",` + general + `,"paths":["` + big + `","` + big2 + `"]}`); e == nil || e.Code != protocol.CodeUploadTooLarge {
		t.Fatalf("too large: %v", e)
	}
	if _, err := os.Stat(big); err != nil {
		t.Fatal("staged file removed on failure")
	}
	dir := t.TempDir()
	for _, paths := range []string{
		`[]`,
		`["relative.png"]`,
		`["` + dir + `"]`,
		`["` + filepath.Join(dir, "missing.png") + `"]`,
		`["/dev/null"]`,
	} {
		if _, e := call(`{"v":1,"id":1,"command":"upload",` + general + `,"paths":` + paths + `}`); e == nil || e.Code != protocol.CodeInvalidArgument {
			t.Errorf("%s: %v", paths, e)
		}
	}
	ok := writeFile(t, filepath.Join(dir, "ok.png"), 10)
	if _, e := call(`{"v":1,"id":1,"command":"upload","channel_id":"300000000000000003","paths":["` + ok + `"]}`); e == nil || e.Code != protocol.CodeChannelNotOpen {
		t.Fatalf("not open: %v", e)
	}
	if len(*names) != 0 || len(progressEvents(client)) != 0 {
		t.Fatal("REST called before validation passed")
	}
}

func TestUploadSpoilerAndFailure(t *testing.T) {
	m, _, names, call := uploadHarness(t)
	a := writeFile(t, filepath.Join(m.stagedDir, "a.png"), 10)
	b := writeFile(t, filepath.Join(m.stagedDir, "SPOILER_b.png"), 10)
	if _, e := call(`{"v":1,"id":1,"command":"upload",` + general + `,"paths":["` + a + `","` + b + `"],"spoiler":true,"reply_to":"600000000000000001"}`); e != nil {
		t.Fatal(e)
	}
	if strings.Join(*names, ",") != "SPOILER_a.png,SPOILER_b.png" {
		t.Fatalf("names %v", *names)
	}
	c := writeFile(t, filepath.Join(m.stagedDir, "c.png"), 10)
	m.rest.send = func(context.Context, *ningen.State, discord.ChannelID, api.SendMessageData) (*discord.Message, error) {
		return nil, httpErr(403)
	}
	if _, e := call(`{"v":1,"id":1,"command":"upload",` + general + `,"paths":["` + c + `"]}`); e == nil || e.Code != protocol.CodeForbidden {
		t.Fatalf("403: %v", e)
	}
	if _, err := os.Stat(c); err != nil {
		t.Fatal("staged file removed after a failed upload")
	}
}

func TestUnderStagedDir(t *testing.T) {
	for p, want := range map[string]bool{
		"/run/user/1000/omarchy-discord/staged/x.png":     true,
		"/run/user/1000/omarchy-discord/staged/sub/x.png": true,
		"/run/user/1000/omarchy-discord/staged":           false,
		"/run/user/1000/omarchy-discord/staged2/x.png":    false,
		"/run/user/1000/omarchy-discord/qr.png":           false,
		"/home/m/x.png":                                   false,
	} {
		if got := underStagedDir("/run/user/1000/omarchy-discord/staged", p); got != want {
			t.Errorf("%s: %v", p, got)
		}
	}
	if underStagedDir("", "/x") {
		t.Fatal("empty staged dir must match nothing")
	}
}

type countingTransport struct {
	mu       sync.Mutex
	attempts int
	bodies   []int64
}

func (c *countingTransport) RoundTrip(r *http.Request) (*http.Response, error) {
	n, _ := io.Copy(io.Discard, r.Body)
	r.Body.Close()
	c.mu.Lock()
	c.attempts++
	c.bodies = append(c.bodies, n)
	c.mu.Unlock()
	return &http.Response{
		StatusCode: http.StatusTooManyRequests,
		Header:     http.Header{"Content-Type": {"application/json"}},
		Body:       io.NopCloser(strings.NewReader(`{"message":"You are being rate limited.","retry_after":0.01,"global":false}`)),
		Request:    r,
	}, nil
}

func TestUploadNoRetryOn429(t *testing.T) {
	m, _, _, call := uploadHarness(t)
	m.rest = liveREST()
	tr := &countingTransport{}
	m.mu.Lock()
	m.n.Client.Client.Client = httpdriver.WrapClient(http.Client{Transport: tr})
	m.mu.Unlock()
	path := writeFile(t, filepath.Join(m.stagedDir, "one.png"), 2048)
	_, e := call(`{"v":1,"id":9,"command":"upload",` + general + `,"paths":["` + path + `"]}`)
	if e == nil || e.Code != protocol.CodeRateLimited {
		t.Fatalf("429: %v", e)
	}
	tr.mu.Lock()
	defer tr.mu.Unlock()
	if tr.attempts != 1 {
		t.Fatalf("attempts %d (bodies %v)", tr.attempts, tr.bodies)
	}
	if tr.bodies[0] < 2048 {
		t.Fatalf("first attempt read only %d bytes", tr.bodies[0])
	}
	if _, err := os.Stat(path); err != nil {
		t.Fatal("staged file removed after a failed upload")
	}
}
