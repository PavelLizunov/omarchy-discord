package session

import (
	"context"
	"io"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/diamondburned/arikawa/v3/discord"
	"github.com/diamondburned/arikawa/v3/utils/sendpart"

	"github.com/mattcalayo/omarchy-discord/backend/internal/protocol"
	"github.com/mattcalayo/omarchy-discord/backend/internal/redact"
	"github.com/mattcalayo/omarchy-discord/backend/internal/socket"
)

const (
	progressInterval = 100 * time.Millisecond
	maxUploadFiles   = 10
)

type progressReader struct {
	r        io.Reader
	total    int64
	sent     int64
	last     time.Time
	now      func() time.Time
	report   func(sent, total int64)
	finished bool
	mu       sync.Mutex
}

func (p *progressReader) Read(b []byte) (int, error) {
	n, err := p.r.Read(b)
	p.mu.Lock()
	defer p.mu.Unlock()
	p.sent += int64(n)
	now := p.now()
	done := err == io.EOF || p.sent >= p.total
	if done && !p.finished {
		p.finished = true
		p.report(p.total, p.total)
	} else if !done && (p.last.IsZero() || now.Sub(p.last) >= progressInterval) {
		p.last = now
		p.report(p.sent, p.total)
	}
	return n, err
}

type stagedFile struct {
	path string
	name string
	size int64
}

func validateUploadPath(p string) (stagedFile, *protocol.Error) {
	if !filepath.IsAbs(p) {
		return stagedFile{}, protocol.Errorf(protocol.CodeInvalidArgument, "upload path must be absolute")
	}
	st, err := os.Stat(p)
	if err != nil {
		return stagedFile{}, protocol.Errorf(protocol.CodeInvalidArgument, "upload path is not readable: %v", err)
	}
	if !st.Mode().IsRegular() {
		return stagedFile{}, protocol.Errorf(protocol.CodeInvalidArgument, "upload path is not a regular file")
	}
	f, err := os.Open(p)
	if err != nil {
		return stagedFile{}, protocol.Errorf(protocol.CodeInvalidArgument, "upload path is not readable: %v", err)
	}
	f.Close()
	return stagedFile{path: p, name: filepath.Base(p), size: st.Size()}, nil
}

func underStagedDir(stagedDir, p string) bool {
	if stagedDir == "" {
		return false
	}
	rel, err := filepath.Rel(stagedDir, p)
	return err == nil && rel != "." && !strings.HasPrefix(rel, "..")
}

func (m *Manager) upload(ctx context.Context, req *protocol.Request) (any, *protocol.Error) {
	var p protocol.UploadParams
	if e := req.Params(&p); e != nil {
		return nil, e
	}
	sf, e := parseSnowflake(p.ChannelID, "channel_id")
	if e != nil {
		return nil, e
	}
	if len(p.Paths) == 0 {
		return nil, protocol.Errorf(protocol.CodeInvalidArgument, "paths must not be empty")
	}
	if len(p.Paths) > maxUploadFiles {
		return nil, protocol.Errorf(protocol.CodeInvalidArgument, "at most %d files per message", maxUploadFiles)
	}
	client, e := requireOpen(ctx, p.ChannelID)
	if e != nil {
		return nil, e
	}
	n, e := m.liveSession()
	if e != nil {
		return nil, e
	}
	if e := validateContent(n, p.Content, true); e != nil {
		return nil, e
	}
	files := make([]stagedFile, 0, len(p.Paths))
	var total int64
	for _, path := range p.Paths {
		f, e := validateUploadPath(path)
		if e != nil {
			return nil, e
		}
		files = append(files, f)
		total += f.size
	}
	chID := discord.ChannelID(sf)
	off := n.Offline()
	var guildID discord.GuildID
	if ch, err := off.Cabinet.Channel(chID); err == nil {
		guildID = ch.GuildID
	}
	if limit := int64(off.DetermineUploadSize(guildID)); total > limit {
		return nil, protocol.Errorf(protocol.CodeUploadTooLarge, "%d bytes exceeds the %d byte upload limit", total, limit)
	}

	data, e := sendData(p.Content, newNonce(), p.ReplyTo, nil)
	if e != nil {
		return nil, e
	}
	handles := make([]*os.File, 0, len(files))
	defer func() {
		for _, h := range handles {
			h.Close()
		}
	}()
	for _, f := range files {
		h, err := os.Open(f.path)
		if err != nil {
			return nil, protocol.Errorf(protocol.CodeInvalidArgument, "upload path is not readable: %v", err)
		}
		handles = append(handles, h)
		name := f.name
		if p.Spoiler && !strings.HasPrefix(name, "SPOILER_") {
			name = "SPOILER_" + name
		}
		data.Files = append(data.Files, sendpart.File{Name: name, Reader: m.progress(client, req.ID, name, h, f.size)})
	}
	msg, err := m.rest.send(ctx, n, chID, data)
	if err != nil {
		return nil, m.restError(n, err, false)
	}
	for _, f := range files {
		if underStagedDir(m.stagedDir, f.path) {
			if err := os.Remove(f.path); err != nil {
				redact.Logf("session: remove staged upload: %v", err)
			}
		}
	}
	return protocol.SendResult{MessageID: msg.ID.String(), Nonce: data.Nonce}, nil
}

func (m *Manager) progress(c socket.Client, uploadID int64, name string, r io.Reader, size int64) io.Reader {
	return &progressReader{r: r, total: size, now: m.now, report: func(sent, total int64) {
		c.Push(protocol.NewUploadProgress(uploadID, name, sent, total))
	}}
}
