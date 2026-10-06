package media

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/mattcalayo/omarchy-discord/backend/internal/panics"
	"github.com/mattcalayo/omarchy-discord/backend/internal/protocol"
	"github.com/mattcalayo/omarchy-discord/backend/internal/redact"
)

const DefaultCapMB = 512

var AllowedHosts = []string{"cdn.discordapp.com", "media.discordapp.net"}

const (
	defaultConcurrency = 4
	downloadTimeout    = 60 * time.Second
	maxBodyBytes       = 100 << 20
	maxRedirects       = 10
	tmpPrefix          = ".tmp-"
	keyLen             = sha256.Size * 2
)

var sizedPrefixes = []string{"/avatars/", "/emojis/", "/icons/", "/app-icons/", "/banners/"}

var extByType = map[string]string{
	"image/png":  ".png",
	"image/jpeg": ".jpg",
	"image/gif":  ".gif",
	"image/webp": ".webp",
	"image/avif": ".avif",
	"video/mp4":  ".mp4",
	"video/webm": ".webm",
	"audio/mpeg": ".mp3",
	"audio/ogg":  ".ogg",
}

type Options struct {
	Dir         string
	CapMB       int
	Emit        func(ev any)
	Hosts       []string
	AllowHTTP   bool
	Concurrency int
	Client      *http.Client
}

type entry struct {
	name string
	size int64
}

type Cache struct {
	dir       string
	emit      func(ev any)
	hosts     []string
	allowHTTP bool
	client    *http.Client
	sem       chan struct{}

	mu       sync.Mutex
	capBytes int64
	indexed  bool
	index    map[string]entry
	total    int64
	inflight map[string]struct{}
}

func New(o Options) (*Cache, error) {
	if o.Dir == "" {
		return nil, errors.New("media: Dir is required")
	}
	if o.Emit == nil {
		return nil, errors.New("media: Emit is required")
	}
	dir, err := filepath.Abs(o.Dir)
	if err != nil {
		return nil, fmt.Errorf("media: %w", err)
	}
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return nil, fmt.Errorf("media: %w", err)
	}
	hosts := o.Hosts
	if hosts == nil {
		hosts = AllowedHosts
	}
	lowered := make([]string, len(hosts))
	for i, h := range hosts {
		lowered[i] = strings.ToLower(h)
	}
	capMB := o.CapMB
	if capMB <= 0 {
		capMB = DefaultCapMB
	}
	conc := o.Concurrency
	if conc <= 0 {
		conc = defaultConcurrency
	}
	c := &Cache{
		dir:       dir,
		emit:      o.Emit,
		hosts:     lowered,
		allowHTTP: o.AllowHTTP,
		sem:       make(chan struct{}, conc),
		capBytes:  int64(capMB) << 20,
		index:     map[string]entry{},
		inflight:  map[string]struct{}{},
	}
	client := &http.Client{}
	if o.Client != nil {
		*client = *o.Client
	}
	client.CheckRedirect = c.checkRedirect
	c.client = client
	return c, nil
}

func (c *Cache) Dir() string { return c.dir }

func (c *Cache) SetCapMB(mb int) {
	if mb <= 0 {
		return
	}
	c.setCapBytes(int64(mb) << 20)
}

func (c *Cache) setCapBytes(n int64) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.capBytes = n
}

func (c *Cache) Fetch(ctx context.Context, rawURL string, size int) (protocol.FetchMediaResult, *protocol.Error) {
	var zero protocol.FetchMediaResult
	if rawURL == "" {
		return zero, protocol.Errorf(protocol.CodeInvalidArgument, "url is required")
	}
	u, err := url.Parse(rawURL)
	if err != nil {
		return zero, protocol.Errorf(protocol.CodeInvalidArgument, "bad url: %v", err)
	}
	if !c.allowed(u) {
		return zero, protocol.Errorf(protocol.CodeMediaError, "disallowed host")
	}
	key := cacheKey(rawURL, size)

	c.mu.Lock()
	defer c.mu.Unlock()
	c.indexLocked()
	if e, ok := c.index[key]; ok {
		path := filepath.Join(c.dir, e.name)
		if st, err := os.Stat(path); err == nil && st.Mode().IsRegular() {
			now := time.Now()
			_ = os.Chtimes(path, now, now)
			return protocol.FetchMediaResult{Cached: true, Path: path}, nil
		}
		delete(c.index, key)
		c.total -= e.size
	}
	if _, ok := c.inflight[key]; !ok {
		c.inflight[key] = struct{}{}
		url := sizedURL(u, rawURL, size)
		panics.Go("media: download", func() { c.download(key, rawURL, url) })
	}
	return protocol.FetchMediaResult{Cached: false}, nil
}

func (c *Cache) allowed(u *url.URL) bool {
	if u.Scheme != "https" && !(c.allowHTTP && u.Scheme == "http") {
		return false
	}
	host := strings.ToLower(u.Host)
	if host == "" {
		return false
	}
	for _, h := range c.hosts {
		if h == host {
			return true
		}
	}
	return false
}

func (c *Cache) checkRedirect(req *http.Request, via []*http.Request) error {
	if len(via) >= maxRedirects {
		return errors.New("too many redirects")
	}
	if !c.allowed(req.URL) {
		return errors.New("disallowed redirect")
	}
	return nil
}

