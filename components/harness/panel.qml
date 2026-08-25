pragma ComponentBehavior: Bound
import QtQuick
import Quickshell

// Offscreen contract harness for the panel's keyboard and guild-entry
// behaviour. `components/harness/run-panel.sh` builds a scratch config root of
// symlinks next to the shell's Commons/Ui, points XDG_RUNTIME_DIR at a scratch
// directory (so the mounted Service can never reach the live backend socket)
// and runs this with QT_QPA_PLATFORM=offscreen. Nothing is injected into the
// Wayland session; every key is a synthesized event through Panel.dispatchKey.
//
// Two halves:
//   * Panel.qml against MockService — the Esc ladder and what entering a guild
//     asks the service to do.
//   * the real Service.qml with no socket — enterGuild / resolveGuildEntry:
//     remembered channel, the general and first-channel fallbacks, the parked
//     retry, and the stale guards.
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

  // A guild with a "general", one without, and a DM list.
  function fixtures() {
    mock.guilds = [
      { id: "g1", name: "First", kind: "guild" },
      { id: "g2", name: "Second", kind: "guild" }
    ]
    mock.dms = [{ id: "d1", name: "ada", type: "dm" }]
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

  // --- Panel.qml + MockService ---
  function runPanelChecks() {
    // open() / close() route through enter() / leave() via onOpenedChanged;
    // with no `persistentWindow` on the mock the panel is in On demand mode,
    // so `opened` is still the host-driven flag.
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

    // The guild that already owns the open channel: re-entering it must not
    // disturb the timeline.
    mock.reset()
    mock.selectedGuildId = "g1"
    mock.currentChannelId = "c-random"
    panel.column = "rail"
    panel.guildCursorId = "g1"
    press("Return")
    check("re-entering the open channel's guild opens nothing", mock.callCount("enterGuild"), 0)
    mock.currentChannelId = ""

    console.log("Panel.qml — one controls row, two hosts (CHANGE A)")
    mock.reset()
    panel.zone = "sidebar"
    panel.column = "rail"
    var readyHost = panel.controls.parent
    // The host must reserve the row's full height, or the focus ring's bottom
    // edge lands on the Timeline pane's border.
    check("the host reserves the whole controls row",
      panel.controls.parent.height >= panel.controls.height, true)
    var reached = false
    for (var i = 0; i < 12 && !reached; i++) { press("Tab"); reached = panel.buttonFocused }
    check("Tab reaches the panel controls while ready", reached, true)

    // The channel-title row hosts the controls only while it can still show a
    // channel name. Squeezing them in regardless pushes the row off the left
    // edge of the timeline column, over the channel list and the rail.
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

  // --- the real Service.qml, no socket ---
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

    // Channels still loading: the intent parks and the response resolves it.
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

    // A guild the user has navigated away from must never fire.
    service.currentChannelId = ""
    service.channelsLoading = { g4: true }
    service.selectedGuildId = "g4"
    service.enterGuild("g4")
    service.selectedGuildId = "g1"
    service.resolveGuildEntry()
    check("navigating away drops the parked entry", service.pendingGuildEntry, "")
    check("and opens nothing", service.currentChannelId, "")

    // A closed panel must never have a channel opened behind it.
    service.setUiVisible("full-panel", false)
    service.selectedGuildId = "g1"
    service.lastChannels = [{ g: "g1", c: "c-random" }]
    service.enterGuild("g1")
    check("a closed panel opens nothing", service.currentChannelId, "")
    service.setUiVisible("full-panel", true)

    // DMs: remembered only, never a default.
    service.currentChannelId = ""
    service.lastChannels = []
    service.selectedGuildId = "dms"
    service.enterGuild("dms")
    check("DMs with nothing remembered open nothing", service.currentChannelId, "")
    service.lastChannels = [{ g: "dms", c: "d1" }]
    service.enterGuild("dms")
    check("DMs open the remembered conversation", service.currentChannelId, "d1")

    // Teardown drops a parked intent.
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
    // No __sourceDir on purpose: an empty pluginDir keeps DaemonManager inert,
    // so `wanted` never goes true and no socket is ever opened.
    manifest: ({ id: "quickshell.discord" })
  }

  Panel {
    id: panel
    shell: null
    manifest: ({ id: "quickshell.discord" })
    service: mock
  }

  // Let the panel window map and every binding settle before asserting.
  Timer {
    interval: 400
    running: true
    onTriggered: harness.run()
  }
}
