package session

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"errors"
	"net/http"
	"strings"
	"sync"
	"time"
	"unicode/utf8"

	"github.com/diamondburned/arikawa/v3/api"
	"github.com/diamondburned/arikawa/v3/discord"
	"github.com/diamondburned/arikawa/v3/utils/httputil"
	"github.com/diamondburned/arikawa/v3/utils/json/option"
	"github.com/diamondburned/ningen/v3"

	"github.com/mattcalayo/omarchy-discord/backend/internal/protocol"
	"github.com/mattcalayo/omarchy-discord/backend/internal/redact"
	"github.com/mattcalayo/omarchy-discord/backend/internal/socket"
)

const (
	// typingInterval is the self-throttle for outbound typing per channel.
	typingInterval = 10 * time.Second
	// contentLimit / contentLimitNitro bound message content (runes).
	contentLimit      = 2000
	contentLimitNitro = 4000
)

// restOps are the write calls against Discord. Tests replace them; the
// defaults go through the live session's REST client.
type restOps struct {
	send      func(ctx context.Context, n *ningen.State, chID discord.ChannelID, data api.SendMessageData) (*discord.Message, error)
	edit      func(ctx context.Context, n *ningen.State, chID discord.ChannelID, msgID discord.MessageID, content string) error
	delete    func(ctx context.Context, n *ningen.State, chID discord.ChannelID, msgID discord.MessageID) error
	react     func(ctx context.Context, n *ningen.State, chID discord.ChannelID, msgID discord.MessageID, emoji discord.APIEmoji) error
	unreact   func(ctx context.Context, n *ningen.State, chID discord.ChannelID, msgID discord.MessageID, emoji discord.APIEmoji) error
	typing    func(ctx context.Context, n *ningen.State, chID discord.ChannelID) error
	setStatus func(n *ningen.State, status discord.Status) error
}

func liveREST() restOps {
	return restOps{
		send: func(ctx context.Context, n *ningen.State, chID discord.ChannelID, data api.SendMessageData) (*discord.Message, error) {
			client := n.Client.WithContext(ctx)
			if len(data.Files) > 0 {
				// arikawa retries 429/5xx by re-sending the request, but the
				// multipart body is a pipe that is consumed on the first
				// attempt; a retry would send a corrupt body. One attempt,
				// and the caller reports rate_limited / discord_error.
				client.Retries = 1
			}
			return client.SendMessageComplex(chID, data)
		},
		edit: func(ctx context.Context, n *ningen.State, chID discord.ChannelID, msgID discord.MessageID, content string) error {
			_, err := n.Client.WithContext(ctx).EditText(chID, msgID, content)
			return err
		},
		delete: func(ctx context.Context, n *ningen.State, chID discord.ChannelID, msgID discord.MessageID) error {
			return n.Client.WithContext(ctx).DeleteMessage(chID, msgID, "")
		},
		react: func(ctx context.Context, n *ningen.State, chID discord.ChannelID, msgID discord.MessageID, emoji discord.APIEmoji) error {
			return n.Client.WithContext(ctx).React(chID, msgID, emoji)
		},
		unreact: func(ctx context.Context, n *ningen.State, chID discord.ChannelID, msgID discord.MessageID, emoji discord.APIEmoji) error {
			return n.Client.WithContext(ctx).Unreact(chID, msgID, emoji)
		},
		typing: func(ctx context.Context, n *ningen.State, chID discord.ChannelID) error {
			return n.Client.WithContext(ctx).Typing(chID)
		},
		setStatus: func(n *ningen.State, status discord.Status) error {
			return n.SetStatus(status, nil)
		},
	}
}

// typingThrottle remembers the last outbound typing call per channel.
type typingThrottle struct {
	mu   sync.Mutex
	last map[discord.ChannelID]time.Time
}

