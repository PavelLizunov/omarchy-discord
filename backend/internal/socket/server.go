package socket

import (
	"bufio"
	"context"
	"errors"
	"fmt"
	"net"
	"os"
	"path/filepath"
	"sync"

	"github.com/mattcalayo/omarchy-discord/backend/internal/panics"
	"github.com/mattcalayo/omarchy-discord/backend/internal/protocol"
	"github.com/mattcalayo/omarchy-discord/backend/internal/redact"
)

type Backend interface {
	Snapshot() []any
	Handle(ctx context.Context, req *protocol.Request) (result any, err *protocol.Error)
}

type ClientCloser interface {
	ClientClosed(c Client)
}

type Client interface {
	OpenChannel(id string)
	CloseChannel(id string) bool
	HasOpen(id string) bool
	SubscribeMembers(id string)
	UnsubscribeMembers(id string) bool
	HasMemberSub(id string) bool
	Push(ev any)
}

type clientKey struct{}

func WithClient(ctx context.Context, c Client) context.Context {
	return context.WithValue(ctx, clientKey{}, c)
}

func ClientFromContext(ctx context.Context) Client {
	c, _ := ctx.Value(clientKey{}).(Client)
	return c
}

type Routed struct {
	ChannelID string
	All       bool
	Open      []string
	Members   []string
	Event     any
}

func (r Routed) matches(open, members map[string]struct{}) bool {
	if r.All {
		return true
	}
	if _, ok := open[r.ChannelID]; ok && r.ChannelID != "" {
		return true
	}
	for _, id := range r.Open {
		if _, ok := open[id]; ok {
			return true
		}
	}
	for _, id := range r.Members {
		if _, ok := members[id]; ok {
			return true
		}
	}
	return false
}

func DefaultPath() string {
	dir := os.Getenv("XDG_RUNTIME_DIR")
	if dir == "" {
		dir = fmt.Sprintf("/run/user/%d", os.Getuid())
	}
	return filepath.Join(dir, "omarchy-discord", "backend.sock")
}

const maxLine = 1 << 20

const queueDepth = 512
const maxInflight = 64

type Server struct {
	path    string
	backend Backend

	mu    sync.Mutex
	conns map[*conn]struct{}
	ln    net.Listener
}

func New(path string, b Backend) *Server {
	return &Server{path: path, backend: b, conns: map[*conn]struct{}{}}
}

func (s *Server) Listen() error {
	if err := os.MkdirAll(filepath.Dir(s.path), 0o700); err != nil {
		return fmt.Errorf("socket: create runtime dir: %w", err)
	}
	if err := os.Remove(s.path); err != nil && !errors.Is(err, os.ErrNotExist) {
		return fmt.Errorf("socket: unlink stale socket: %w", err)
	}
	ln, err := net.Listen("unix", s.path)
	if err != nil {
		return fmt.Errorf("socket: bind: %w", err)
	}
	if err := os.Chmod(s.path, 0o600); err != nil {
		ln.Close()
		return fmt.Errorf("socket: chmod: %w", err)
	}
	s.ln = ln
	return nil
}

func (s *Server) Path() string { return s.path }

func (s *Server) Serve(ctx context.Context) error {
	if s.ln == nil {
		return errors.New("socket: Serve before Listen")
	}
	go func() {
		<-ctx.Done()
		s.ln.Close()
	}()
	var wg sync.WaitGroup
	for {
		c, err := s.ln.Accept()
		if err != nil {
			if ctx.Err() != nil {
				break
			}
			redact.Logf("socket: accept: %v", err)
			continue
		}
		wg.Add(1)
		go func() {
			defer wg.Done()
			s.handle(ctx, c)
		}()
	}
	s.mu.Lock()
	for c := range s.conns {
		c.close()
	}
	s.mu.Unlock()
	wg.Wait()
	os.Remove(s.path)
	return nil
}

func (s *Server) Broadcast(ev any) {
	r, routed := ev.(Routed)
	if routed {
		ev = r.Event
	}
	line := protocol.MustEncode(0, ev)
	s.mu.Lock()
	defer s.mu.Unlock()
	for c := range s.conns {
		if routed && !r.matches(c.open, c.members) {
			continue
		}
		c.send(line)
	}
}

type conn struct {
	nc   net.Conn
	out  chan []byte
	done chan struct{}
	once sync.Once
	srv  *Server

	open    map[string]struct{}
	members map[string]struct{}

	lanesMu sync.Mutex
	lanes   map[string]*ticket
}

type ticket struct {
	id   string
	prev *ticket
	done chan struct{}
}

type channelParams struct {
	ChannelID string `json:"channel_id"`
}

func (c *conn) enqueue(req *protocol.Request) *ticket {
	if req.Command != "open_channel" && req.Command != "close_channel" {
		return nil
	}
	var p channelParams
	if req.Params(&p) != nil || p.ChannelID == "" {
		return nil
	}
	c.lanesMu.Lock()
	defer c.lanesMu.Unlock()
	t := &ticket{id: p.ChannelID, prev: c.lanes[p.ChannelID], done: make(chan struct{})}
	c.lanes[p.ChannelID] = t
	return t
}

func (t *ticket) wait(c *conn) {
	if t == nil || t.prev == nil {
		return
	}
	select {
	case <-t.prev.done:
	case <-c.done:
	}
}

