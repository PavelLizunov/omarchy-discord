package session

import (
	"context"

	"github.com/mattcalayo/omarchy-discord/backend/internal/protocol"
)

func (m *Manager) fetchMedia(ctx context.Context, req *protocol.Request) (any, *protocol.Error) {
	var p protocol.FetchMediaParams
	if e := req.Params(&p); e != nil {
		return nil, e
	}
	if m.media == nil {
		return nil, protocol.Errorf(protocol.CodeMediaError, "media cache unavailable")
	}
	res, e := m.media.Fetch(ctx, p.URL, p.Size)
	if e != nil {
		return nil, e
	}
	return res, nil
}

func (m *Manager) setConfig(req *protocol.Request) (any, *protocol.Error) {
	var p protocol.SetConfigParams
	if e := req.Params(&p); e != nil {
		return nil, e
	}
	if p.MediaCacheMB != nil {
		if *p.MediaCacheMB <= 0 {
			return nil, protocol.Errorf(protocol.CodeInvalidArgument, "media_cache_mb must be positive")
		}
		if m.media != nil {
			m.media.SetCapMB(*p.MediaCacheMB)
		}
	}
	return protocol.EmptyResult{}, nil
}
