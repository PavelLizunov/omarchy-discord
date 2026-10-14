package session

import (
	"bufio"
	"context"
	"encoding/json"
	"fmt"
	"net"
	"path/filepath"
	"runtime"
	"sync"
	"testing"
	"time"

	"github.com/diamondburned/arikawa/v3/discord"
	"github.com/diamondburned/arikawa/v3/gateway"
	"github.com/mattcalayo/omarchy-discord/backend/internal/protocol"
	"github.com/mattcalayo/omarchy-discord/backend/internal/socket"
)

func TestDMClosedSocketRefreshWithoutNotification(t *testing.T) {
	m, n := readyManager(t)
	drain(m)
	// NoNotifications prevents forwarding message content to a closed channel.
	dispatch(n, &gateway.UserGuildSettingsUpdateEvent{UserGuildSetting: gateway.UserGuildSetting{ChannelOverrides: []gateway.UserChannelOverride{{ChannelID: dmGroup, Muted: true, Notifications: gateway.NoNotifications}}}})
	srv := socket.New(filepath.Join(t.TempDir(), "dm.sock"), m)
	if err := srv.Listen(); err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan struct{})
	go func() { defer close(done); srv.Serve(ctx) }()
	defer func() { cancel(); <-done }()
	c, err := net.Dial("unix", srv.Path())
	if err != nil {
		t.Fatal(err)
	}
	defer c.Close()
	c.SetDeadline(time.Now().Add(2 * time.Second))
	sc := bufio.NewScanner(c)
	scan := func() map[string]any {
		if !sc.Scan() {
			t.Fatalf("socket: %v", sc.Err())
		}
		var v map[string]any
		if err := json.Unmarshal(sc.Bytes(), &v); err != nil {
			t.Fatal(err)
		}
		return v
	}
	// Wait for a correlated ping after the account snapshot before dispatching.
	fmt.Fprintln(c, `{"v":1,"id":31,"command":"ping"}`)
	for {
		if v := scan(); v["id"] == float64(31) {
			break
		}
	}
	dispatch(n, &gateway.MessageCreateEvent{Message: discord.Message{ID: msgBase + 18, ChannelID: dmGroup, Author: discord.User{ID: adaID}, Timestamp: ts(18)}})
	deadline := time.After(time.Second)
	forwarded := false
	for !forwarded {
		select {
		case ev := <-m.Events():
			srv.Broadcast(ev)
			_, forwarded = ev.(protocol.ChannelUpdateEvent)
		case <-deadline:
			t.Fatal("no activity metadata")
		}
	}
	for {
		v := scan()
		if v["event"] == "message_create" {
			t.Fatal("muted closed DM leaked notification content")
		}
		if v["event"] == "channel_update" {
			break
		}
	}
	fmt.Fprintln(c, `{"v":1,"id":32,"command":"list_dms"}`)
	for {
		v := scan()
		if v["id"] != float64(32) {
			continue
		}
		result := v["result"].(map[string]any)
		chs := result["channels"].([]any)
		first := chs[0].(map[string]any)
		if first["id"] != discord.ChannelID(dmGroup).String() || first["last_message_id"] != (msgBase+18).String() || first["unread"] != "read" {
			t.Fatalf("refreshed muted list: %+v", first)
		}
		break
	}
}

func TestDMActivityPublishesClosedConversationMetadata(t *testing.T) {
	m, n := readyManager(t)
	drain(m)
	msg := discord.Message{ID: msgBase + 8, ChannelID: dmGroup, Author: discord.User{ID: adaID}, Content: "inert incoming", Timestamp: ts(8)}
	dispatch(n, &gateway.MessageCreateEvent{Message: msg})
	deadline := time.After(time.Second)
	for {
		select {
		case event := <-m.Events():
			if update, ok := event.(protocol.ChannelUpdateEvent); ok {
				if update.Channel.ID != discord.ChannelID(dmGroup).String() || update.Channel.LastMessageID == nil || *update.Channel.LastMessageID != msg.ID.String() || update.Channel.Unread == protocol.UnreadRead {
					t.Fatalf("DM activity metadata: %+v", update)
				}
				return
			}
		case <-deadline:
			t.Fatal("closed DM activity never published metadata to refresh the list")
		}
	}
}

func TestReadStateSnapshotDetached(t *testing.T) {
	_, n := readyManager(t)
	snapshot := n.ReadState.ReadState(dmGroup)
	if snapshot == nil {
		t.Fatal("missing seeded read state")
	}
	before := *snapshot
	dispatch(n, &gateway.MessageAckEvent{ChannelID: dmGroup, MessageID: msgBase + 21})
	if *snapshot != before {
		t.Fatal("returned snapshot changed after concurrent-owner update")
	}
	snapshot.MentionCount = 999
	current := n.ReadState.ReadState(dmGroup)
	if current.MentionCount == 999 || current.LastMessageID != msgBase+21 {
		t.Fatalf("snapshot mutated owner: %+v", current)
	}
}

