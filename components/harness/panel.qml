pragma ComponentBehavior: Bound
import QtQuick
import Quickshell

import "Keymap.js" as Keymap

ShellRoot {
  id: harness

  property int failures: 0
  property int checks: 0

  function check(name, actual, expected) {
    checks++
    var a = JSON.stringify(actual)
    var e = JSON.stringify(expected)
    if (a === e) { console.log("  ok   " + name); return }
    failures++
    console.log("  FAIL " + name + ": expected " + e + ", got " + a)
  }

  function press(name, modifiers) {
    var keys = {
      Escape: Qt.Key_Escape, Return: Qt.Key_Return, Tab: Qt.Key_Tab,
      Backtab: Qt.Key_Backtab, Up: Qt.Key_Up, Down: Qt.Key_Down,
      Left: Qt.Key_Left, Right: Qt.Key_Right
    }
    var event = {
      key: keys[name] !== undefined ? keys[name] : 0,
      text: keys[name] !== undefined ? "" : name,
      modifiers: modifiers || Qt.NoModifier,
      accepted: false
    }
    panel.dispatchKey(event)
    return event.accepted
  }

  function chord(key, modifiers) {
    var event = { key: key, text: "", modifiers: modifiers, accepted: false }
    panel.dispatchKey(event)
    return event.accepted
  }

  function fixtures() {
    mock.guilds = [
      { id: "g1", name: "First", kind: "guild" },
      { id: "g2", name: "Second", kind: "guild" }
    ]
    mock.dms = [{ id: "d1", name: "ada", type: "dm" }]
    mock.voiceMembers = {
      g1: [{ channel_id: "c-voice", users: [
        { id: "u1", username: "ada", display_name: "Ada Lovelace", avatar_url: "" },
        { id: "u2", username: "lin", display_name: "Lin", avatar_url: "" }
      ] }]
    }
    mock.speaking = { u1: true }
    mock.channelsByGuild = {
      g1: [
        { id: "c-cat", name: "Text", type: "category" },
        { id: "c-voice", name: "Lounge", type: "voice" },
        { id: "c-general", name: "general", type: "text" },
        { id: "c-random", name: "random", type: "text" }
      ],
      g2: [
        { id: "c-news", name: "announcements", type: "announcement" },
        { id: "c-chat", name: "chat", type: "text" }
      ]
    }
  }

  function runPanelChecks() {
    console.log("Panel.qml — window lifecycle")
    mock.reset()
    panel.close()
    check("close() reports the panel shut", panel.opened, false)
    check("close() told the service nobody is looking",
      mock.lastCall("setUiVisible"), "full-panel=false")
    panel.open("{}")
    check("open() reports the panel open", panel.opened, true)
    check("open() told the service someone is looking",
      mock.lastCall("setUiVisible"), "full-panel=true")

    console.log("Panel.qml — Esc ladder (CHANGE C)")
    panel.zone = "sidebar"
    panel.column = "channels"
    check("Esc in the channel list is consumed", press("Escape"), true)
    check("Esc leaves the channel list for the rail", panel.column, "rail")
    check("the panel is still open", panel.opened, true)
    check("Esc on the rail is consumed", press("Escape"), true)
    check("Esc on the rail does not close the panel", panel.opened, true)
    check("Esc on the rail stays on the rail", panel.column, "rail")

    console.log("Panel.qml — entering a guild (CHANGE B)")
    mock.reset()
    panel.zone = "sidebar"
    panel.column = "rail"
    panel.guildCursorId = "g2"
    press("j")
    press("k")
    check("moving the rail cursor opens nothing", mock.callCount("enterGuild"), 0)
    check("Enter on the rail is consumed", press("Return"), true)
    check("Enter enters the cursor's guild", mock.lastCall("enterGuild"), "g2")
    check("Enter moves to the channel column", panel.column, "channels")

    mock.reset()
    panel.column = "rail"
    panel.guildCursorId = "g1"
    press("l")
    check("l enters the guild too", mock.lastCall("enterGuild"), "g1")

    mock.reset()
    mock.selectedGuildId = "g1"
    mock.currentChannelId = "c-random"
    panel.column = "rail"
    panel.guildCursorId = "g1"
    press("Return")
    check("re-entering the open channel's guild opens nothing", mock.callCount("enterGuild"), 0)
    mock.currentChannelId = ""

    console.log("Panel.qml — voice channels")
    mock.reset()
    mock.voice = { status: "idle", guildId: "", channelId: "", muted: false, deafened: false, error: "" }
    mock.selectedGuildId = "g1"
    panel.zone = "sidebar"
    panel.column = "channels"
    check("a voice channel is in the channel list",
      panel.indexOfId(panel.channelRows, "c-voice") >= 0, true)
    panel.channelCursorId = "c-voice"
    check("the sidebar cursor lands on a voice row", panel.channelCursor >= 0, true)
    check("Enter on a voice row is consumed", press("Return"), true)
    check("Enter on a voice row joins it", mock.lastCall("voiceJoin"), "c-voice")
    check("and never opens it as a channel", mock.callCount("showChannel"), 0)

    mock.reset()
    mock.voice = { status: "connected", guildId: "g1", channelId: "c-voice",
      muted: false, deafened: false, error: "" }
    panel.zone = "sidebar"
    panel.column = "channels"
    panel.channelCursorId = "c-voice"
    press("Return")
    check("Enter on the joined channel does not re-join", mock.callCount("voiceJoin"), 0)
    check("Enter on the joined channel focuses the call bar", panel.callBarFocused, true)
    check("the call bar is a button-style stop, not a zone", panel.focusedZone, "")
    check("Esc leaves the call bar", press("Escape"), true)
    check("and the call bar no longer has focus", panel.callBarFocused, false)

    mock.reset()
    mock.voice = { status: "error", guildId: "g1", channelId: "c-voice",
      muted: false, deafened: false, error: "no audio device" }
    panel.zone = "sidebar"
    panel.column = "channels"
    panel.channelCursorId = "c-voice"
    press("Return")
    check("Enter on a failed call retries the join", mock.lastCall("voiceJoin"), "c-voice")
    check("and does not park on the call bar", panel.callBarFocused, false)

    mock.reset()
    mock.currentChannelId = "c-general"
    mock.voice = { status: "connected", guildId: "g1", channelId: "c-voice",
      muted: false, deafened: false, error: "" }
    panel.zone = "timeline"
    panel.focusZone()
    panel.focusCallBar()
    check("the call bar takes focus from the timeline", panel.callBarFocused, true)
    check("Ctrl+Shift+H from the bar hangs up", chord(Qt.Key_H, Qt.ControlModifier | Qt.ShiftModifier), true)
    mock.voice = { status: "idle", guildId: "", channelId: "", muted: false, deafened: false, error: "" }
    check("the bar going away hands the keyboard back to a live item",
      !!panel.controls.Window.activeFocusItem && panel.controls.Window.activeFocusItem.visible, true)
    check("and the zone is focused again", panel.focusedZone, "timeline")
    mock.currentChannelId = ""

    mock.reset()
    check("Ctrl+Shift+M is consumed", chord(Qt.Key_M, Qt.ControlModifier | Qt.ShiftModifier), true)
    check("Ctrl+Shift+M toggles the mic", mock.callCount("toggleMute"), 1)
    chord(Qt.Key_D, Qt.ControlModifier | Qt.ShiftModifier)
    check("Ctrl+Shift+D toggles deafen", mock.callCount("toggleDeafen"), 1)
    chord(Qt.Key_H, Qt.ControlModifier | Qt.ShiftModifier)
    check("Ctrl+Shift+H hangs up", mock.callCount("voiceLeave"), 1)
    mock.voice = { status: "idle", guildId: "", channelId: "", muted: false, deafened: false, error: "" }
    check("the call bar is gone with the call", panel.callBarFocused, false)

    console.log("Keymap.js — one key table")
    check("every footer hint resolves to an entry", Keymap.missingFooterIds(), [])

    console.log("Panel.qml — one controls row, two hosts (CHANGE A)")
    mock.reset()
    panel.zone = "sidebar"
    panel.column = "rail"
    var readyHost = panel.controls.parent
    check("the host reserves the whole controls row",
      panel.controls.parent.height >= panel.controls.height, true)
    var reached = false
    for (var i = 0; i < 12 && !reached; i++) { press("Tab"); reached = panel.buttonFocused }
    check("Tab reaches the panel controls while ready", reached, true)

    mock.currentChannelId = "c-general"
    mock.membersWanted = true
    check("the member pane never squeezes the channel name out",
      !panel.controlsInHeader
        || panel.controls.width <= panel.channelHeaderRoom - panel.channelTitleFloor + 0.5,
      true)
    check("the controls are hosted either way",
      panel.controls.parent !== null && panel.controls.visible, true)
    mock.membersWanted = false
    mock.currentChannelId = ""

    mock.showStructure = false
    check("the client is down", panel.ready, false)
    check("the controls moved to the other host", panel.controls.parent !== readyHost, true)
    check("and are still on screen", panel.controls.parent !== null && panel.controls.visible, true)
    reached = false
    for (var j = 0; j < 12 && !reached; j++) { press("Tab"); reached = panel.buttonFocused }
    check("Tab still reaches the controls on the status screen", reached, true)
    check("Esc on the status screen still closes", press("Escape"), true)
    check("the status screen's Esc closed the panel", panel.opened, false)
    mock.showStructure = true
  }

  function runServiceChecks() {
    console.log("Service.qml — guild entry (CHANGE B)")
    service.setUiVisible("full-panel", true)
    service.channelsByGuild = mock.channelsByGuild
    service.dms = mock.dms

    service.currentChannelId = ""
    service.lastChannels = [{ g: "g1", c: "c-random" }]
    service.selectedGuildId = "g1"
    service.enterGuild("g1")
    check("the remembered channel opens", service.currentChannelId, "c-random")

    service.currentChannelId = ""
    service.lastChannels = []
    service.selectedGuildId = "g1"
    service.enterGuild("g1")
    check("no memory falls back to general", service.currentChannelId, "c-general")

    service.currentChannelId = ""
    service.lastChannels = [{ g: "g2", c: "c-gone" }]
    service.selectedGuildId = "g2"
    service.enterGuild("g2")
    check("a stale memory falls back to the first channel", service.currentChannelId, "c-news")

    service.currentChannelId = ""
    service.lastChannels = [{ g: "g1", c: "c-voice" }]
    service.selectedGuildId = "g1"
    service.enterGuild("g1")
    check("an unopenable memory falls back too", service.currentChannelId, "c-general")

    service.currentChannelId = ""
    service.lastChannels = [{ g: "g3", c: "c-late" }]
    service.channelsLoading = { g3: true }
    service.selectedGuildId = "g3"
    service.enterGuild("g3")
    check("a loading guild parks the entry", service.pendingGuildEntry, "g3")
    check("and opens nothing yet", service.currentChannelId, "")
    var next = Object.assign({}, service.channelsByGuild)
    next["g3"] = [{ id: "c-late", name: "late", type: "text" }]
    service.channelsByGuild = next
    service.channelsLoading = ({})
    service.resolveGuildEntry()
    check("the response resolves the parked entry", service.currentChannelId, "c-late")
    check("and unparks it", service.pendingGuildEntry, "")

    service.currentChannelId = ""
    service.channelsLoading = { g4: true }
    service.selectedGuildId = "g4"
    service.enterGuild("g4")
    service.selectedGuildId = "g1"
    service.resolveGuildEntry()
    check("navigating away drops the parked entry", service.pendingGuildEntry, "")
    check("and opens nothing", service.currentChannelId, "")

    service.setUiVisible("full-panel", false)
    service.selectedGuildId = "g1"
    service.lastChannels = [{ g: "g1", c: "c-random" }]
    service.enterGuild("g1")
    check("a closed panel opens nothing", service.currentChannelId, "")
    service.setUiVisible("full-panel", true)

    service.currentChannelId = ""
    service.lastChannels = []
    service.selectedGuildId = "dms"
    service.enterGuild("dms")
    check("DMs with nothing remembered open nothing", service.currentChannelId, "")
    service.lastChannels = [{ g: "dms", c: "d1" }]
    service.enterGuild("dms")
    check("DMs open the remembered conversation", service.currentChannelId, "d1")

    service.channelsLoading = { g5: true }
    service.selectedGuildId = "g5"
    service.enterGuild("g5")
    service.clearMessages()
    check("clearMessages drops the parked entry", service.pendingGuildEntry, "")

    console.log("Service.qml — recording a visit")
    service.lastChannels = []
    service.noteChannelVisit("g1", "c-random")
    check("a visit is recorded", service.lastChannels, [{ g: "g1", c: "c-random" }])
    service.noteChannelVisit("g1", "c-random")
    check("re-opening the same channel writes nothing", service.lastChannels, [{ g: "g1", c: "c-random" }])
    service.noteChannelVisit("g2", "c-chat")
    service.noteChannelVisit("g1", "c-general")
    check("the newest guild is first, one entry per guild", service.lastChannels,
      [{ g: "g1", c: "c-general" }, { g: "g2", c: "c-chat" }])

    console.log("Service.qml — voice state and events")
    service.applyState({ lifecycle: "connecting", generation: 500,
      voice: { status: "connected", guild_id: "g1", channel_id: "c-voice",
        muted: true, deafened: false, error: "" } })
    check("the call rides state_changed", service.voice.status, "connected")
    check("the joined channel is mirrored", service.voice.channelId, "c-voice")
    check("self-mute is mirrored", service.voice.muted, true)
    check("the call is not idle", service.inCall, true)

    service.handleEvent("voice_members", { guild_id: "g1", channels: [
      { channel_id: "c-voice", users: [{ id: "u1", username: "ada", display_name: "Ada", avatar_url: "" }] }
    ] })
    check("voice_members fills the occupants", service.voiceUsers("g1", "c-voice").length, 1)
    check("the occupant carries its own name",
      service.voiceUsers("g1", "c-voice")[0].display_name, "Ada")
    check("an empty channel has none", service.voiceUsers("g1", "c-general").length, 0)
    check("an unknown guild has none", service.voiceUsers("g9", "c-voice").length, 0)

    service.handleEvent("voice_members", { guild_id: "g2", channels: [
      { channel_id: "c-chat", users: [{ id: "u2", username: "lin", display_name: "Lin", avatar_url: "" }] }
    ] })
    check("a second guild's occupants join the first's",
      [service.voiceUsers("g1", "c-voice").length, service.voiceUsers("g2", "c-chat").length], [1, 1])

    service.handleEvent("voice_speaking", { user_id: "u1", speaking: true })
    check("voice_speaking marks the speaker", service.speaking["u1"] === true, true)
    service.handleEvent("voice_speaking", { user_id: "u1", speaking: false })
    check("and unmarks it", service.speaking["u1"] === undefined, true)

    service.handleEvent("voice_speaking", { user_id: "u1", speaking: true })
    service.applyState({ lifecycle: "connecting", generation: 501,
      voice: { status: "idle", guild_id: null, channel_id: null,
        muted: false, deafened: false, error: "" } })
    check("leaving the call clears speaking", Object.keys(service.speaking).length, 0)
    check("and the call is idle again", service.voice.status, "idle")
    check("a state without a voice object reads as idle", service.inCall, false)
  }

  function run() {
    fixtures()
    panel.open("{}")
    runPanelChecks()
    runServiceChecks()
    console.log(failures === 0
      ? "HARNESS PASS (" + checks + " checks)"
      : "HARNESS FAIL (" + failures + "/" + checks + " checks)")
    Qt.exit(failures === 0 ? 0 : 1)
  }

  MockService { id: mock }

  Service {
    id: service
    shell: null
    manifest: ({ id: "quickshell.discord" })
  }

  Panel {
    id: panel
    shell: null
    manifest: ({ id: "quickshell.discord" })
    service: mock
  }

  Timer {
    interval: 400
    running: true
    onTriggered: harness.run()
  }
}