// allow reports whether a typing call may go out now and records it if so.
func (t *typingThrottle) allow(chID discord.ChannelID, now time.Time) bool {
	t.mu.Lock()
	defer t.mu.Unlock()
	if t.last == nil {
		t.last = map[discord.ChannelID]time.Time{}
	}
	if last, ok := t.last[chID]; ok && now.Sub(last) < typingInterval {
		return false
	}
	t.last[chID] = now
	return true
}

// newNonce returns 16 random hex characters.
func newNonce() string {
	var b [8]byte
	if _, err := rand.Read(b[:]); err != nil {
		return hex.EncodeToString([]byte(time.Now().Format("150405.000")))[:16]
	}
	return hex.EncodeToString(b[:])
}

// sanitizeEmoji strips U+FE0F variation selectors (the REST call 400s on
// them) and validates the shape: unicode, or "name:id" for custom emoji.
func sanitizeEmoji(s string) (discord.APIEmoji, *protocol.Error) {
	s = strings.ReplaceAll(strings.TrimSpace(s), "\ufe0f", "")
	if s == "" {
		return "", protocol.Errorf(protocol.CodeInvalidArgument, "emoji is required")
	}
	if i := strings.LastIndexByte(s, ':'); i >= 0 {
		name, id := s[:i], s[i+1:]
		sf, err := discord.ParseSnowflake(id)
		if name == "" || err != nil || !sf.IsValid() {
			return "", protocol.Errorf(protocol.CodeInvalidArgument, "custom emoji must be \"name:id\"")
		}
		return discord.NewAPIEmoji(discord.EmojiID(sf), name), nil
	}
	return discord.APIEmoji(s), nil
}

// liveSession returns the session for write commands: it must exist, have
// seen READY, and currently be connected.
func (m *Manager) liveSession() (*ningen.State, *protocol.Error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	switch {
	case m.n == nil || m.lifecycle == protocol.LifecycleReauthNeeded:
		return nil, protocol.Errorf(protocol.CodeNotLoggedIn, "not logged in")
	case !m.everReady || m.lifecycle != protocol.LifecycleReady:
		return nil, protocol.Errorf(protocol.CodeGatewayUnavailable, "gateway is not connected")
	}
	return m.n, nil
}

// requireOpen checks that the connection has the channel open.
func requireOpen(ctx context.Context, chID string) (socket.Client, *protocol.Error) {
	c := socket.ClientFromContext(ctx)
	if c == nil || !c.HasOpen(chID) {
		return nil, protocol.Errorf(protocol.CodeChannelNotOpen, "channel %s is not open", chID)
	}
	return c, nil
}

// restError maps a REST failure on any command. A 401 means the token is
// dead: the session is moved to reauth_needed and the command fails with
// not_logged_in. 404 maps to unknown_message for message-scoped calls.
func (m *Manager) restError(n *ningen.State, err error, messageScoped bool) *protocol.Error {
	var herr *httputil.HTTPError
	if errors.As(err, &herr) {
		switch herr.Status {
		case http.StatusUnauthorized:
			m.reauthFromREST(n, err)
			return protocol.Errorf(protocol.CodeNotLoggedIn, "session invalidated; log in again")
		case http.StatusNotFound:
			if messageScoped {
				return protocol.Errorf(protocol.CodeUnknownMessage, "message not found: %v", err)
			}
		}
	}
	return discordError(err)
}

// reauthFromREST handles a REST 401 on the live session: stop the gateway
// loop, drop the token, clear the keyring, keep the cache read-only.
func (m *Manager) reauthFromREST(n *ningen.State, cause error) {
	m.mu.Lock()
	if m.n != n || m.lifecycle == protocol.LifecycleReauthNeeded {
		m.mu.Unlock()
		return
	}
	if m.cancel != nil {
		m.cancel()
		m.cancel = nil
	}
	m.token = ""
	// The session is done: the call goes with it, even though the cache
	// stays readable.
	v := m.voice
	m.voice, m.voiceState = nil, idleVoice
	m.setLifecycleLocked(protocol.LifecycleReauthNeeded, "session invalidated: "+cause.Error())
	m.mu.Unlock()
	// Close the gateway off the command path; a later login's teardown
	// tolerates an already-closed session.
	go closeAndWait(n, nil, v)
	if err := m.kr.Clear(context.Background()); err != nil {
		redact.Logf("session: keyring clear after 401: %v", err)
	}
}

