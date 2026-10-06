package remoteauth

import (
	"context"
	"crypto/rand"
	"crypto/rsa"
	"crypto/sha256"
	"crypto/x509"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"strings"
	"time"

	"github.com/diamondburned/arikawa/v3/api"
	"github.com/diamondburned/arikawa/v3/utils/httputil"
	"github.com/gorilla/websocket"
)

const GatewayURL = "wss://remote-auth-gateway.discord.gg/?v=2"

const Origin = "https://discord.com"

const DefaultUserAgent = "Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/146.0.0.0 Safari/537.36"

var (
	ErrDeclined = errors.New("remoteauth: login declined on phone")
	ErrExpired  = errors.New("remoteauth: QR code expired")
	ErrExchange = errors.New("remoteauth: ticket exchange failed")
	ErrProtocol = errors.New("remoteauth: protocol error")
)

type User struct {
	ID            string
	Discriminator string
	AvatarHash    string
	Username      string
}

type Events interface {
	QRCode(url string, fingerprint string, expiresIn time.Duration)
	Scanned(u User)
	Approved()
}

type Result struct {
	Token string
	User  User
}

type Options struct {
	UserAgent  string
	GatewayURL string
	Dial       func(ctx context.Context, url string, header http.Header) (*websocket.Conn, error)
	Exchange   func(ctx context.Context, ticket, fingerprint string) (encryptedToken string, err error)
}

func (o Options) withDefaults() Options {
	if o.UserAgent == "" {
		o.UserAgent = DefaultUserAgent
	}
	if o.GatewayURL == "" {
		o.GatewayURL = GatewayURL
	}
	if o.Dial == nil {
		o.Dial = func(ctx context.Context, url string, header http.Header) (*websocket.Conn, error) {
			conn, _, err := websocket.DefaultDialer.DialContext(ctx, url, header)
			return conn, err
		}
	}
	if o.Exchange == nil {
		ua := o.UserAgent
		o.Exchange = func(ctx context.Context, ticket, fingerprint string) (string, error) {
			return exchangeTicket(ctx, ua, ticket, fingerprint)
		}
	}
	return o
}

func Run(ctx context.Context, ev Events, opts Options) (Result, error) {
	opts = opts.withDefaults()

	key, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		return Result{}, fmt.Errorf("remoteauth: generate key: %w", err)
	}
	spki, err := x509.MarshalPKIXPublicKey(&key.PublicKey)
	if err != nil {
		return Result{}, fmt.Errorf("remoteauth: encode public key: %w", err)
	}

	header := http.Header{}
	header.Set("User-Agent", opts.UserAgent)
	header.Set("Origin", Origin)
	conn, err := opts.Dial(ctx, opts.GatewayURL, header)
	if err != nil {
		return Result{}, fmt.Errorf("remoteauth: dial gateway: %w", err)
	}

	f := &flow{conn: conn, key: key, spki: spki, ev: ev}
	ticket, fingerprint, err := f.run(ctx)
	_ = conn.Close()
	if err != nil {
		return Result{}, err
	}

	encToken, err := opts.Exchange(ctx, ticket, fingerprint)
	if err != nil {
		return Result{}, wrapExchangeErr(err)
	}
	tokenBytes, err := decryptB64(key, encToken)
	if err != nil {
		return Result{}, fmt.Errorf("%w: decrypt token", ErrExchange)
	}
	return Result{Token: string(tokenBytes), User: f.user}, nil
}

type flow struct {
	conn    *websocket.Conn
	key     *rsa.PrivateKey
	spki    []byte
	ev      Events
	user    User
	timeout time.Duration
}

type readResult struct {
	data []byte
	err  error
}

