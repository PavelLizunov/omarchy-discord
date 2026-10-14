package session

import (
	"github.com/diamondburned/arikawa/v3/gateway"
	"github.com/diamondburned/arikawa/v3/utils/ws"
)

type streamCreateEvent struct {
	Key     string `json:"stream_key"`
	Server  string `json:"rtc_server_id"`
	Channel string `json:"rtc_channel_id"`
}

func (*streamCreateEvent) Op() ws.OpCode           { return 0 }
func (*streamCreateEvent) EventType() ws.EventType { return "STREAM_CREATE" }

type streamServerEvent struct {
	Key      string `json:"stream_key"`
	Endpoint string `json:"endpoint"`
	Token    string `json:"token"`
}

func (*streamServerEvent) Op() ws.OpCode           { return 0 }
func (*streamServerEvent) EventType() ws.EventType { return "STREAM_SERVER_UPDATE" }

type streamDeleteEvent struct {
	Key string `json:"stream_key"`
}

func (*streamDeleteEvent) Op() ws.OpCode           { return 0 }
func (*streamDeleteEvent) EventType() ws.EventType { return "STREAM_DELETE" }

type streamCommand struct {
	Code ws.OpCode `json:"-"`
	Key  string    `json:"stream_key"`
}

func (s *streamCommand) Op() ws.OpCode         { return s.Code }
func (*streamCommand) EventType() ws.EventType { return "" }
func init() {
	gateway.OpUnmarshalers.Add(func() ws.Event { return new(streamCreateEvent) }, func() ws.Event { return new(streamServerEvent) }, func() ws.Event { return new(streamDeleteEvent) })
}