// validateContent checks message text length against the account's cap.
func validateContent(n *ningen.State, content string, allowEmpty bool) *protocol.Error {
	if strings.TrimSpace(content) == "" {
		if allowEmpty {
			return nil
		}
		return protocol.Errorf(protocol.CodeInvalidArgument, "content is empty")
	}
	limit := contentLimit
	if me, err := n.Offline().Cabinet.Me(); err == nil && me.Nitro == discord.NitroFull {
		limit = contentLimitNitro
	}
	if utf8.RuneCountInString(content) > limit {
		return protocol.Errorf(protocol.CodeInvalidArgument, "content exceeds %d characters", limit)
	}
	return nil
}

// sendData builds SendMessageData for send/upload: nonce always set, reply
// reference and reply mention when requested.
func sendData(content, nonce, replyTo string, replyMention *bool) (api.SendMessageData, *protocol.Error) {
	data := api.SendMessageData{Content: content, Nonce: nonce}
	if replyTo != "" {
		sf, e := parseSnowflake(replyTo, "reply_to")
		if e != nil {
			return data, e
		}
		data.Reference = &discord.MessageReference{MessageID: discord.MessageID(sf)}
		mention := replyMention == nil || *replyMention
		data.AllowedMentions = &api.AllowedMentions{
			Parse:       []api.AllowedMentionType{api.AllowUserMention, api.AllowRoleMention, api.AllowEveryoneMention},
			RepliedUser: option.Bool(&mention),
		}
	}
	return data, nil
}

// send implements the send command.
func (m *Manager) send(ctx context.Context, req *protocol.Request) (any, *protocol.Error) {
	var p protocol.SendParams
	if e := req.Params(&p); e != nil {
		return nil, e
	}
	sf, e := parseSnowflake(p.ChannelID, "channel_id")
	if e != nil {
		return nil, e
	}
	if _, e := requireOpen(ctx, p.ChannelID); e != nil {
		return nil, e
	}
	n, e := m.liveSession()
	if e != nil {
		return nil, e
	}
	if e := validateContent(n, p.Content, false); e != nil {
		return nil, e
	}
	data, e := sendData(p.Content, newNonce(), p.ReplyTo, p.ReplyMention)
	if e != nil {
		return nil, e
	}
	msg, err := m.rest.send(ctx, n, discord.ChannelID(sf), data)
	if err != nil {
		return nil, m.restError(n, err, false)
	}
	return protocol.SendResult{MessageID: msg.ID.String(), Nonce: data.Nonce}, nil
}

// messageTarget parses the channel/message pair shared by edit/delete/react
// and checks the channel is open on this connection.
func (m *Manager) messageTarget(ctx context.Context, chStr, msgStr string) (*ningen.State, discord.ChannelID, discord.MessageID, *protocol.Error) {
	chSF, e := parseSnowflake(chStr, "channel_id")
	if e != nil {
		return nil, 0, 0, e
	}
	msgSF, e := parseSnowflake(msgStr, "message_id")
	if e != nil {
		return nil, 0, 0, e
	}
	if _, e := requireOpen(ctx, chStr); e != nil {
		return nil, 0, 0, e
	}
	n, e := m.liveSession()
	if e != nil {
		return nil, 0, 0, e
	}
	return n, discord.ChannelID(chSF), discord.MessageID(msgSF), nil
}

