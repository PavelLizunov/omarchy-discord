// Package socket serves the line-delimited JSON protocol over a private unix
// socket. Each connection has exactly one writer goroutine; responses and
// broadcast events are queued to it, never written directly.
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

	"github.com/mattcalayo/omarchy-discord/backend/internal/protocol"
	"github.com/mattcalayo/omarchy-discord/backend/internal/redact"
)

// Backend answers commands and provides the connect-time snapshot.
type Backend interface {
	// Snapshot returns the events pushed to a client on connect, in order
	// (state_changed, then guilds_synced when ready).
	Snapshot() []any
	// Handle answers one decoded request. hello and ping never reach it.
	Handle(ctx context.Context, req *protocol.Request) (result any, err *protocol.Error)
}

// DefaultPath computes $XDG_RUNTIME_DIR/omarchy-discord/backend.sock.
func DefaultPath() string {
	dir := os.Getenv("XDG_RUNTIME_DIR")
	if dir == "" {
		dir = fmt.Sprintf("/run/user/%d", os.Getuid())
	}
	return filepath.Join(dir, "omarchy-discord", "backend.sock")
}

// maxLine bounds a single request line (1 MiB).
const maxLine = 1 << 20

// queueDepth is the per-connection outbound queue; a client that falls this far
// behind is disconnected rather than allowed to block the fan-out.
const queueDepth = 512

// Server owns the listener and the connection set.
type Server struct {
	path    string
	backend Backend

	mu    sync.Mutex
	conns map[*conn]struct{}
	ln    net.Listener
}

// New creates a server for the given socket path.
func New(path string, b Backend) *Server {
	return &Server{path: path, backend: b, conns: map[*conn]struct{}{}}
}

// Listen prepares the socket: parent dir 0700, stale file unlinked, bind,
// chmod 0600.
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

// Path returns the socket path.
func (s *Server) Path() string { return s.path }

// Serve accepts connections until ctx is cancelled, then closes every
// connection and removes the socket file.
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

// Broadcast queues an event to every connected client, preserving order
// relative to other broadcasts and to connect-time snapshots.
func (s *Server) Broadcast(ev any) {
	line := protocol.MustEncode(0, ev)
	s.mu.Lock()
	defer s.mu.Unlock()
	for c := range s.conns {
		c.send(line)
	}
}

type conn struct {
	nc    net.Conn
	out   chan []byte
	done  chan struct{}
	once  sync.Once
	srv   *Server
	label string
}

func (s *Server) handle(ctx context.Context, nc net.Conn) {
	c := &conn{nc: nc, out: make(chan []byte, queueDepth), done: make(chan struct{}), srv: s}
	defer c.close()
	go c.writer()

	// Register and enqueue the snapshot while holding the same lock Broadcast
	// takes, so no event can interleave the snapshot's lines or reach this
	// client before them. (Events already queued upstream may still arrive after
	// the snapshot; clients resolve that with the generation stamp.)
	s.mu.Lock()
	s.conns[c] = struct{}{}
	for _, ev := range s.backend.Snapshot() {
		c.send(protocol.MustEncode(0, ev))
	}
	s.mu.Unlock()
	defer func() {
		s.mu.Lock()
		delete(s.conns, c)
		s.mu.Unlock()
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
			// Echo the id whenever the line parsed; only an unparseable line
			// gets id 0.
			var id int64
			if req != nil {
				id = req.ID
			}
			c.send(protocol.MustEncode(id, protocol.ErrResponse(id, perr)))
			continue
		}
		// Each request is answered on its own goroutine so a slow command never
		// blocks ping; the writer serializes the output.
		go c.dispatch(ctx, req)
	}
	if err := sc.Err(); err != nil && ctx.Err() == nil {
		redact.Logf("socket: read: %v", err)
	}
}

func (c *conn) dispatch(ctx context.Context, req *protocol.Request) {
	var result any
	var perr *protocol.Error
	switch req.Command {
	case "hello":
		result = protocol.Hello()
	case "ping":
		result = protocol.PingResult{Pong: true}
	default:
		result, perr = c.srv.backend.Handle(ctx, req)
	}
	if perr != nil {
		c.send(protocol.MustEncode(req.ID, protocol.ErrResponse(req.ID, perr)))
		return
	}
	c.send(protocol.MustEncode(req.ID, protocol.OKResponse(req.ID, result)))
}

// send queues a line; a client whose queue is full is dropped.
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
