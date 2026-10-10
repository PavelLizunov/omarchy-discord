package session

import (
	"context"
	"testing"

	dsnowflake "github.com/disgoorg/snowflake/v2"
	"github.com/mattcalayo/omarchy-discord/backend/internal/protocol"
	"github.com/mattcalayo/omarchy-discord/backend/internal/voice"
)

type participantVoice struct {
	fakeVoice
	levels map[string]voice.UserAudio
	sets   int
}

func (v *participantVoice) UserLevels() map[string]voice.UserAudio { return v.levels }
func (v *participantVoice) SetUserAudio(id dsnowflake.ID, vol *int, mute *bool) error {
	v.sets++
	r := voice.UserAudio{Volume: 100}
	if vol != nil {
		r.Volume = *vol
	}
	if mute != nil {
		r.Muted = *mute
	}
	v.levels = map[string]voice.UserAudio{id.String(): r}
	return nil
}
func TestParticipantCommandsValidateCurrentRoom(t *testing.T) {
	m, n, _ := voiceManager(t)
	v := &participantVoice{}
	v.state = voice.State{Status: voice.StatusConnected, GuildID: guildOmar, ChannelID: chVoice}
	m.voice = v
	run := func(fields string) (any, *protocol.Error) {
		return m.Handle(context.Background(), req(t, `{"v":1,"id":88,"command":"voice_user_set",`+fields+`}`))
	}
	if _, e := run(`"user_id":"100000000000000002","volume":50`); e == nil {
		t.Fatal("nonparticipant accepted")
	}
	dispatch(n, voiceStateEvent(chVoice, ada))
	nextVoiceMembers(t, m)
	r, e := run(`"user_id":"100000000000000002","volume":50,"muted":true`)
	if e != nil || r.(map[string]voice.UserAudio)["100000000000000002"].Volume != 50 {
		t.Fatal(r, e)
	}
	for _, fields := range []string{`"user_id":"100000000000000002","volume":201`, `"user_id":"100000000000000002","volume":1.5`, `"user_id":"bad","volume":50`, `"user_id":"100000000000000002"`} {
		if _, e := run(fields); e == nil {
			t.Fatal("invalid accepted", fields)
		}
	}
	if v.sets != 1 || v.took() != "" {
		t.Fatal("local controls changed session", v.sets, v.took())
	}
}