// edit implements the edit command (own messages only).
func (m *Manager) edit(ctx context.Context, req *protocol.Request) (any, *protocol.Error) {
	var p protocol.EditParams
	if e := req.Params(&p); e != nil {
		return nil, e
	}
	n, chID, msgID, e := m.messageTarget(ctx, p.ChannelID, p.MessageID)
	if e != nil {
		return nil, e
	}
	if e := validateContent(n, p.Content, false); e != nil {
		return nil, e
	}
	off := n.Offline()
	if msg, err := off.Cabinet.Message(chID, msgID); err == nil {
		if me, err := off.Cabinet.Me(); err == nil && msg.Author.ID != me.ID {
			return nil, protocol.Errorf(protocol.CodeForbidden, "only own messages can be edited")
		}
	}
	if err := m.rest.edit(ctx, n, chID, msgID, p.Content); err != nil {
		return nil, m.restError(n, err, true)
	}
	return protocol.EmptyResult{}, nil
}

// deleteMessage implements the delete command.
func (m *Manager) deleteMessage(ctx context.Context, req *protocol.Request) (any, *protocol.Error) {
	var p protocol.DeleteParams
	if e := req.Params(&p); e != nil {
		return nil, e
	}
	n, chID, msgID, e := m.messageTarget(ctx, p.ChannelID, p.MessageID)
	if e != nil {
		return nil, e
	}
	if err := m.rest.delete(ctx, n, chID, msgID); err != nil {
		return nil, m.restError(n, err, true)
	}
	return protocol.EmptyResult{}, nil
}

// react implements react (add=true) and unreact (add=false).
func (m *Manager) react(ctx context.Context, req *protocol.Request, add bool) (any, *protocol.Error) {
	var p protocol.ReactParams
	if e := req.Params(&p); e != nil {
		return nil, e
	}
	emoji, e := sanitizeEmoji(p.Emoji)
	if e != nil {
		return nil, e
	}
	n, chID, msgID, e := m.messageTarget(ctx, p.ChannelID, p.MessageID)
	if e != nil {
		return nil, e
	}
	call := m.rest.unreact
	if add {
		call = m.rest.react
	}
	if err := call(ctx, n, chID, msgID, emoji); err != nil {
		return nil, m.restError(n, err, true)
	}
	return protocol.EmptyResult{}, nil
}

// typing implements the typing command with the per-channel throttle; a
// throttled call still succeeds.
func (m *Manager) typing(ctx context.Context, req *protocol.Request) (any, *protocol.Error) {
	var p protocol.TypingParams
	if e := req.Params(&p); e != nil {
		return nil, e
	}
	sf, e := parseSnowflake(p.ChannelID, "channel_id")
	if e != nil {
		return nil, e
	}
	if _, e := requireOpen(ctx, p.ChannelID); e != nil {
		return nil, e
	}
	n, e := m.liveSession()
	if e != nil {
		return nil, e
	}
	chID := discord.ChannelID(sf)
	if !m.typers.allow(chID, m.now()) {
		return protocol.EmptyResult{}, nil
	}
	if err := m.rest.typing(ctx, n, chID); err != nil {
		return nil, m.restError(n, err, false)
	}
	return protocol.EmptyResult{}, nil
}

// setPresence implements set_presence.
func (m *Manager) setPresence(req *protocol.Request) (any, *protocol.Error) {
	var p protocol.SetPresenceParams
	if e := req.Params(&p); e != nil {
		return nil, e
	}
	var status discord.Status
	switch p.Status {
	case "online":
		status = discord.OnlineStatus
	case "idle":
		status = discord.IdleStatus
	case "dnd":
		status = discord.DoNotDisturbStatus
	case "invisible", "offline":
		status = discord.InvisibleStatus
	default:
		return nil, protocol.Errorf(protocol.CodeInvalidArgument, "status must be one of online, idle, dnd, invisible")
	}
	n, e := m.liveSession()
	if e != nil {
		return nil, e
	}
	if err := m.rest.setStatus(n, status); err != nil {
		return nil, m.restError(n, err, false)
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.n == n && m.presence != presenceString(status) {
		m.presence = presenceString(status)
		m.bump()
	}
	return protocol.EmptyResult{}, nil
}