func (t *ticket) release(c *conn) {
	if t == nil {
		return
	}
	close(t.done)
	c.lanesMu.Lock()
	defer c.lanesMu.Unlock()
	if c.lanes[t.id] == t {
		delete(c.lanes, t.id)
	}
}

func (c *conn) OpenChannel(id string) {
	c.srv.mu.Lock()
	defer c.srv.mu.Unlock()
	c.open[id] = struct{}{}
}

func (c *conn) CloseChannel(id string) bool {
	c.srv.mu.Lock()
	defer c.srv.mu.Unlock()
	_, ok := c.open[id]
	delete(c.open, id)
	return ok
}

func (c *conn) HasOpen(id string) bool {
	c.srv.mu.Lock()
	defer c.srv.mu.Unlock()
	_, ok := c.open[id]
	return ok
}

func (c *conn) SubscribeMembers(id string) {
	c.srv.mu.Lock()
	defer c.srv.mu.Unlock()
	c.members[id] = struct{}{}
}

func (c *conn) UnsubscribeMembers(id string) bool {
	c.srv.mu.Lock()
	defer c.srv.mu.Unlock()
	_, ok := c.members[id]
	delete(c.members, id)
	return ok
}

func (c *conn) HasMemberSub(id string) bool {
	c.srv.mu.Lock()
	defer c.srv.mu.Unlock()
	_, ok := c.members[id]
	return ok
}

func (c *conn) Push(ev any) { c.send(protocol.MustEncode(0, ev)) }

func (s *Server) handle(ctx context.Context, nc net.Conn) {
	c := &conn{nc: nc, out: make(chan []byte, queueDepth), done: make(chan struct{}), srv: s, open: map[string]struct{}{}, members: map[string]struct{}{}, lanes: map[string]*ticket{}}
	defer c.close()
	go c.writer()
	ctx, cancel := context.WithCancel(WithClient(ctx, c))
	var requests sync.WaitGroup
	inflight := make(chan struct{}, maxInflight)
	defer cancel()

	s.mu.Lock()
	s.conns[c] = struct{}{}
	func() {
		defer panics.Recover("socket: snapshot")
		for _, ev := range s.backend.Snapshot() {
			c.send(protocol.MustEncode(0, ev))
		}
	}()
	s.mu.Unlock()
	defer func() {
		c.close()
		cancel()
		requests.Wait()
		s.mu.Lock()
		delete(s.conns, c)
		s.mu.Unlock()
		if cc, ok := s.backend.(ClientCloser); ok {
			defer panics.Recover("socket: client closed")
			cc.ClientClosed(c)
		}
	}()

	sc := bufio.NewScanner(nc)
	sc.Buffer(make([]byte, 64*1024), maxLine)
	for sc.Scan() {
		line := sc.Bytes()
		if len(line) == 0 {
			continue
		}
		req, perr := protocol.DecodeRequest(line)
		if perr != nil {
			var id int64
			if req != nil {
				id = req.ID
			}
			c.send(protocol.MustEncode(id, protocol.ErrResponse(id, perr)))
			continue
		}
		if req.Command == "hello" || req.Command == "ping" {
			c.dispatch(ctx, req, nil)
			continue
		}
		select {
		case inflight <- struct{}{}:
		default:
			c.send(protocol.MustEncode(req.ID, protocol.ErrResponse(req.ID,
				protocol.Errorf(protocol.CodeRateLimited, "too many in-flight requests"))))
			continue
		}
		t := c.enqueue(req)
		requests.Add(1)
		go func() {
			defer requests.Done()
			defer func() { <-inflight }()
			c.dispatch(ctx, req, t)
		}()
	}
	if err := sc.Err(); err != nil && ctx.Err() == nil {
		redact.Logf("socket: read: %v", err)
	}
}

func (c *conn) dispatch(ctx context.Context, req *protocol.Request, t *ticket) {
	if t != nil {
		t.wait(c)
		defer t.release(c)
	}
	select {
	case <-c.done:
		return
	case <-ctx.Done():
		return
	default:
	}
	result, perr := c.answer(ctx, req)
	if perr != nil {
		c.send(protocol.MustEncode(req.ID, protocol.ErrResponse(req.ID, perr)))
		return
	}
	c.send(protocol.MustEncode(req.ID, protocol.OKResponse(req.ID, result)))
}

func (c *conn) answer(ctx context.Context, req *protocol.Request) (result any, perr *protocol.Error) {
	defer func() {
		if r := recover(); r != nil {
			panics.Log("socket: command "+req.Command, r)
			result, perr = nil, protocol.Errorf(protocol.CodeInternalError, "internal error handling %s", req.Command)
		}
	}()
	switch req.Command {
	case "hello":
		return protocol.Hello(), nil
	case "ping":
		return protocol.PingResult{Pong: true}, nil
	}
	return c.srv.backend.Handle(ctx, req)
}

func (c *conn) send(line []byte) {
	select {
	case c.out <- line:
	case <-c.done:
	default:
		redact.Logf("socket: client too slow, dropping connection")
		c.close()
	}
}

func (c *conn) writer() {
	for {
		select {
		case line := <-c.out:
			if _, err := c.nc.Write(line); err != nil {
				c.close()
				return
			}
		case <-c.done:
			return
		}
	}
}

func (c *conn) close() {
	c.once.Do(func() {
		close(c.done)
		c.nc.Close()
	})
}
