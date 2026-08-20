// Package media caches Discord CDN downloads in a size-capped directory and
// hands local paths to the client. Fetch answers immediately — a hit returns
// the path, a miss starts a background download that reports completion with a
// media_ready event. Only allowlisted hosts are ever contacted, redirects
// included.
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

	"github.com/mattcalayo/omarchy-discord/backend/internal/protocol"
	"github.com/mattcalayo/omarchy-discord/backend/internal/redact"
)

// DefaultCapMB is the default LRU cap.
const DefaultCapMB = 512

// AllowedHosts are the only hosts fetched by default.
var AllowedHosts = []string{"cdn.discordapp.com", "media.discordapp.net"}

const (
	defaultConcurrency = 4
	downloadTimeout    = 60 * time.Second
	maxBodyBytes       = 100 << 20
	maxRedirects       = 10
	tmpPrefix          = ".tmp-"
	keyLen             = sha256.Size * 2
)

// sizedPrefixes are the CDN path families that honour a ?size= hint.
var sizedPrefixes = []string{"/avatars/", "/emojis/", "/icons/", "/app-icons/", "/banners/"}

// extByType maps the content types we serve to their file extension.
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

// Options configures a Cache.
type Options struct {
	Dir         string       // cache dir; created 0700 by New
	CapMB       int          // 0 → DefaultCapMB
	Emit        func(ev any) // receives protocol.MediaReadyEvent values; required
	Hosts       []string     // nil → AllowedHosts
	AllowHTTP   bool         // tests only; otherwise the scheme must be https
	Concurrency int          // 0 → 4
	Client      *http.Client // optional base; New copies it and installs its own CheckRedirect
}

// entry is one indexed cache file.
type entry struct {
	name string
	size int64
}

// Cache is a concurrent, LRU-evicted directory of downloaded media.
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

// New creates the cache directory and returns a ready cache.
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

// Dir returns the cache directory.
func (c *Cache) Dir() string { return c.dir }

// SetCapMB changes the cap; it takes effect at the next eviction pass.
func (c *Cache) SetCapMB(mb int) {
	if mb <= 0 {
		return
	}
	c.setCapBytes(int64(mb) << 20)
}

// setCapBytes sets the cap in bytes (tests need caps smaller than a megabyte).
func (c *Cache) setCapBytes(n int64) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.capBytes = n
}

// Fetch returns the cached path for rawURL, or reports a miss and downloads it
// in the background. ctx bounds the caller's request only: the download outlives
// it, since the connection may drop before the bytes arrive.
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
		go c.download(key, rawURL, sizedURL(u, rawURL, size))
	}
	return protocol.FetchMediaResult{Cached: false}, nil
}

// allowed reports whether u is on the allowlist and uses a permitted scheme.
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

// checkRedirect refuses to follow a redirect off the allowlist.
func (c *Cache) checkRedirect(req *http.Request, via []*http.Request) error {
	if len(via) >= maxRedirects {
		return errors.New("too many redirects")
	}
	if !c.allowed(req.URL) {
		return errors.New("disallowed redirect")
	}
	return nil
}

// cacheKey is the hex sha256 of the URL and its size hint.
func cacheKey(rawURL string, size int) string {
	sum := sha256.Sum256([]byte(rawURL + "\x00" + strconv.Itoa(size)))
	return hex.EncodeToString(sum[:])
}

// sizedURL appends the size hint on the CDN paths that support it; every other
// URL is fetched exactly as given.
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

// download fetches one entry and emits its media_ready, keyed by the URL the
// caller asked for rather than the sized one it resolved to.
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

// get downloads fetchURL into the cache directory and indexes it.
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

// extFor picks the file extension from the content type, falling back to the
// URL's own extension when the type is unknown.
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

// indexLocked scans the cache directory once, dropping leftover temp files.
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

// isKeyName reports whether name starts with a cache key.
func isKeyName(name string) bool {
	if len(name) < keyLen {
		return false
	}
	_, err := hex.DecodeString(name[:keyLen])
	return err == nil
}

// evictLocked deletes oldest-first until the cache fits its cap, never touching
// the entry just written.
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

// cause unwraps the transport's *url.Error so the message never echoes the URL.
func cause(err error) error {
	var ue *url.Error
	if errors.As(err, &ue) && ue.Err != nil {
		return ue.Err
	}
	return err
}
