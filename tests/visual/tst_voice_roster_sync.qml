import QtQuick
import QtTest
import "../../components" as Components
import "../../ServerList.js" as ServerList

TestCase {
  id: test
  name: "VoiceRosterSync"
  when: windowShown
  property var requests: []
  property var replies: []
  QtObject {
    id: service
    property bool connected: true
    property string lifecycle: "logged_out"
    property var guilds: [{id:"1"}, {id:"2"}]
    property var voiceMembers: ({})
    property string selectedGuildId: "1"
    property var backend: transport
    function applyVoiceMembers(message) {
      var next = Object.assign({}, voiceMembers)
      next[message.guild_id] = message.channels
      voiceMembers = next
    }
  }
  QtObject {
    id: transport
    signal eventReceived(string name, var message)
    function sendCommand(name, fields, callback) {
      test.requests.push({name:name, id:fields.guild_id})
      test.replies.push(callback)
    }
  }
  Components.VoiceRosterSync { id: sync; service: service }
  function init() {
    service.lifecycle = "logged_out"
    sync.cancel(); sync.active = false
    service.voiceMembers = ({})
    requests = []; replies = []
    service.lifecycle = "ready"
    tryVerify(function() { return replies.length === 1 })
  }
  function roster(id, populated) {
    return {guild_id:id, channels:populated ? [{channel_id:"10", users:[{id:"20"}]}] : []}
  }
  function settle() {
    replies.shift()(true, roster("1", false))
    tryVerify(function() { return replies.length === 1 })
    replies.shift()(true, roster("2", true))
    tryCompare(sync, "busy", false)
  }
  function test_unvisited_guild_updates_count_without_navigation() {
    settle()
    compare(ServerList.voiceGuildsCount(service.guilds, {}, service.voiceMembers), 1)
    compare(service.selectedGuildId, "1")
    compare(requests.length, 2)
    for (var i=0; i<requests.length; i++) compare(requests[i].name, "voice_members")
    sync.active = true
    tryVerify(function() { return replies.length === 1 })
    replies.shift()(true, roster("1", false))
    tryVerify(function() { return replies.length === 1 })
    replies.shift()(true, roster("2", false))
    tryCompare(sync, "busy", false)
    compare(ServerList.voiceGuildsCount(service.guilds, {}, service.voiceMembers), 0)
  }
  function test_timer_refreshes_all_guilds_without_navigation() {
    settle(); sync.active = true
    tryVerify(function() { return replies.length === 1 }); settle()
    var timer = findChild(sync, "voice-roster-timer")
    verify(timer.running); timer.interval = 20
    tryVerify(function() { return replies.length === 1 })
    timer.interval = 15000
    settle(); compare(service.selectedGuildId, "1")
    sync.active = false; verify(!timer.running)
  }
  function test_live_event_wins_over_delayed_snapshot() {
    var event = roster("1", true)
    service.applyVoiceMembers(event); transport.eventReceived("voice_members", event)
    replies.shift()(true, roster("1", false))
    tryVerify(function() { return replies.length === 1 })
    replies.shift()(true, roster("2", false))
    tryCompare(sync, "busy", false)
    compare(ServerList.voiceGuildsCount(service.guilds, {}, service.voiceMembers), 1)
  }
  function test_disconnect_cancels_old_callback_and_timeout_unlocks() {
    var stale = replies.shift()
    service.connected = false; compare(sync.busy, false)
    service.voiceMembers = ({})
    stale(true, roster("1", true)); compare(Object.keys(service.voiceMembers).length, 0)
    service.connected = true
    tryVerify(function() { return replies.length === 1 })
    var deadline = findChild(sync, "voice-roster-deadline")
    deadline.interval = 10
    tryCompare(sync, "busy", false)
    replies.shift()(true, roster("1", true)); compare(Object.keys(service.voiceMembers).length, 0)
    deadline.interval = 15000
  }
}
