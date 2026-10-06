package session

import (
	"errors"
	"sort"

	"github.com/diamondburned/arikawa/v3/discord"
	"github.com/diamondburned/arikawa/v3/gateway"
	"github.com/diamondburned/ningen/v3"

	"github.com/mattcalayo/omarchy-discord/backend/internal/protocol"
)

var errUnknownChannel = errors.New("unknown channel")

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
	addSyncHandler(n, "channel_create", func(ev *gateway.ChannelCreateEvent) { emit(protocol.ChannelChangeCreate, ev.Channel) })
	addSyncHandler(n, "channel_update", func(ev *gateway.ChannelUpdateEvent) { emit(protocol.ChannelChangeUpdate, ev.Channel) })
	addSyncHandler(n, "channel_delete", func(ev *gateway.ChannelDeleteEvent) { emit(protocol.ChannelChangeDelete, ev.Channel) })
	addSyncHandler(n, "thread_create", func(ev *gateway.ThreadCreateEvent) { emit(protocol.ChannelChangeCreate, ev.Channel) })
	addSyncHandler(n, "thread_update", func(ev *gateway.ThreadUpdateEvent) { emit(protocol.ChannelChangeUpdate, ev.Channel) })
	addSyncHandler(n, "thread_delete", func(ev *gateway.ThreadDeleteEvent) {
		emit(protocol.ChannelChangeDelete, discord.Channel{ID: ev.ID, GuildID: ev.GuildID, Type: ev.Type, ParentID: ev.ParentID})
	})
	addSyncHandler(n, "thread_list_sync", func(ev *gateway.ThreadListSyncEvent) {
		for _, th := range ev.Threads {
			if th.GuildID == 0 {
				th.GuildID = ev.GuildID
			}
			emit(protocol.ChannelChangeCreate, th)
		}
	})
}