func cacheKey(rawURL string, size int) string {
	sum := sha256.Sum256([]byte(rawURL + "\x00" + strconv.Itoa(size)))
	return hex.EncodeToString(sum[:])
}

func sizedURL(u *url.URL, rawURL string, size int) string {
	if size <= 0 || u.RawQuery != "" {
		return rawURL
	}
	for _, p := range sizedPrefixes {
		if strings.HasPrefix(u.Path, p) {
			sized := *u
			sized.RawQuery = "size=" + strconv.Itoa(size)
			return sized.String()
		}
	}
	return rawURL
}

func (c *Cache) download(key, rawURL, fetchURL string) {
	c.sem <- struct{}{}
	defer func() { <-c.sem }()

	ctx, cancel := context.WithTimeout(context.Background(), downloadTimeout)
	defer cancel()
	path, err := c.get(ctx, key, fetchURL)

	c.mu.Lock()
	delete(c.inflight, key)
	if err == nil {
		c.evictLocked(key)
	}
	c.mu.Unlock()

	if err != nil {
		redact.Logf("media: download failed: %v", err)
		c.emit(protocol.NewMediaReady(rawURL, false, "", err.Error()))
		return
	}
	c.emit(protocol.NewMediaReady(rawURL, true, path, ""))
}

func (c *Cache) get(ctx context.Context, key, fetchURL string) (string, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, fetchURL, nil)
	if err != nil {
		return "", fmt.Errorf("request: %v", err)
	}
	resp, err := c.client.Do(req)
	if err != nil {
		return "", fmt.Errorf("get: %v", cause(err))
	}
	defer resp.Body.Close()
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return "", fmt.Errorf("http %d", resp.StatusCode)
	}

	tmp, err := os.CreateTemp(c.dir, tmpPrefix+"*")
	if err != nil {
		return "", fmt.Errorf("write: %v", err)
	}
	n, err := io.Copy(tmp, io.LimitReader(resp.Body, maxBodyBytes+1))
	if err == nil && n > maxBodyBytes {
		err = errors.New("body too large")
	}
	if err == nil {
		err = tmp.Chmod(0o600)
	}
	if cerr := tmp.Close(); err == nil {
		err = cerr
	}
	if err != nil {
		os.Remove(tmp.Name())
		return "", fmt.Errorf("write: %v", err)
	}

	name := key + extFor(resp.Header.Get("Content-Type"), fetchURL)
	path := filepath.Join(c.dir, name)
	if err := os.Rename(tmp.Name(), path); err != nil {
		os.Remove(tmp.Name())
		return "", fmt.Errorf("write: %v", err)
	}

	c.mu.Lock()
	defer c.mu.Unlock()
	c.indexLocked()
	if old, ok := c.index[key]; ok {
		c.total -= old.size
		if old.name != name {
			os.Remove(filepath.Join(c.dir, old.name))
		}
	}
	c.index[key] = entry{name: name, size: n}
	c.total += n
	return path, nil
}

func extFor(contentType, rawURL string) string {
	ct := strings.ToLower(strings.TrimSpace(contentType))
	if i := strings.IndexByte(ct, ';'); i >= 0 {
		ct = strings.TrimSpace(ct[:i])
	}
	if ext, ok := extByType[ct]; ok {
		return ext
	}
	path := rawURL
	if u, err := url.Parse(rawURL); err == nil {
		path = u.Path
	}
	ext := strings.ToLower(filepath.Ext(path))
	if len(ext) < 2 || len(ext) > 8 {
		return ""
	}
	for _, r := range ext {
		if (r < 'a' || r > 'z') && (r < '0' || r > '9') && r != '.' {
			return ""
		}
	}
	return ext
}

func (c *Cache) indexLocked() {
	if c.indexed {
		return
	}
	c.indexed = true
	ents, err := os.ReadDir(c.dir)
	if err != nil {
		redact.Logf("media: cannot read cache dir: %v", err)
		return
	}
	for _, de := range ents {
		name := de.Name()
		if strings.HasPrefix(name, tmpPrefix) {
			os.Remove(filepath.Join(c.dir, name))
			continue
		}
		if !de.Type().IsRegular() || !isKeyName(name) {
			continue
		}
		info, err := de.Info()
		if err != nil {
			continue
		}
		key := name[:keyLen]
		if _, dup := c.index[key]; dup {
			continue
		}
		c.index[key] = entry{name: name, size: info.Size()}
		c.total += info.Size()
	}
}

func isKeyName(name string) bool {
	if len(name) < keyLen {
		return false
	}
	_, err := hex.DecodeString(name[:keyLen])
	return err == nil
}

func (c *Cache) evictLocked(keep string) {
	for c.total > c.capBytes {
		var (
			victim string
			ve     entry
			oldest time.Time
		)
		for k, e := range c.index {
			if k == keep {
				continue
			}
			st, err := os.Stat(filepath.Join(c.dir, e.name))
			if err != nil {
				delete(c.index, k)
				c.total -= e.size
				continue
			}
			if victim == "" || st.ModTime().Before(oldest) {
				victim, ve, oldest = k, e, st.ModTime()
			}
		}
		if victim == "" {
			return
		}
		os.Remove(filepath.Join(c.dir, ve.name))
		delete(c.index, victim)
		c.total -= ve.size
	}
}

func cause(err error) error {
	var ue *url.Error
	if errors.As(err, &ue) && ue.Err != nil {
		return ue.Err
	}
	return err
}
