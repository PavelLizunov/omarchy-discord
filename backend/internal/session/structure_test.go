package session

import (
	"encoding/json"
	"flag"
	"os"
	"path/filepath"
	"testing"

	"github.com/diamondburned/arikawa/v3/discord"
	"github.com/diamondburned/arikawa/v3/gateway"
	"github.com/diamondburned/arikawa/v3/state"
	"github.com/diamondburned/ningen/v3"
)

var update = flag.Bool("update", false, "rewrite testdata/ready.json from the synthetic builder")

const (
	selfID     = 100000000000000001
	adaID      = 100000000000000002
	linID      = 100000000000000003
	guildOmar  = 200000000000000001
	guildQuiet = 200000000000000002
	catGeneral = 300000000000000001
	chGeneral  = 300000000000000002
	chDev      = 300000000000000003
	chVoice    = 300000000000000004
	chSecret   = 300000000000000005
	catEmpty   = 300000000000000006
	chLoose    = 300000000000000007
	chQuiet    = 300000000000000008
	dmAda      = 400000000000000001
	dmGroup    = 400000000000000002
)

type readyFixture struct {
	gateway.ReadyEvent
	UserSettings      *gateway.UserSettings          `json:"user_settings"`
	ReadStates        []gateway.ReadState            `json:"read_state"`
	UserGuildSettings []gateway.UserGuildSetting     `json:"user_guild_settings"`
	MergedMembers     [][]gateway.SupplementalMember `json:"merged_members"`
}

func buildReady() readyFixture {
	text := func(id, parent discord.ChannelID, name string, pos int, last discord.MessageID, ow ...discord.Overwrite) discord.Channel {
		return discord.Channel{ID: id, GuildID: guildOmar, Type: discord.GuildText, Name: name, Position: pos, ParentID: parent, LastMessageID: last, Overwrites: ow}
	}
	everyone := discord.Role{ID: discord.RoleID(guildOmar), Name: "@everyone", Permissions: discord.PermissionViewChannel | discord.PermissionSendMessages | discord.PermissionConnect}
	quietEveryone := discord.Role{ID: discord.RoleID(guildQuiet), Name: "@everyone", Permissions: discord.PermissionViewChannel}
	self := discord.User{ID: selfID, Username: "tester", DisplayName: "Tester"}
	ada := discord.User{ID: adaID, Username: "ada", DisplayName: "Ada", Avatar: "aaaa"}
	lin := discord.User{ID: linID, Username: "lin"}

	var r readyFixture
	r.Version = 9
	r.User = self
	r.SessionID = "synthetic-session"
	r.Guilds = []gateway.GuildCreateEvent{
		{
			Guild: discord.Guild{ID: guildOmar, Name: "Omarchy", Icon: "iconhash", OwnerID: adaID, Roles: []discord.Role{everyone}},
			Channels: []discord.Channel{
				{ID: catGeneral, GuildID: guildOmar, Type: discord.GuildCategory, Name: "General", Position: 1},
				text(chDev, catGeneral, "dev", 1, 500000000000000020),
				text(chGeneral, catGeneral, "general", 0, 500000000000000010),
				{ID: chVoice, GuildID: guildOmar, Type: discord.GuildVoice, Name: "Voice", Position: 0, ParentID: catGeneral},
				text(chSecret, catGeneral, "secret", 2, 500000000000000030, discord.Overwrite{ID: discord.Snowflake(guildOmar), Type: discord.OverwriteRole, Deny: discord.PermissionViewChannel}),
				{ID: catEmpty, GuildID: guildOmar, Type: discord.GuildCategory, Name: "Empty", Position: 0},
				text(chLoose, 0, "loose", 5, 500000000000000040),
			},
		},
		{
			Guild: discord.Guild{ID: guildQuiet, Name: "Quiet", OwnerID: selfID, Roles: []discord.Role{quietEveryone}},
			Channels: []discord.Channel{
				{ID: chQuiet, GuildID: guildQuiet, Type: discord.GuildText, Name: "chat", LastMessageID: 500000000000000050},
			},
		},
	}
	r.MergedMembers = [][]gateway.SupplementalMember{
		{{UserID: selfID, RoleIDs: []discord.RoleID{}}},
		{{UserID: selfID, RoleIDs: []discord.RoleID{}}},
	}
	r.PrivateChannels = []discord.Channel{
		{ID: dmGroup, Type: discord.GroupDM, DMRecipients: []discord.User{ada, lin}, LastMessageID: 500000000000000060},
		{ID: dmAda, Type: discord.DirectMessage, DMRecipients: []discord.User{ada}, LastMessageID: 500000000000000070},
	}
	r.UserSettings = &gateway.UserSettings{
		Status: discord.IdleStatus,
		GuildFolders: []gateway.GuildFolder{
			{GuildIDs: []discord.GuildID{guildQuiet}},
			{GuildIDs: []discord.GuildID{guildOmar}},
		},
	}
	r.ReadStates = []gateway.ReadState{
		{ChannelID: chGeneral, LastMessageID: 500000000000000010, MentionCount: 0},
		{ChannelID: chDev, LastMessageID: 500000000000000019, MentionCount: 2},
		{ChannelID: chLoose, LastMessageID: 500000000000000039, MentionCount: 0},
		{ChannelID: chQuiet, LastMessageID: 500000000000000049, MentionCount: 0},
		{ChannelID: dmAda, LastMessageID: 500000000000000069, MentionCount: 1},
		{ChannelID: dmGroup, LastMessageID: 500000000000000060, MentionCount: 0},
	}
	r.UserGuildSettings = []gateway.UserGuildSetting{
		{GuildID: guildQuiet, Muted: true},
		{GuildID: guildOmar, ChannelOverrides: []gateway.UserChannelOverride{{ChannelID: chLoose, Muted: false}}},
	}
	return r
}

