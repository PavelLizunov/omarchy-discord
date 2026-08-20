// Package protocol implements the wire contract in docs/BACKEND_PROTOCOL.md:
// line-delimited JSON requests, responses, and events, protocol version 1.
package protocol

import (
	"encoding/json"
	"errors"
	"fmt"

	"github.com/mattcalayo/omarchy-discord/backend/internal/redact"
)

// Version is the only protocol version this backend speaks.
const Version = 1

// BackendVersion is the semver of the binary, reported in hello and state.
const BackendVersion = "0.1.0"

// Engine is reported by hello.
const Engine = "arikawa"

// Error codes. Stable and machine-readable; see the protocol doc table.
const (
	CodeInvalidRequest     = "invalid_request"
	CodeUnsupportedVersion = "unsupported_version"
	CodeUnknownCommand     = "unknown_command"
	CodeInvalidArgument    = "invalid_argument"
	CodeSerializationError = "serialization_error"
	CodeNotLoggedIn        = "not_logged_in"
	CodeLoginFailed        = "login_failed"
	CodeGatewayUnavailable = "gateway_unavailable"
	CodeUnknownGuild       = "unknown_guild"
	CodeUnknownChannel     = "unknown_channel"
	CodeDiscordError       = "discord_error"
	CodeInternalError      = "internal_error"
)

// Error is the failure payload of a response.
type Error struct {
	Code    string `json:"code"`
	Message string `json:"message"`
}

func (e *Error) Error() string { return e.Code + ": " + e.Message }

// Errorf builds an Error with a redacted, formatted message.
func Errorf(code, format string, args ...any) *Error {
	return &Error{Code: code, Message: redact.Redact(fmt.Sprintf(format, args...))}
}

// Request is a decoded request line. Params stay raw and are decoded per
// command with Request.Params.
type Request struct {
	V       int    `json:"v"`
	ID      int64  `json:"id"`
	Command string `json:"command"`

	raw []byte
}

// Params decodes the flattened request parameters into dst.
func (r *Request) Params(dst any) *Error {
	if err := json.Unmarshal(r.raw, dst); err != nil {
		return Errorf(CodeInvalidArgument, "bad parameters for %s: %v", r.Command, err)
	}
	return nil
}

// Raw returns the original request line (without the trailing newline).
func (r *Request) Raw() []byte { return r.raw }

// DecodeRequest parses one request line. A parse failure yields an
// invalid_request error; a version mismatch yields unsupported_version with the
// request's id preserved when it could be read.
func DecodeRequest(line []byte) (*Request, *Error) {
	var req Request
	if err := json.Unmarshal(line, &req); err != nil {
		return nil, Errorf(CodeInvalidRequest, "malformed request: %v", err)
	}
	req.raw = append([]byte(nil), line...)
	if req.V != Version {
		return &req, Errorf(CodeUnsupportedVersion, "unsupported protocol version %d (want %d)", req.V, Version)
	}
	if req.Command == "" {
		return &req, Errorf(CodeInvalidRequest, "missing command")
	}
	return &req, nil
}

// Response is the reply to one request. Result and Err are mutually exclusive.
type Response struct {
	Type   string `json:"type"`
	V      int    `json:"v"`
	ID     int64  `json:"id"`
	OK     bool   `json:"ok"`
	Result any    `json:"result,omitempty"`
	Err    *Error `json:"error,omitempty"`
}

// OKResponse builds a success response.
func OKResponse(id int64, result any) Response {
	return Response{Type: "response", V: Version, ID: id, OK: true, Result: result}
}

// ErrResponse builds a failure response.
func ErrResponse(id int64, e *Error) Response {
	return Response{Type: "response", V: Version, ID: id, OK: false, Err: e}
}

// EventHeader is embedded (first) in every event payload struct.
type EventHeader struct {
	Type  string `json:"type"`
	V     int    `json:"v"`
	Event string `json:"event"`
}

func header(name string) EventHeader {
	return EventHeader{Type: "event", V: Version, Event: name}
}

// Encode serializes one wire object as a single redacted JSON line including
// the trailing newline.
func Encode(v any) ([]byte, error) {
	b, err := json.Marshal(v)
	if err != nil {
		return nil, err
	}
	b = []byte(redact.Redact(string(b)))
	return append(b, '\n'), nil
}

// MustEncode encodes v; if serialization fails it encodes a serialization_error
// response with the given id instead (which cannot fail).
func MustEncode(id int64, v any) []byte {
	b, err := Encode(v)
	if err == nil {
		return b
	}
	b, err = Encode(ErrResponse(id, Errorf(CodeSerializationError, "%v", err)))
	if err != nil {
		panic(errors.New("protocol: cannot encode error response"))
	}
	return b
}