func TestDMConcurrentReadSnapshotAndMetadata(t *testing.T) {
	m, n := readyManager(t)
	drain(m)
	start := make(chan struct{})
	var wg sync.WaitGroup
	// One ordered gateway writer overlaps two metadata/read-state consumers and
	// ningen's asynchronous UpdateEvent callbacks. No sleeps or event drains.
	wg.Add(3)
	go func() {
		defer wg.Done()
		<-start
		for i := 0; i < 120; i++ {
			id := msgBase + discord.MessageID(100+i)
			dispatch(n, &gateway.MessageCreateEvent{Message: discord.Message{ID: id, ChannelID: dmGroup, Author: discord.User{ID: adaID}, Timestamp: ts(1)}})
			dispatch(n, &gateway.MessageAckEvent{ChannelID: dmGroup, MessageID: id})
			runtime.Gosched()
		}
	}()
	for reader := 0; reader < 2; reader++ {
		go func() {
			defer wg.Done()
			<-start
			for i := 0; i < 300; i++ {
				if snapshot := n.ReadState.ReadState(dmGroup); snapshot != nil {
					_ = snapshot.LastMessageID
					_ = snapshot.MentionCount
				}
				DMs(n.Offline())
				UnreadDM(n.Offline())
				m.Snapshot()
				runtime.Gosched()
			}
		}()
	}
	close(start)
	wg.Wait()
	dms, err := DMs(n.Offline())
	if err != nil {
		t.Fatal(err)
	}
	if len(dms) == 0 || dms[0].ID != discord.ChannelID(dmGroup).String() || dms[0].Unread != "read" {
		t.Fatalf("final concurrent DM metadata: %+v", dms)
	}
}

func TestDMInvalidLastMessageSortsAfterRealActivity(t *testing.T) {
	_, n := readyManager(t)
	if err := n.Cabinet.ChannelSet(&discord.Channel{ID: 400000000000000097, Type: discord.DirectMessage, DMRecipients: []discord.User{{ID: linID}}, LastMessageID: discord.MessageID(discord.NullSnowflake)}, false); err != nil {
		t.Fatal(err)
	}
	dms, err := DMs(n.Offline())
	if err != nil {
		t.Fatal(err)
	}
	if dms[len(dms)-1].ID != "400000000000000097" {
		t.Fatalf("invalid/null last message obscured recent DMs: %+v", dms)
	}
}

func TestFirstDMWithoutReadBoundaryStaysDiscoverable(t *testing.T) {
	m, n := readyManager(t)
	const id discord.ChannelID = 400000000000000099
	dispatch(n, &gateway.ChannelCreateEvent{Channel: discord.Channel{ID: id, Type: discord.DirectMessage, DMRecipients: []discord.User{{ID: linID, Username: "lin"}}}})
	drain(m)
	dispatch(n, &gateway.MessageCreateEvent{Message: discord.Message{ID: msgBase + 9, ChannelID: id, Author: discord.User{ID: linID}, Content: "first incoming", Timestamp: ts(9)}})
	dms, err := DMs(n.Offline())
	if err != nil {
		t.Fatal(err)
	}
	if len(dms) == 0 || dms[0].ID != id.String() || dms[0].Unread == protocol.UnreadRead {
		t.Fatalf("first incoming DM hidden: %+v", dms)
	}
	if got := UnreadDM(n.Offline()); got == nil || *got != id.String() {
		t.Fatalf("middle-click target missing: %v", got)
	}
	// Do not drain: the incoming update and remote read may overlap.
	dispatch(n, &gateway.MessageAckEvent{ChannelID: id, MessageID: msgBase + 9})
	dms, _ = DMs(n.Offline())
	if dms[0].Unread != protocol.UnreadRead {
		t.Fatalf("confirmed remote read remained unread: %+v", dms[0])
	}
	const ownID discord.ChannelID = dmGroup
	dispatch(n, &gateway.MessageCreateEvent{Message: discord.Message{ID: msgBase + 10, ChannelID: ownID, Author: discord.User{ID: selfID}, Content: "own", Timestamp: ts(10)}})
	dms, _ = DMs(n.Offline())
	if dms[0].Unread != protocol.UnreadRead {
		t.Fatalf("own first DM marked unread: %+v", dms[0])
	}
}
