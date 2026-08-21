package session

import (
	"errors"
	"sort"

	"github.com/diamondburned/arikawa/v3/discord"
	"github.com/diamondburned/arikawa/v3/gateway"
	"github.com/diamondburned/ningen/v3"

	"github.com/mattcalayo/omarchy-discord/backend/internal/protocol"
)

// errUnknownChannel is returned by Threads for an uncached parent.
var errUnknownChannel = errors.New("unknown channel")

// Threads lists the active (unarchived) cached threads of a text channel,
// newest activity first. errUnknownChannel when the parent is not cached.
func Threads(n *ningen.State, parentID discord.ChannelID) ([]protocol.Channel, error) {
	parent, err := n.Cabinet.Channel(parentID)
	if err != nil {
		return nil, errUnknownChannel
	}
	out := []protocol.Channel{}
	if !parent.GuildID.IsValid() {
		return out, nil
	}
	chs, err := n.Cabinet.Channels(parent.GuildID)
	if err != nil {
		return out, nil
	}
	var threads []discord.Channel
	for _, ch := range chs {
		if isThread(ch.Type) && ch.ParentID == parentID && !archived(&ch) {
			threads = append(threads, ch)
		}
	}
	sort.SliceStable(threads, func(i, j int) bool {
		if threads[i].LastMessageID != threads[j].LastMessageID {
			return threads[i].LastMessageID > threads[j].LastMessageID
		}
		return threads[i].ID > threads[j].ID
	})
	for _, ch := range threads {
		out = append(out, wireChannel(n, ch))
	}
	return out, nil
}

// listThreads implements the list_threads command.
func (m *Manager) listThreads(req *protocol.Request) (any, *protocol.Error) {
	var p protocol.ListThreadsParams
	if e := req.Params(&p); e != nil {
		return nil, e
	}
	sf, e := parseSnowflake(p.ChannelID, "channel_id")
	if e != nil {
		return nil, e
	}
	n, e := m.cachedSession()
	if e != nil {
		return nil, e
	}
	threads, err := Threads(n.Offline(), discord.ChannelID(sf))
	if err != nil {
		return nil, protocol.Errorf(protocol.CodeUnknownChannel, "channel %s is not visible to this account", p.ChannelID)
	}
	return protocol.ListThreadsResult{Threads: threads}, nil
}

// installChannelHandlers turns gateway channel/thread lifecycle events into
// channel_update broadcasts. Handlers are sync on ningen's handler, so the
// cabinet already reflects the change.
func (m *Manager) installChannelHandlers(n *ningen.State) {
	live := func() bool {
		m.mu.Lock()
		defer m.mu.Unlock()
		return m.n == n && m.everReady
	}
	emit := func(change string, ch discord.Channel) {
		if !live() {
			return
		}
		m.push(protocol.NewChannelUpdate(change, wireChannel(n.Offline(), ch)))
	}
	n.AddSyncHandler(func(ev *gateway.ChannelCreateEvent) { emit(protocol.ChannelChangeCreate, ev.Channel) })
	n.AddSyncHandler(func(ev *gateway.ChannelUpdateEvent) { emit(protocol.ChannelChangeUpdate, ev.Channel) })
	n.AddSyncHandler(func(ev *gateway.ChannelDeleteEvent) { emit(protocol.ChannelChangeDelete, ev.Channel) })
	n.AddSyncHandler(func(ev *gateway.ThreadCreateEvent) { emit(protocol.ChannelChangeCreate, ev.Channel) })
	n.AddSyncHandler(func(ev *gateway.ThreadUpdateEvent) { emit(protocol.ChannelChangeUpdate, ev.Channel) })
	n.AddSyncHandler(func(ev *gateway.ThreadDeleteEvent) {
		emit(protocol.ChannelChangeDelete, discord.Channel{ID: ev.ID, GuildID: ev.GuildID, Type: ev.Type, ParentID: ev.ParentID})
	})
	n.AddSyncHandler(func(ev *gateway.ThreadListSyncEvent) {
		for _, th := range ev.Threads {
			if th.GuildID == 0 {
				th.GuildID = ev.GuildID
			}
			emit(protocol.ChannelChangeCreate, th)
		}
	})
}
