package protocol

import (
	"encoding/json"
	"errors"
	"fmt"

	"github.com/mattcalayo/omarchy-discord/backend/internal/redact"
)

const Version = 1

const BackendVersion = "0.1.0"

const Engine = "arikawa"

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
	CodeUnknownMessage     = "unknown_message"
	CodeChannelNotOpen     = "channel_not_open"
	CodeForbidden          = "forbidden"
	CodeRateLimited        = "rate_limited"
	CodeEmptyDMRefused     = "empty_dm_refused"
	CodeUploadTooLarge     = "upload_too_large"
	CodeMediaError         = "media_error"
	CodeQRUnavailable      = "qr_unavailable"
	CodeDiscordError       = "discord_error"
	CodeInternalError      = "internal_error"
)

type Error struct {
	Code    string `json:"code"`
	Message string `json:"message"`
}

func (e *Error) Error() string { return e.Code + ": " + e.Message }

func Errorf(code, format string, args ...any) *Error {
	return &Error{Code: code, Message: redact.Redact(fmt.Sprintf(format, args...))}
}

type Request struct {
	V       int    `json:"v"`
	ID      int64  `json:"id"`
	Command string `json:"command"`

	raw []byte
}

func (r *Request) Params(dst any) *Error {
	if err := json.Unmarshal(r.raw, dst); err != nil {
		return Errorf(CodeInvalidArgument, "bad parameters for %s: %v", r.Command, err)
	}
	return nil
}

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

type Response struct {
	Type   string `json:"type"`
	V      int    `json:"v"`
	ID     int64  `json:"id"`
	OK     bool   `json:"ok"`
	Result any    `json:"result,omitempty"`
	Err    *Error `json:"error,omitempty"`
}

func OKResponse(id int64, result any) Response {
	return Response{Type: "response", V: Version, ID: id, OK: true, Result: result}
}

func ErrResponse(id int64, e *Error) Response {
	return Response{Type: "response", V: Version, ID: id, OK: false, Err: e}
}

type EventHeader struct {
	Type  string `json:"type"`
	V     int    `json:"v"`
	Event string `json:"event"`
}

func header(name string) EventHeader {
	return EventHeader{Type: "event", V: Version, Event: name}
}

func Encode(v any) ([]byte, error) {
	b, err := json.Marshal(v)
	if err != nil {
		return nil, err
	}
	return append(b, '\n'), nil
}

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