func loadOfflineState(t *testing.T) *ningen.State {
	t.Helper()
	n, ready := newUnopenedState(t)
	dispatch(n, ready)
	seedSelfMembers(n, ready)
	return n.Offline()
}

func dispatch(n *ningen.State, ev any) {
	n.State.Session.Handler.Call(ev)
}

func newUnopenedState(t *testing.T) (*ningen.State, *gateway.ReadyEvent) {
	t.Helper()
	path := filepath.Join("testdata", "ready.json")
	if *update {
		b, err := json.MarshalIndent(buildReady(), "", "  ")
		if err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(path, append(b, '\n'), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	raw, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("%v (run with -update to regenerate)", err)
	}
	var ready gateway.ReadyEvent
	if err := json.Unmarshal(raw, &ready); err != nil {
		t.Fatal(err)
	}
	if len(ready.RawEventBody) == 0 {
		t.Fatal("ReadyEventKeepRaw not in effect; ningen init() should force it")
	}
	st := state.NewWithIdentifier(gateway.DefaultIdentifier("offline-fixture-token"))
	return ningen.FromState(st), &ready
}

func TestGuildsFromFixture(t *testing.T) {
	n := loadOfflineState(t)
	gs, err := Guilds(n)
	if err != nil {
		t.Fatal(err)
	}
	if len(gs) != 2 {
		t.Fatalf("guilds: %+v", gs)
	}
	if gs[0].Name != "Quiet" || gs[0].Position != 0 || gs[1].Name != "Omarchy" || gs[1].Position != 1 {
		t.Fatalf("order: %+v", gs)
	}
	quiet, omar := gs[0], gs[1]
	if quiet.Unread != "read" || quiet.MentionCount != 0 || quiet.IconURL != nil {
		t.Errorf("muted guild with plain unreads must read as read: %+v", quiet)
	}
	if omar.Unread != "mentioned" || omar.MentionCount != 2 {
		t.Errorf("omarchy: %+v", omar)
	}
	if omar.IconURL == nil || *omar.IconURL == "" {
		t.Errorf("icon url missing: %+v", omar)
	}
	if omar.ID != "200000000000000001" {
		t.Errorf("snowflake must be a string: %q", omar.ID)
	}
}

func TestChannelsFromFixture(t *testing.T) {
	n := loadOfflineState(t)
	chs, err := Channels(n, guildOmar)
	if err != nil {
		t.Fatal(err)
	}
	var names []string
	byName := map[string]int{}
	for i, c := range chs {
		names = append(names, c.Name)
		byName[c.Name] = i
	}
	want := []string{"loose", "General", "general", "dev", "Voice"}
	if len(names) != len(want) {
		t.Fatalf("channels %v, want %v", names, want)
	}
	for i := range want {
		if names[i] != want[i] {
			t.Fatalf("order %v, want %v", names, want)
		}
	}
	if _, ok := byName["secret"]; ok {
		t.Error("secret channel should be permission-filtered")
	}
	if _, ok := byName["Empty"]; ok {
		t.Error("empty category should be removed")
	}
	check := func(name, typ, unread string, mentions int, parent bool) {
		c := chs[byName[name]]
		if c.Type != typ || c.Unread != unread || c.MentionCount != mentions || (c.ParentID != nil) != parent {
			t.Errorf("%s: %+v", name, c)
		}
		if c.GuildID == nil || *c.GuildID != "200000000000000001" {
			t.Errorf("%s guild_id: %v", name, c.GuildID)
		}
	}
	check("general", "text", "read", 0, true)
	check("dev", "text", "mentioned", 2, true)
	check("Voice", "voice", "read", 0, true)
	check("loose", "text", "unread", 0, false)
	check("General", "category", "read", 0, false)
	if c := chs[byName["general"]]; c.LastMessageID == nil || *c.LastMessageID != "500000000000000010" {
		t.Errorf("last_message_id: %+v", c)
	}
	if c := chs[byName["General"]]; c.LastMessageID != nil {
		t.Errorf("category last_message_id should be null: %+v", c)
	}

	if _, err := Channels(n, 999); err != ErrUnknownGuild {
		t.Errorf("unknown guild: %v", err)
	}
}

func TestDMsFromFixture(t *testing.T) {
	n := loadOfflineState(t)
	dms, err := DMs(n)
	if err != nil {
		t.Fatal(err)
	}
	if len(dms) != 2 {
		t.Fatalf("%+v", dms)
	}
	ada, group := dms[0], dms[1]
	if ada.Type != "dm" || ada.Name != "Ada" || ada.Unread != "mentioned" || ada.MentionCount != 1 || ada.GuildID != nil {
		t.Errorf("ada: %+v", ada)
	}
	if len(ada.Recipients) != 1 || ada.Recipients[0].Username != "ada" || ada.Recipients[0].AvatarURL == "" {
		t.Errorf("ada recipients: %+v", ada.Recipients)
	}
	if group.Type != "group_dm" || group.Name != "Ada, lin" || group.Unread != "read" || len(group.Recipients) != 2 {
		t.Errorf("group: %+v", group)
	}
	if id := UnreadDM(n); id == nil || *id != "400000000000000001" {
		t.Errorf("unread dm: %v", id)
	}
	if got := n.ReadState.TotalMentionCount(); got != 3 {
		t.Errorf("total mentions %d, want 3", got)
	}
}