func (f *flow) run(ctx context.Context) (string, string, error) {
	reads := make(chan readResult, 1)
	done := make(chan struct{})
	defer close(done)
	go func() {
		for {
			_, data, err := f.conn.ReadMessage()
			select {
			case reads <- readResult{data, err}:
			case <-ctx.Done():
				return
			case <-done:
				return
			}
			if err != nil {
				return
			}
		}
	}()

	var (
		heartbeat   *time.Ticker
		heartbeatC  <-chan time.Time
		expiry      *time.Timer
		expiryC     <-chan time.Time
		fingerprint string
		gotHello    bool
	)
	defer func() {
		if heartbeat != nil {
			heartbeat.Stop()
		}
		if expiry != nil {
			expiry.Stop()
		}
	}()

	for {
		select {
		case <-ctx.Done():
			return "", "", ctx.Err()
		case <-expiryC:
			return "", "", ErrExpired
		case <-heartbeatC:
			if err := f.send(map[string]string{"op": "heartbeat"}); err != nil {
				return "", "", fmt.Errorf("remoteauth: send heartbeat: %w", err)
			}
		case r := <-reads:
			if r.err != nil {
				if ctx.Err() != nil {
					return "", "", ctx.Err()
				}
				if websocket.IsCloseError(r.err, 4003) {
					return "", "", ErrExpired
				}
				return "", "", fmt.Errorf("remoteauth: gateway read: %w", r.err)
			}
			var msg gatewayMsg
			if err := json.Unmarshal(r.data, &msg); err != nil {
				return "", "", fmt.Errorf("%w: bad json: %v", ErrProtocol, err)
			}
			if !gotHello && msg.Op != "hello" {
				return "", "", fmt.Errorf("%w: expected hello, got %q", ErrProtocol, msg.Op)
			}

			switch msg.Op {
			case "hello":
				gotHello = true
				if msg.HeartbeatInterval <= 0 || msg.TimeoutMS <= 0 {
					return "", "", fmt.Errorf("%w: invalid hello", ErrProtocol)
				}
				heartbeat = time.NewTicker(time.Duration(msg.HeartbeatInterval) * time.Millisecond)
				heartbeatC = heartbeat.C
				expiry = time.NewTimer(time.Duration(msg.TimeoutMS) * time.Millisecond)
				expiryC = expiry.C
				f.timeout = time.Duration(msg.TimeoutMS) * time.Millisecond
				if err := f.send(map[string]string{
					"op":                 "init",
					"encoded_public_key": base64.StdEncoding.EncodeToString(f.spki),
				}); err != nil {
					return "", "", fmt.Errorf("remoteauth: send init: %w", err)
				}

			case "heartbeat_ack":

			case "nonce_proof":
				nonce, err := decryptB64(f.key, msg.EncryptedNonce)
				if err != nil {
					return "", "", fmt.Errorf("%w: decrypt nonce", ErrProtocol)
				}
				if err := f.send(map[string]string{
					"op":    "nonce_proof",
					"nonce": base64.RawURLEncoding.EncodeToString(nonce),
				}); err != nil {
					return "", "", fmt.Errorf("remoteauth: send nonce_proof: %w", err)
				}

			case "pending_remote_init":
				sum := sha256.Sum256(f.spki)
				if want := base64.RawURLEncoding.EncodeToString(sum[:]); msg.Fingerprint != want {
					return "", "", fmt.Errorf("%w: fingerprint does not match our public key", ErrProtocol)
				}
				fingerprint = msg.Fingerprint
				f.ev.QRCode("https://discord.com/ra/"+fingerprint, fingerprint, f.timeout)

			case "pending_ticket":
				payload, err := decryptB64(f.key, msg.EncryptedUserPayload)
				if err != nil {
					return "", "", fmt.Errorf("%w: decrypt user payload", ErrProtocol)
				}
				parts := strings.Split(string(payload), ":")
				if len(parts) != 4 {
					return "", "", fmt.Errorf("%w: malformed user payload", ErrProtocol)
				}
				f.user = User{ID: parts[0], Discriminator: parts[1], AvatarHash: parts[2], Username: parts[3]}
				if f.user.AvatarHash == "0" {
					f.user.AvatarHash = ""
				}
				f.ev.Scanned(f.user)

			case "cancel":
				return "", "", ErrDeclined

			case "pending_login":
				if msg.Ticket == "" || fingerprint == "" {
					return "", "", fmt.Errorf("%w: pending_login before handshake completed", ErrProtocol)
				}
				f.ev.Approved()
				return msg.Ticket, fingerprint, nil

			default:
			}
		}
	}
}

func (f *flow) send(v any) error {
	_ = f.conn.SetWriteDeadline(time.Now().Add(10 * time.Second))
	return f.conn.WriteJSON(v)
}

type gatewayMsg struct {
	Op                   string `json:"op"`
	HeartbeatInterval    int    `json:"heartbeat_interval"`
	TimeoutMS            int    `json:"timeout_ms"`
	EncryptedNonce       string `json:"encrypted_nonce"`
	Fingerprint          string `json:"fingerprint"`
	EncryptedUserPayload string `json:"encrypted_user_payload"`
	Ticket               string `json:"ticket"`
}

func decryptB64(key *rsa.PrivateKey, enc string) ([]byte, error) {
	raw, err := base64.StdEncoding.DecodeString(enc)
	if err != nil {
		return nil, err
	}
	return rsa.DecryptOAEP(sha256.New(), nil, key, raw, nil)
}

func exchangeTicket(ctx context.Context, userAgent, ticket, fingerprint string) (string, error) {
	client := api.NewClient("").WithContext(ctx)
	client.UserAgent = userAgent
	h := http.Header{}
	h.Set("Origin", Origin)
	h.Set("Referer", Origin+"/login")
	if fingerprint != "" {
		h.Set("X-Fingerprint", fingerprint)
	}
	client.OnRequest = append(client.OnRequest, httputil.WithHeaders(h))
	return client.ExchangeRemoteAuthTicket(ticket)
}

func wrapExchangeErr(err error) error {
	var httpErr *httputil.HTTPError
	if errors.As(err, &httpErr) {
		return fmt.Errorf("%w: http %d", ErrExchange, httpErr.Status)
	}
	if errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded) {
		return err
	}
	return fmt.Errorf("%w: %T", ErrExchange, err)
}
