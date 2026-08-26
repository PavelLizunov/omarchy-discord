pragma ComponentBehavior: Bound
import QtQuick
import QtQuick.Controls
import QtQuick.Effects
import Quickshell
import Quickshell.Hyprland
import qs.Commons
import qs.Ui

import "Api.js" as Api
import "Keymap.js" as Keymap
import "components" as Components

// Panel: login/status screens, then guild rail + channel list + timeline +
// composer. Host contract: root Item with shell/manifest/service injected,
// `opened`, open(payloadJson) (JSON string), close(). The manifest sets
// keepLoaded, so this item outlives a hide, but authoritative state (selected
// guild, open channel, messages, drafts, staged files) still lives in
// Service.qml; this file only keeps cursors.
//
// Two window modes (the `window` setting). On demand: the window maps on
// open() and unmaps on close(), and `opened` is that host-driven flag.
// Persistent: the window is mapped from shell start for Hyprland to place,
// and `opened` — "someone is looking", which gates read-acks, refreshes and
// notification suppression — is keyboard focus instead.
Item {
  id: root

  property var shell: null
  property var manifest: null
  property var service: null
  readonly property bool persistent: !!(service && service.persistentWindow)
  // Host-driven open state; `opened` is this only in On demand mode.
  property bool hostOpened: false
  readonly property bool opened: persistent ? focusScope.Window.active : hostOpened
  // Persistent mode: the window's own mapped state. A compositor close
  // (SUPER+W) unmaps it and the next open() maps it again.
  property bool persistentVisible: true
  // open() asked to summon the window before the compositor had mapped it.
  property bool pendingFocus: false
  // Where the window is parked when dismissed: the special workspace the
  // window-rule first mapped it on (e.g. "special:scratchpad"), captured once.
  // Empty when no rule parks it on a special workspace — then dismiss unmaps.
  property string parkWorkspace: ""
  // The panel's own Hyprland window. Live only after refreshToplevels() —
  // Quickshell never populates the toplevel model on its own — and its
  // `workspace` fills in when Hyprland maps the surface, a frame or more
  // after `visible` goes true.
  readonly property var toplevel: {
    var list = Hyprland.toplevels.values
    for (var i = 0; i < list.length; i++)
      if (String(list[i].title || "") === window.title) return list[i]
    return null
  }
  readonly property bool toplevelMapped: !!(toplevel && toplevel.workspace)
  property bool closingFromHost: false
  // Exposed for offscreen harnesses (dispatchKey + state inspection).
  readonly property alias timeline: timelineView
  readonly property alias composer: composerView
  readonly property alias cheatsheet: cheatsheetView
  readonly property alias picker: pickerView
  readonly property alias members: membersView
  readonly property alias controls: headerControls
  // A modal overlay (cheatsheet / emoji picker) owns the keyboard.
  readonly property bool overlayShown: cheatsheetView.shown || pickerView.shown

  readonly property string pluginId: manifest && manifest.id
    ? String(manifest.id) : "quickshell.discord"
  readonly property color foreground: Color.foreground
  readonly property color muted: Api.secondaryColor(Color.muted, Color.foreground, Color.background)
  readonly property color background: Color.background
  readonly property color accent: Color.accent
  readonly property string fontFamily: Style.font.family
  // Voice occupant rows under a voice channel in the sidebar.
  readonly property int voiceAvatarSize: Style.space(16)
  readonly property int voiceOccupantHeight: Style.space(22)
  readonly property var panelBorderSpec: Border.flat(Color.popups.border,
    Math.max(1, Style.normalBorderWidth))
  // The controls row is a Button tall, which is more than controlHeight once
  // the focus ring's reserved border is counted; both hosts reserve that so
  // the ring is never clipped by the pane below.
  readonly property real controlsRowHeight: Math.max(Style.spacing.controlHeight, headerControls.height)
  // The three buttons at their natural width, the gaps between them included
  // (a Row skips an invisible child and the gap it would have taken). Their
  // visibility conditions are repeated here rather than read off `visible`,
  // which answers EFFECTIVE visibility: while the top strip hosts the row
  // that answer depends on controlsInHeader, and reading it would close the
  // loop. `ready` is the other half of both conditions and controlsInHeader
  // already demands it.
  readonly property real controlButtonsWidth:
    (currentChannelId !== "" ? membersButton.width + Style.spacing.controlGap : 0)
    + logoutButton.width + Style.spacing.controlGap
    + closeButton.width
  // What the channel-title row can give the controls while still leaving the
  // title its gap. Driven by the timeline column's width alone, so nothing in
  // the row can feed back into it.
  readonly property real channelHeaderRoom: channelHeader.width - Style.spacing.sm * 2
    - Style.spacing.controlGap
  // The channel name is the one thing that row exists to show, so it keeps a
  // floor and the status gives way first. When even the buttons plus that
  // floor do not fit — a narrow window, more so with the member pane open —
  // the controls go back to the top strip. Hosting them regardless would push
  // the row off the left edge of the timeline column and paint it over the
  // channel list and the guild rail.
  readonly property real channelTitleFloor: Style.space(120)
  readonly property bool controlsInHeader: ready
    && channelHeaderRoom - controlButtonsWidth >= channelTitleFloor
  // The status is whatever is left over the title floor, still under its own
  // cap; on the full-width top strip only the cap applies. Too narrow to read
  // is worse than absent — a zero width drops it, and the Row then skips the
  // gap it would have taken too.
  readonly property real statusWidthBudget: {
    if (!controlsInHeader) return Style.space(200)
    var room = channelHeaderRoom - controlButtonsWidth - channelTitleFloor
      - Style.spacing.controlGap
    return room >= Style.space(80) ? Math.min(Style.space(200), room) : 0
  }

  readonly property string lifecycle: service ? service.lifecycle : ""
  readonly property bool connected: !!(service && service.connected)
  // Structure stays visible through the short reconnect grace (Service.showStructure).
  readonly property bool ready: !!(service && service.showStructure)
  readonly property bool showLogin: connected
    && (lifecycle === "logged_out" || lifecycle === "reauth_needed" || lifecycle === "qr_pending")
  readonly property string errorText: service ? Api.redact(service.lastError) : ""
  // QR login: the service mirrors qr_* events in `qr`; the QR view replaces
  // the login choices while a flow runs or has just ended (Try again).
  readonly property var qr: service ? service.qr : null
  readonly property string qrStage: qr ? String(qr.stage || "") : ""
  // Gate on the lifecycle, the pending start and any qr object so the view
  // never flashes the choices between a response and its events.
  readonly property bool qrView: showLogin && (lifecycle === "qr_pending"
    || !!(service && service.qrBusy) || qr !== null)
  // Any running flow can be cancelled, code in hand or not (after a
  // reconnect the code is replayed; until then Cancel must still work).
  readonly property bool qrCancelable: lifecycle === "qr_pending" && qrStage !== "approved"
  // Reconnected mid-flow and the replayed code never came: Try again.
  readonly property bool qrMissing: !!(service && service.qrMissing)
  property int qrSecondsLeft: 0
  // The panel window owns keyboard focus (notification suppression).
  readonly property bool windowActive: opened && focusScope.Window.active

  // --- zones: "sidebar" (columns "rail" | "channels"), "timeline", "composer",
  // "members" (only while the member pane is shown) ---
  // `zone` is the last keyboard zone; the panel controls sit outside the
  // zones, so while one of them owns focus `focusedZone` is "" and no pane
  // paints a focus border. Esc (or Tab around) hands focus back to `zone`.
  property string zone: "sidebar"
  property string column: "rail"
  readonly property bool buttonFocused: logoutButton.activeFocus || closeButton.activeFocus
    || membersButton.activeFocus || startBackendButton.activeFocus || callBarFocused
  // The call bar is one of those out-of-zone stops: Enter on the voice
  // channel you are already in lands here, and Esc / Tab leave it again.
  readonly property bool callBarFocused: callBar.visible && callBar.activeFocus
  // The channel of a call that is up or coming up. A failed call keeps its
  // ids on the wire, so gating on the id alone would leave the row reading
  // as joined and Enter parked on the error instead of retrying the join.
  readonly property string activeVoiceChannelId: {
    if (!service || !service.voice) return ""
    var status = String(service.voice.status || "")
    if (status !== "connected" && status !== "connecting") return ""
    return String(service.voice.channelId || "")
  }
  // The member pane: toggle state lives in the service (survives a
  // re-summon); it is a zone only while visible and a channel is open.
  readonly property bool membersVisible: !!(service && service.membersWanted) && currentChannelId !== "" && ready
  // Parent channel id -> true while its threads are listed beneath it.
  property var expandedThreads: ({})
  // Whether `t` from the timeline has a parent to expand (see
  // currentThreadParent): false in DMs and on an unknown channel.
  readonly property bool canToggleCurrentThreads: currentThreadParent() !== null
  readonly property string focusedZone: buttonFocused ? "" : zone
  // The composer's input or one of its chips owns the keyboard: plain keys
  // are text, only Alt chords and Tab are panel-level.
  readonly property bool composerFocused: composerView.activeFocus
  // Roving cursors keyed by id so a resync/reorder keeps the same row.
  property string guildCursorId: "dms"
  property string channelCursorId: ""

  readonly property string selectedGuildId: service ? service.selectedGuildId : ""
  readonly property string currentChannelId: service ? service.currentChannelId : ""
  readonly property var currentEntry: service && currentChannelId
    ? (service.channelData[currentChannelId] || null) : null
  readonly property var currentMessages: currentEntry && Array.isArray(currentEntry.messages)
    ? currentEntry.messages : []

  readonly property var guildRows: {
    var rows = [{ kind: "dms", id: "dms", name: "Direct Messages",
      mention_count: dmMentionCount(), unread: dmUnread() }]
    var guilds = service && Array.isArray(service.guilds) ? service.guilds : []
    for (var i = 0; i < guilds.length; i++) rows.push(guilds[i])
    return rows
  }
  readonly property bool dmsSelected: selectedGuildId === "dms"
  // Active-thread counts per parent (from the raw channel list, which
  // carries every thread the cache knows) — the "N threads" affordance.
  readonly property var threadCounts: service && selectedGuildId && !dmsSelected
    ? Api.threadCounts(service.channelsFor(selectedGuildId)) : ({})
  readonly property var channelRows: {
    if (!service) return []
    if (dmsSelected) return Array.isArray(service.dms) ? service.dms : []
    if (!selectedGuildId) return []
    var visible = Api.visibleChannels(service.channelsFor(selectedGuildId))
    var out = []
    for (var i = 0; i < visible.length; i++) {
      var row = visible[i]
      out.push(row)
      var id = String(row.id || "")
      if (!expandedThreads[id] || !Api.hasThreads(row.type)) continue
      var threads = service.threadsFor(id, selectedGuildId)
      for (var t = 0; t < threads.length; t++) out.push(threads[t])
    }
    return out
  }
  readonly property int guildCursor: indexOfId(guildRows, guildCursorId)
  readonly property int channelCursor: indexOfId(channelRows, channelCursorId)
  readonly property bool channelsLoading: !!(service && selectedGuildId
    && !dmsSelected && service.isLoadingChannels(selectedGuildId))
  readonly property string selectedGuildName: {
    for (var i = 0; i < guildRows.length; i++)
      if (String(guildRows[i].id) === selectedGuildId) return String(guildRows[i].name || "")
    return ""
  }
  // The open channel's row: prefer the structure mirror (live unread state),
  // fall back to the open_channel response.
  readonly property var currentChannel: {
    if (!service || !currentChannelId) return null
    var lists = [service.dms]
    for (var gid in service.channelsByGuild) lists.push(service.channelsByGuild[gid])
    for (var pid in service.threadsByParent) lists.push(service.threadsByParent[pid])
    for (var l = 0; l < lists.length; l++) {
      var list = Array.isArray(lists[l]) ? lists[l] : []
      for (var i = 0; i < list.length; i++)
        if (String(list[i].id || "") === currentChannelId) return list[i]
    }
    return currentEntry ? currentEntry.channel : null
  }
  // A thread's header reads "#parent › thread".
  readonly property string currentChannelTitle: {
    if (!currentChannel) return ""
    var name = String(currentChannel.name || "")
    if (String(currentChannel.type || "") === "thread" && currentChannel.parent_id) {
      var parent = service ? String(service.channelNames[String(currentChannel.parent_id)] || "") : ""
      if (parent) return "#" + parent + " › " + Api.channelGlyph("thread") + " " + name
    }
    return Api.channelGlyph(currentChannel.type) + " " + name
  }
  readonly property string currentTopic: currentChannel
    ? String(currentChannel.topic || "").replace(/\s+/g, " ") : ""
  readonly property string typingText: {
    if (!service || !currentChannelId) return ""
    var list = service.typers[currentChannelId]
    if (!Array.isArray(list) || !list.length) return ""
    var names = []
    for (var i = 0; i < list.length && i < 3; i++) names.push(String(list[i].display_name || "Someone"))
    if (list.length > 3) return "Several people are typing…"
    if (names.length === 1) return names[0] + " is typing…"
    return names.slice(0, -1).join(", ") + " and " + names[names.length - 1] + " are typing…"
  }
  property string hint: ""

  function dmMentionCount() {
    var dms = service && Array.isArray(service.dms) ? service.dms : []
    var total = 0
    for (var i = 0; i < dms.length; i++) total += Number(dms[i].mention_count) || 0
    return total
  }

  function dmUnread() {
    var dms = service && Array.isArray(service.dms) ? service.dms : []
    var state = "read"
    for (var i = 0; i < dms.length; i++) {
      var s = String(dms[i].unread || "read")
      if (s === "mentioned") return "mentioned"
      if (s === "unread") state = "unread"
    }
    return state
  }

  function textInputFocused() {
    return tokenField.activeFocus || composerView.inputFocused
  }

  // --- overlays: quick switcher (service-owned), cheatsheet, emoji picker ---
  function openSwitcher() {
    if (service) service.openSwitcher()
  }

  function toggleCheatsheet() {
    if (cheatsheetView.shown) cheatsheetView.hide()
    else { pickerView.hide(); cheatsheetView.show() }
  }

  // E on a timeline row: pick an emoji for that message. The picker lists
  // the row's own reactions first so Enter on one toggles it.
  function openPicker(messageId) {
    var message = service ? service.findMessage(currentChannelId, messageId) : null
    if (!message || message.pending) return
    cheatsheetView.hide()
    pickerView.show(message)
  }

  function applyReaction(messageId, emoji) {
    if (!service || !currentChannelId) return
    service.toggleReaction(currentChannelId, messageId, emoji)
  }

  // Chords that work from every zone, text inputs included (Ctrl+K, Ctrl+/)
  // or only outside them (/ and ?). Returns true when handled.
  function handleGlobalKey(event) {
    var key = event.key
    var text = event.text
    var ctrl = (event.modifiers & Qt.ControlModifier) !== 0
    var shift = (event.modifiers & Qt.ShiftModifier) !== 0
    if (ctrl && shift && key === Qt.Key_M) voiceAction("mute")
    else if (ctrl && shift && key === Qt.Key_D) voiceAction("deafen")
    else if (ctrl && shift && key === Qt.Key_H) voiceAction("leave")
    else if (ctrl && key === Qt.Key_K) openSwitcher()
    else if (ctrl && key === Qt.Key_Slash) toggleCheatsheet()
    // Gated on the text input itself, not on the composer zone: an
    // attachment chip owns the keyboard without being a text input, so / and
    // ? keep working from there.
    else if (!textInputFocused() && !showLogin && text === "/") openSwitcher()
    else if (!textInputFocused() && !showLogin && text === "?") toggleCheatsheet()
    else return false
    event.accepted = true
    return true
  }

  function publishActive() {
    if (service) service.panelActive = windowActive
  }

  // Tell the service whether the window is on screen at all: with keepLoaded
  // the delegates keep resolving avatars and attachments while it is unmapped.
  function publishMapped() {
    if (service) service.panelMapped = window.visible
  }

  // Login screen Tab order: Scan QR -> token field -> Log in -> Close; in
  // the QR view: Cancel / Try again -> Close.
  function loginStops() {
    return qrView ? [qrActionButton, closeButton] : [scanQrButton, tokenField, loginButton, closeButton]
  }

  function focusLogin() {
    if (!showLogin) return
    var stops = loginStops()
    for (var i = 0; i < stops.length; i++) if (stops[i].activeFocus) return
    stops[0].forceActiveFocus()
  }

  function cycleLoginFocus(delta) {
    var stops = loginStops()
    var index = -1
    for (var i = 0; i < stops.length; i++) if (stops[i].activeFocus) { index = i; break }
    stops[clampCursor(index + delta, stops.length)].forceActiveFocus()
  }

  function startQrLogin() {
    if (service) service.startQrLogin()
  }

  // Esc in the QR view: cancel a running flow, dismiss a finished one. An
  // approved flow is past cancelling (the backend is exchanging the ticket).
  function leaveQr() {
    if (!service) return
    if (lifecycle === "qr_pending") { if (qrStage !== "approved") service.cancelQrLogin() }
    else service.dismissQr()
  }

  function qrStatusText() {
    if (!qr) {
      if (qrMissing) return "The QR code did not come back after reconnecting."
      return lifecycle === "qr_pending" && !(service && service.qrBusy)
        ? "Reconnecting to QR login…" : "Starting QR login…"
    }
    var user = qr.user || null
    var name = user ? String(user.username || "") : ""
    switch (qrStage) {
      case "code": return "Scan with the Discord mobile app (Settings › Scan QR Code)"
      case "scanned": return "Logging in" + (name ? " as " + name : "") + " — confirm on your phone"
      case "approved": return "Approved, connecting…"
      case "cancelled":
        switch (String(qr.reason || "")) {
          case "declined": return "Login was declined on the phone."
          case "expired": return "The QR code expired."
          case "error": return "QR login failed." + (qr.error ? " " + qr.error : "")
          default: return "QR login cancelled."
        }
      default: return ""
    }
  }

  function qrAvatarUrl() {
    var user = qr && qr.user ? qr.user : null
    if (!user || !user.id || !user.avatar_hash) return ""
    return "https://cdn.discordapp.com/avatars/" + String(user.id) + "/" + String(user.avatar_hash) + ".png"
  }

  function updateQrCountdown() {
    qrSecondsLeft = qr && qr.expiresAt ? Math.max(0, Math.ceil((Number(qr.expiresAt) - Date.now()) / 1000)) : 0
  }

  // --- host contract ---
  // Someone started / stopped looking: a host summon in On demand mode, the
  // window gaining / losing keyboard focus in Persistent mode. That fires on
  // every focus change, so these hold only the cheap half; refreshing and
  // restoring the cursors belong to an actual open() (alt-tabbing back must
  // not move the cursor or re-fetch the structure).
  function enter() {
    if (service) service.setUiVisible("full-panel", true)
    publishScreen()
    publishPinned()
    Qt.callLater(function() {
      root.focusZone()
      if (root.showLogin) root.focusLogin()
    })
  }

  function leave() {
    if (service) service.setUiVisible("full-panel", false)
    publishScreen()
    publishPinned()
    publishActive()
  }

  function open(payloadJson) {
    var payload = ({})
    try { payload = JSON.parse(String(payloadJson || "{}")) || ({}) } catch (e) {}
    var requested = String(payload.channel_id || payload.channel || "")
    if (persistent) {
      // Summon the window to the workspace the user is on now — not by
      // revealing the scratchpad it is parked on. Map it again first if
      // SUPER+W closed it. Focus arriving is what runs enter(); a window
      // mapped in this turn is not known to Hyprland yet, so the move waits
      // for the toplevel.
      persistentVisible = true
      if (toplevelMapped) summonHere()
      else pendingFocus = true
    } else {
      closingFromHost = false
      hostOpened = true
    }
    if (service) service.refresh()
    restoreView()
    if (requested && service) {
      service.showChannel(requested, guildIdForChannel(requested))
      enterComposer()
    }
  }

  function close() {
    tokenField.clear()
    cheatsheetView.shown = false
    pickerView.shown = false
    if (persistent) {
      // Close what someone is looking at, nothing else: the host hides every
      // panel on a plugin reload, which must not toggle the user's scratchpad
      // away under them.
      if (opened) hidePersistent()
      return
    }
    closingFromHost = true
    hostOpened = false
    closingFromHost = false
  }

  // Omarchy Quattro's Hyprland evaluates every dispatch as Lua — it wraps the
  // string as `return hl.dispatch(<string>)` — so plain "movetoworkspace ..."
  // is rejected (this is why focus/reveal never worked). We send the Lua
  // dispatcher form, addressing the window explicitly so only this window
  // moves — the scratchpad it parks on is shared with other windows.
  function moveWindow(workspace, follow) {
    var addr = toplevel ? String(toplevel.address || "") : ""
    if (!addr) return false
    var f = follow ? "" : ", follow = false"
    Hyprland.dispatch("hl.dsp.window.move({ window = \"address:" + addr
      + "\", workspace = \"" + workspace + "\"" + f + " })")
    return true
  }

  function focusWindow() {
    moveWindow(Hyprland.focusedWorkspace ? Hyprland.focusedWorkspace.id : "", true)
  }

  // Persistent mode's open: bring the window to the workspace the user is on
  // and focus it (move follows the window), pulling it off its park rather
  // than revealing that shared workspace in place.
  function summonHere() {
    var ws = Hyprland.focusedWorkspace
    if (!(ws && ws.id !== undefined && moveWindow(ws.id, true)))
      persistentVisible = true
  }

  // Persistent mode's close: send the window back to its park workspace
  // silently (follow = false, so the user is not switched away), leaving it
  // mapped and ready for the next summon. With no park rule, unmap it — the
  // same state SUPER+W leaves behind, which open() maps back.
  function hidePersistent() {
    if (!(parkWorkspace && moveWindow(parkWorkspace, false)))
      persistentVisible = false
  }

  // Tell the service whether the timeline is scrolled up, so it never trims
  // the rolling message window out from under the user (nobody is looking
  // while the panel is closed).
  function publishPinned() {
    if (service) service.timelinePinned = !opened || timelineView.pinned
  }

  // Tell the bar widgets which monitor hosts the panel, so a click on the
  // same monitor closes it while a click elsewhere remaps it.
  function publishScreen() {
    if (!service) return
    service.panelScreenName = opened && window.screen ? String(window.screen.name || "") : ""
  }

  function requestClose() {
    if (shell && typeof shell.hide === "function") shell.hide(pluginId)
    else close()
  }

  // Put the cursors back on the view the service remembers.
  function restoreView() {
    guildCursorId = selectedGuildId || "dms"
    channelCursorId = currentChannelId
    if (currentChannelId) {
      zone = "composer"
      column = "channels"
    } else {
      zone = "sidebar"
      column = selectedGuildId ? "channels" : "rail"
    }
  }

  function guildIdForChannel(channelId) {
    if (!service) return ""
    var id = String(channelId || "")
    for (var d = 0; d < service.dms.length; d++)
      if (String(service.dms[d].id || "") === id) return "dms"
    for (var gid in service.channelsByGuild) {
      var list = service.channelsByGuild[gid]
      if (!Array.isArray(list)) continue
      for (var i = 0; i < list.length; i++) if (String(list[i].id || "") === id) return gid
    }
    return ""
  }

  // --- cursor helpers ---
  function clampCursor(index, length) {
    if (length <= 0) return 0
    return ((index % length) + length) % length
  }

  function indexOfId(list, id) {
    if (!id) return -1
    for (var i = 0; i < list.length; i++)
      if (String(list[i].id || "") === id) return i
    return -1
  }

  function setGuildCursor(index) {
    if (!guildRows.length) { guildCursorId = "dms"; return }
    index = Math.max(0, Math.min(index, guildRows.length - 1))
    guildCursorId = String(guildRows[index].id || "")
    guildList.positionViewAtIndex(index, ListView.Contain)
  }

  function moveGuildCursor(delta) {
    if (!guildRows.length) return
    var index = guildCursor < 0 ? 0 : guildCursor
    setGuildCursor(clampCursor(index + delta, guildRows.length))
  }

  function setChannelCursor(index) {
    if (index < 0 || index >= channelRows.length) return
    channelCursorId = String(channelRows[index].id || "")
    channelList.positionViewAtIndex(index, ListView.Contain)
  }

  // Steps from `from` by `delta` (wrapping) to the next row satisfying
  // `accept`; -1 when none does.
  function findChannel(from, delta, accept) {
    var count = channelRows.length
    if (!count) return -1
    var index = from
    for (var step = 0; step < count; step++) {
      index = clampCursor(index + delta, count)
      if (accept(channelRows[index])) return index
    }
    return -1
  }

  function moveChannelCursor(delta) {
    var from = channelCursor
    if (from < 0) from = delta > 0 ? -1 : 0
    var next = findChannel(from, delta, Api.isSelectableChannel)
    if (next >= 0) setChannelCursor(next)
  }

  function ensureCursors() {
    if (guildCursor < 0) guildCursorId = "dms"
    if (channelCursor >= 0 && Api.isSelectableChannel(channelRows[channelCursor])) return
    // The open channel wins over row zero: entering a guild clears the cursor
    // and the restored channel only appears once its list lands, so this is
    // what puts the cursor on the channel that just opened.
    var at = indexOfId(channelRows, currentChannelId)
    if (at < 0) at = findChannel(-1, 1, Api.isSelectableChannel)
    if (at >= 0) setChannelCursor(at)
    else channelCursorId = ""
  }

  // --- navigation ---
  function selectGuild(index) {
    if (index < 0 || index >= guildRows.length || !service) return
    setGuildCursor(index)
    var id = String(guildRows[index].id || "")
    if (service.selectedGuildId !== id) {
      service.selectedGuildId = id
      channelCursorId = ""
    }
    if (id !== "dms") service.loadChannels(id)
  }

  function enterChannels() {
    var index = guildCursor < 0 ? 0 : guildCursor
    // Read before selectGuild mutates the selection: re-entering the server
    // that already owns the open channel must not tear the timeline down.
    var target = index >= 0 && index < guildRows.length ? String(guildRows[index].id || "") : ""
    var owned = target !== "" && target === selectedGuildId && currentChannelId !== ""
    selectGuild(index)
    zone = "sidebar"
    column = "channels"
    hint = ""
    // Open the channel this server was last left on. When its list is already
    // cached this resolves synchronously, which is why it runs before the
    // cursor fixup below; otherwise ensureCursors() catches the cursor up when
    // the list lands. Focus deliberately stays in the channel column — unlike
    // activateChannel this never drags the keyboard into the composer.
    if (!owned && service) service.enterGuild(target)
    if (currentChannelId && indexOfId(channelRows, currentChannelId) >= 0)
      channelCursorId = currentChannelId
    ensureCursors()
    focusZone()
  }

  function leaveChannels() {
    column = "rail"
    hint = ""
  }

  // `origin` is the zone the activation came from: Enter in the channel list,
  // a click and a summon leave it empty and focus the composer (PLAN keyboard
  // contract), while Alt+↑/↓ stepping passes its zone so the keyboard stays
  // where it was — in the timeline (cursor on the newest row) or the member
  // pane — instead of being dragged into the composer.
  function activateChannel(index, origin) {
    var row = channelRows[index]
    if (!row || !service) return
    // A forum is not a channel to read: Enter lists its threads instead.
    if (String(row.type || "") === "forum") { toggleThreads(index); return }
    // A voice channel is not a channel to read either: Enter joins it.
    if (String(row.type || "") === "voice") { joinVoice(index); return }
    if (!Api.isOpenableChannel(row)) return
    hint = ""
    setChannelCursor(index)
    service.showChannel(String(row.id || ""), selectedGuildId)
    timelineView.focusNewest()
    if (origin === "timeline") enterTimeline()
    else if (origin === "members" && membersVisible) enterMembers()
    else enterComposer()
  }

  // --- voice ---
  // Enter on a voice row: join it, or — when it is the call already running —
  // put the keyboard on the call bar instead of re-joining.
  function joinVoice(index) {
    var row = channelRows[index]
    if (!row || !service) return
    hint = ""
    setChannelCursor(index)
    var id = String(row.id || "")
    if (activeVoiceChannelId && activeVoiceChannelId === id) { focusCallBar(); return }
    service.voiceJoin(String(row.guild_id || selectedGuildId), id)
  }

  function focusCallBar() {
    if (!callBar.visible) return
    callBar.forceActiveFocus()
  }

  // The three call chords, from every zone (Composer forwards its own).
  function voiceAction(action) {
    if (!service) return
    if (action === "mute") service.toggleMute()
    else if (action === "deafen") service.toggleDeafen()
    else if (action === "leave") service.voiceLeave()
  }

  // t on a channel (or a thread: its parent): list / hide the active
  // threads beneath it. The list comes from list_threads (cached, refreshed
  // on channel_update for that parent); until it answers the thread rows
  // the channel list already carries stand in.
  function toggleThreads(index) {
    var row = channelRows[index]
    if (!row || !service || dmsSelected) return
    var id = String(row.id || "")
    if (String(row.type || "") === "thread") id = String(row.parent_id || "")
    toggleThreadsFor(id, String(row.id || ""))
  }

  // Expand / collapse a parent by id (the row may not be in the list yet —
  // `t` from a thread's timeline expands the parent in a guild whose channels
  // are still loading).
  function toggleThreadsFor(parentId, cursorId) {
    var id = String(parentId || "")
    if (!id || !service) return
    var next = Api.shallowCopy(expandedThreads)
    if (next[id]) delete next[id]
    else {
      next[id] = true
      service.listThreads(id)
    }
    expandedThreads = next
    // The cursor is id-keyed: it stays on the row as the list reflows, or
    // moves up to the parent when the row was one of the threads hidden.
    channelCursorId = next[id] ? String(cursorId || id) : id
  }

  // The parent the open channel's threads hang off: itself for a text /
  // announcement / forum channel, its parent for a thread. null when `t`
  // from the timeline has nothing to do (DMs, unknown channel).
  function currentThreadParent() {
    var row = currentChannel
    if (!row) return null
    var guildId = String(row.guild_id || "")
    if (!guildId) return null
    var type = String(row.type || "")
    var id = type === "thread" ? String(row.parent_id || "")
      : (Api.hasThreads(type) ? String(row.id || "") : "")
    return id ? { guildId: guildId, id: id } : null
  }

  // t from the timeline: the open channel's threads (a thread's parent's),
  // cursor on the open channel. The parent may live in another guild than
  // the one the sidebar shows, so select that guild first.
  function toggleCurrentThreads() {
    var target = currentThreadParent()
    if (!target) return
    if (selectedGuildId !== target.guildId) {
      var guildIndex = indexOfId(guildRows, target.guildId)
      if (guildIndex >= 0) selectGuild(guildIndex)
    }
    zone = "sidebar"
    column = "channels"
    toggleThreadsFor(target.id, currentChannelId)
    focusZone()
  }

  // --- member pane ---
  function toggleMembers() {
    if (!service) return
    // The pane is a channel's member list: with nothing open there is
    // nothing to show, so say so instead of arming it invisibly.
    if (!currentChannelId) { hint = "Open a channel first"; return }
    service.setMembersWanted(!service.membersWanted)
    hint = ""
    if (!service.membersWanted && zone === "members") { zone = "composer"; focusZone() }
  }

  function enterMembers() {
    if (!membersVisible) return
    zone = "members"
    hint = ""
    focusZone()
  }

  function leaveMembers() {
    zone = "composer"
    focusZone()
  }

  function enterTimeline() {
    if (!currentChannelId) return
    zone = "timeline"
    timelineView.focusNewest()
    focusZone()
  }

  function leaveTimeline(markRead) {
    if (markRead && service && currentChannelId) service.markChannelRead(currentChannelId)
    zone = "sidebar"
    column = "channels"
    if (currentChannelId && indexOfId(channelRows, currentChannelId) >= 0)
      channelCursorId = currentChannelId
    focusZone()
  }

  function enterComposer() {
    if (!currentChannelId) return
    zone = "composer"
    hint = ""
    focusZone()
  }

  // Esc out of the composer: the channel was being read, so ack its newest
  // row on the way to the timeline.
  function leaveComposer(markRead) {
    if (markRead && service && currentChannelId) service.markChannelRead(currentChannelId)
    enterTimeline()
  }

  // Reply from the timeline's R: reply mode in the composer, focus there.
  function replyTo(messageId) {
    var message = service ? service.findMessage(currentChannelId, messageId) : null
    if (!message) return
    var author = message.author || {}
    composerView.startReply(messageId, String(author.display_name || author.username || "someone"))
    enterComposer()
  }

  function moveZone(direction) {
    if (direction === "right") {
      if (zone === "sidebar" && column === "rail") { enterChannels(); return }
      if (zone === "sidebar" && currentChannelId) enterTimeline()
      else if (zone === "timeline") enterComposer()
      else if (zone === "composer" && membersVisible) enterMembers()
      return
    }
    if (zone === "members") leaveMembers()
    else if (zone === "composer") leaveComposer(false)
    else if (zone === "timeline") leaveTimeline(false)
    else if (column === "channels") leaveChannels()
  }

  // Alt+Up/Down (and Shift for unread only): step through the current list
  // from the open channel and open the neighbour.
  function stepChannel(delta, unreadOnly) {
    var from = indexOfId(channelRows, currentChannelId)
    // Open channel not in this list: the cursor row itself is the first candidate.
    if (from < 0) from = channelCursor >= 0 ? channelCursor - delta : (delta > 0 ? -1 : 0)
    var accept = unreadOnly
      ? function(row) { return Api.isOpenableChannel(row) && Api.isUnread(row) }
      : Api.isOpenableChannel
    var next = findChannel(from, delta, accept)
    if (next < 0 || next === from) return
    // Stepping keeps the keyboard where it is (timeline / member pane).
    activateChannel(next, zone)
  }

  function focusZone() {
    if (zone === "members" && !membersVisible) zone = "composer"
    if ((zone === "timeline" || zone === "composer") && !currentChannelId) {
      zone = "sidebar"
      column = selectedGuildId ? "channels" : "rail"
    }
    if (zone === "members") membersView.forceActiveFocus()
    else if (zone === "composer") composerView.focusInput()
    else if (zone === "timeline") timelineView.forceActiveFocus()
    else sidebarFocus.forceActiveFocus()
  }

  // Tab order: rail -> channels -> timeline -> composer (then its chips) ->
  // member list -> Members -> Log out -> Close -> rail. Stops that cannot
  // take focus right now (no open channel, hidden pane or button) are skipped.
  function cycleFocus(delta) {
    var stops = ["rail", "channels", "timeline", "composer", "callbar", "members", "startBackend",
      "membersButton", "logout", "close"]
    var current = callBarFocused ? "callbar"
      : (buttonFocused
        ? (closeButton.activeFocus ? "close"
          : (membersButton.activeFocus ? "membersButton"
            : (startBackendButton.activeFocus ? "startBackend" : "logout")))
        : (zone === "sidebar" ? column : zone))
    var index = stops.indexOf(current)
    for (var step = 0; step < stops.length; step++) {
      index = clampCursor(index + delta, stops.length)
      var stop = stops[index]
      // No zones while the login / status screen covers the panel body.
      if ((stop === "rail" || stop === "channels") && !ready) continue
      if ((stop === "timeline" || stop === "composer") && !currentChannelId) continue
      if (stop === "callbar" && !(ready && callBar.visible)) continue
      if (stop === "members" && !membersVisible) continue
      if (stop === "startBackend" && !startBackendButton.visible) continue
      if (stop === "membersButton" && !membersButton.visible) continue
      if (stop === "logout" && !logoutButton.visible) continue
      focusStop(stop, delta)
      return
    }
  }

  function focusStop(stop, delta) {
    hint = ""
    if (stop === "logout") { logoutButton.forceActiveFocus(); return }
    if (stop === "close") { closeButton.forceActiveFocus(); return }
    if (stop === "membersButton") { membersButton.forceActiveFocus(); return }
    if (stop === "startBackend") { startBackendButton.forceActiveFocus(); return }
    if (stop === "callbar") { focusCallBar(); return }
    if (stop === "members") { enterMembers(); return }
    if (stop === "channels") { enterChannels(); return }
    if (stop === "composer") {
      zone = "composer"
      // Shift+Tab backwards lands on the last chip first, then the input.
      if (delta < 0 && composerView.chips.length) composerView.focusChip(composerView.chips.length - 1)
      else composerView.focusInput()
      return
    }
    if (stop === "timeline") zone = "timeline"
    else { zone = "sidebar"; column = "rail" }
    focusZone()
  }

  // `r` while the browser is down: start the backend if it is not running,
  // otherwise re-pull state.
  function retry() {
    if (!service) return
    if (!connected) service.startBackend()
    else service.refresh()
  }

  function reload() {
    if (!service) return
    service.refresh()
    if (selectedGuildId && !dmsSelected) service.loadChannels(selectedGuildId, true)
    if (currentChannelId) service.openChannel(currentChannelId)
  }

  function submitToken() {
    if (!service) return
    var token = tokenField.text
    tokenField.clear()
    service.login(token)
    token = ""
  }

  // Panel-level keys. The Timeline and the Composer handle their own keys
  // first when they have focus and only unhandled ones arrive here.
  function handleKey(event) {
    var key = event.key
    var text = event.text
    var alt = (event.modifiers & Qt.AltModifier) !== 0
    var shift = (event.modifiers & Qt.ShiftModifier) !== 0
    if (overlayShown) return
    if (handleGlobalKey(event)) return
    if (showLogin) {
      // Buttons take Enter themselves; the token field takes its text.
      if (key === Qt.Key_Tab || key === Qt.Key_Backtab) cycleLoginFocus(key === Qt.Key_Backtab || shift ? -1 : 1)
      else if (key === Qt.Key_Escape) { if (qrView) leaveQr(); else root.requestClose() }
      else return
      event.accepted = true
      return
    }
    if (tokenField.activeFocus) return
    if (!ready) {
      // The footer promises Tab reaches the buttons: cycleFocus skips every
      // stop that cannot take focus right now, which down here leaves
      // Start backend (when it is shown) and Close.
      if (key === Qt.Key_Tab || key === Qt.Key_Backtab) cycleFocus(key === Qt.Key_Backtab || shift ? -1 : 1)
      else if (key === Qt.Key_Escape) root.requestClose()
      else if (text === "r") retry()
      else return
      event.accepted = true
      return
    }
    if (composerFocused) {
      // Everything but the channel-stepping chords is text (or chip keys);
      // Alt+m is claimed by the composer itself (membersRequested).
      if (alt && key === Qt.Key_Down) stepChannel(1, shift)
      else if (alt && key === Qt.Key_Up) stepChannel(-1, shift)
      else return
      event.accepted = true
      return
    }
    if (key === Qt.Key_Tab || key === Qt.Key_Backtab) cycleFocus(key === Qt.Key_Backtab || shift ? -1 : 1)
    else if (buttonFocused) {
      // The button handles Enter/Space itself; Esc returns to the zone.
      if (key !== Qt.Key_Escape) return
      focusZone()
    }
    else if (alt && key === Qt.Key_Down) stepChannel(1, shift)
    else if (alt && key === Qt.Key_Up) stepChannel(-1, shift)
    else if (alt && key === Qt.Key_H) moveZone("left")
    else if (alt && key === Qt.Key_L) moveZone("right")
    else if (text === "m" || (alt && key === Qt.Key_M)) toggleMembers()
    else if (text === "r") reload()
    else if (text === "t" && zone === "timeline") toggleCurrentThreads()
    else if (zone === "sidebar") {
      if (!handleSidebarKey(key, text)) return
    } else return
    event.accepted = true
  }

  function handleSidebarKey(key, text) {
    // The rail is the end of the Esc ladder: Esc is consumed here so walking
    // back out can never close the panel. Closing is the Close button, the
    // window close, the bar widget, or the quickshell.discord.panel IPC.
    if (key === Qt.Key_Escape) {
      if (column === "channels") leaveChannels()
      return true
    }
    if (column === "rail") {
      if (key === Qt.Key_Down || text === "j") moveGuildCursor(1)
      else if (key === Qt.Key_Up || text === "k") moveGuildCursor(-1)
      else if (key === Qt.Key_Home || text === "g") setGuildCursor(0)
      else if (key === Qt.Key_End || text === "G") setGuildCursor(guildRows.length - 1)
      else if (key === Qt.Key_Return || key === Qt.Key_Enter || key === Qt.Key_Right
          || text === "l") enterChannels()
      else return false
      return true
    }
    if (key === Qt.Key_Down || text === "j") moveChannelCursor(1)
    else if (key === Qt.Key_Up || text === "k") moveChannelCursor(-1)
    else if (key === Qt.Key_Home || text === "g") { channelCursorId = ""; moveChannelCursor(1) }
    else if (key === Qt.Key_End || text === "G") { channelCursorId = ""; moveChannelCursor(-1) }
    else if (key === Qt.Key_Left || text === "h") leaveChannels()
    else if (key === Qt.Key_Right || text === "l") { if (currentChannelId) enterTimeline() }
    else if (key === Qt.Key_Return || key === Qt.Key_Enter) activateChannel(channelCursor)
    else if (text === "t") toggleThreads(channelCursor)
    else return false
    return true
  }

  // Synthesized-key entry point for offscreen harnesses (mirrors the focus
  // chain: the timeline first when it owns the zone, then the panel).
  function dispatchKey(event) {
    if (cheatsheetView.shown) { cheatsheetView.handleKey(event); return event.accepted }
    if (pickerView.shown) { pickerView.handleKey(event); return event.accepted }
    if (ready && !buttonFocused) {
      if (zone === "composer" && composerFocused) composerView.handleKey(event)
      else if (zone === "timeline" && !textInputFocused()) timelineView.handleKey(event)
      else if (zone === "members" && membersVisible) membersView.handleKey(event)
    }
    if (!event.accepted) handleKey(event)
    return event.accepted
  }

  onGuildRowsChanged: ensureCursors()
  onChannelRowsChanged: ensureCursors()
  onShowLoginChanged: if (showLogin && opened) Qt.callLater(focusLogin)
  // Switching between the choices and the QR view moves the focus stop.
  onQrViewChanged: if (showLogin && opened) Qt.callLater(focusLogin)
  onWindowActiveChanged: publishActive()
  // Host-driven and focus-driven transitions share the same two functions.
  onOpenedChanged: opened ? enter() : leave()
  // The compositor has the window now. Capture its park workspace the first
  // time it maps there (the window rule's special workspace), then run any
  // summon that open() deferred until the toplevel existed.
  onToplevelMappedChanged: if (toplevelMapped) {
    if (!parkWorkspace && toplevel && toplevel.workspace) {
      var name = String(toplevel.workspace.name || "")
      if (name.indexOf("special:") === 0) parkWorkspace = name
    }
    if (pendingFocus) {
      pendingFocus = false
      summonHere()
    }
  }
  // Quickshell leaves the toplevel model empty until something asks for it,
  // and only tracks it live from that point on.
  onPersistentChanged: if (persistent) Hyprland.refreshToplevels()
  Component.onCompleted: {
    publishMapped()
    if (persistent) Hyprland.refreshToplevels()
  }
  onQrChanged: updateQrCountdown()
  // Losing the session hides the panes; put the zone and the keyboard focus
  // back on the rail together (the timeline would otherwise keep focus while
  // the zone says "rail", and reclaim it when the panes reappear).
  onReadyChanged: {
    if (!ready) { zone = "sidebar"; column = "rail" }
    if (opened) focusZone()
  }
  onMembersVisibleChanged: if (!membersVisible && zone === "members" && opened) { zone = "composer"; focusZone() }
  // A guild switch drops the expansion state with the rows it applied to.
  onSelectedGuildIdChanged: expandedThreads = ({})

  Component.onDestruction: {
    tokenField.clear()
    if (service) {
      service.setUiVisible("full-panel", false)
      service.panelScreenName = ""
      service.panelActive = false
      service.panelMapped = false
      service.timelinePinned = true
    }
  }

  Timer {
    interval: 1000
    repeat: true
    running: root.opened && (root.qrStage === "code" || root.qrStage === "scanned")
    triggeredOnStart: true
    onTriggered: root.updateQrCountdown()
  }

  // Ack-on-read: the timeline reached its newest row while focused and
  // visible; debounce so a burst of arrivals acks once, and re-check at fire
  // time that we are still pinned to the bottom.
  Timer {
    id: ackTimer
    interval: 500
    onTriggered: {
      if (!root.opened || !root.currentChannelId) return
      if (root.focusedZone !== "timeline" && root.focusedZone !== "composer") return
      if (!timelineView.pinned || !root.service) return
      root.service.markChannelRead(root.currentChannelId)
    }
  }

  FloatingWindow {
    id: window
    visible: root.persistent ? root.persistentVisible : root.hostOpened
    title: "Omarchy Discord"
    color: root.background
    implicitWidth: Style.space(1040)
    implicitHeight: Style.space(680)
    minimumSize: Qt.size(Style.space(640), Style.space(420))

    onVisibleChanged: {
      root.publishMapped()
      // Persistent mode: a compositor close (SUPER+W) just unmaps the window;
      // mirror it so the binding agrees and the next open() maps it again.
      if (root.persistent) {
        if (!visible) root.persistentVisible = false
        return
      }
      if (!visible && root.opened && !root.closingFromHost) root.requestClose()
    }
    onScreenChanged: root.publishScreen()

    FocusScope {
      id: focusScope
      anchors.fill: parent
      focus: true
      Keys.priority: Keys.BeforeItem
      Keys.onPressed: function(event) { root.handleKey(event) }

      // Keyboard owner for the sidebar zone (keys bubble to focusScope).
      Item {
        id: sidebarFocus
        focus: true
        width: 0
        height: 0
      }

      // The panel controls, declared once and reparented between two hosts:
      // the channel-title row while the client is up, a slim top strip during
      // the login / QR / status screens. cycleFocus(), focusStop(),
      // buttonFocused and loginStops() address these buttons by id, so a
      // second copy would break the Tab cycle (and duplicate ids are illegal
      // anyway). Anchorless on purpose — anchors cannot survive a reparent;
      // each host slot sizes itself to the row instead.
      Row {
        id: headerControls
        parent: root.controlsInHeader ? channelControlsSlot : topControlsSlot
        spacing: Style.spacing.controlGap

        Text {
          anchors.verticalCenter: parent.verticalCenter
          // The status shares a row with the channel title now, so it takes
          // what that row can spare (statusWidthBudget) and drops out
          // entirely — the Row skips it and its gap — before the title is
          // squeezed.
          width: Math.min(implicitWidth, root.statusWidthBudget)
          visible: width > 0
          elide: Text.ElideRight
          text: root.service ? root.service.statusText
            + (root.service.user ? " · " + String(root.service.user.display_name
              || root.service.user.username || "") : "") : ""
          color: root.muted
          font.family: root.fontFamily
          font.pixelSize: Style.font.bodySmall
        }

        // All three buttons are reached through the panel's own Tab cycle
        // (cycleFocus); Qt's tab chain would otherwise capture Tab while a
        // button has focus and bounce between them.
        Button {
          id: membersButton
          visible: root.ready && root.currentChannelId !== ""
          text: "Members"
          active: root.membersVisible
          focusable: true
          activeFocusOnTab: false
          foreground: root.foreground
          tooltipText: "Show / hide the member list (m)"
          onClicked: root.toggleMembers()
        }
        Button {
          id: logoutButton
          visible: root.ready
          text: "Log out"
          focusable: true
          activeFocusOnTab: false
          foreground: root.foreground
          onClicked: if (root.service) root.service.logout()
        }
        Button {
          id: closeButton
          text: "Close"
          focusable: true
          activeFocusOnTab: false
          foreground: root.foreground
          onClicked: root.requestClose()
        }
      }

      // The channel a guild entry restored (Service.resolveGuildEntry): its
      // list may only have landed a moment ago, by which point ensureCursors()
      // has already parked the cursor on the first row.
      Connections {
        target: root.service
        ignoreUnknownSignals: true
        function onGuildEntered(guildId, channelId) {
          if (guildId !== root.selectedGuildId) return
          var at = root.indexOfId(root.channelRows, channelId)
          if (at >= 0) root.setChannelCursor(at)
        }
      }

      // Modal overlays above the whole panel; each returns the keyboard to
      // the last zone when it closes.
      Components.Cheatsheet {
        id: cheatsheetView
        anchors.fill: parent
        z: 10
        onCloseRequested: Qt.callLater(root.focusZone)
      }

      Components.EmojiPicker {
        id: pickerView
        anchors.fill: parent
        z: 10
        service: root.service
        onPicked: function(emoji) { root.applyReaction(pickerView.messageId, emoji) }
        onCloseRequested: Qt.callLater(root.focusZone)
      }

      Column {
        anchors.fill: parent
        anchors.margins: Style.spacing.panelPadding
        spacing: Style.spacing.panelGap

        // The login / QR / status screens have no channel-title row to host
        // the controls, and neither does a channel-title row too narrow to
        // hold them, so a slim strip at the top right takes them. It is
        // hidden (not zero-height) otherwise, so the Column drops its
        // panelGap too and the body reclaims the whole space.
        Item {
          id: topStrip
          width: parent.width
          height: root.controlsRowHeight
          visible: !root.controlsInHeader

          Item {
            id: topControlsSlot
            anchors.right: parent.right
            anchors.verticalCenter: parent.verticalCenter
            width: headerControls.width
            height: headerControls.height
          }
        }

        // Body
        Item {
          id: body
          width: parent.width
          // A Column skips invisible children and the gap they would have
          // added, so the top strip's height and its gap only count while it
          // is shown.
          height: parent.height - footer.height - parent.spacing
            - (topStrip.visible ? topStrip.height + parent.spacing : 0)

          // Status (backend down / starting / connecting)
          Column {
            anchors.centerIn: parent
            visible: !root.showLogin && !root.ready
            spacing: Style.spacing.lg
            width: Math.min(parent.width, Style.space(420))

            Text {
              width: parent.width
              horizontalAlignment: Text.AlignHCenter
              text: root.service ? root.service.statusText : "Loading"
              color: root.foreground
              font.family: root.fontFamily
              font.pixelSize: Style.font.heading
            }
            Text {
              width: parent.width
              horizontalAlignment: Text.AlignHCenter
              wrapMode: Text.WordWrap
              text: {
                if (!root.service) return "The Discord service is not loaded."
                if (!root.service.daemon.runtimeChecked) return "Checking the backend install."
                if (root.service.daemon.setupBusy) return "Installing the bundled backend."
                if (!root.service.daemon.runtimeAvailable) return "The backend is not installed. Run scripts/setup.sh from the plugin directory."
                if (!root.service.daemon.running) return "The backend is stopped. Press r or Start backend."
                if (!root.connected) return "Waiting for the backend socket. Press r to retry."
                return "Connecting to Discord."
              }
              color: root.muted
              font.family: root.fontFamily
              font.pixelSize: Style.font.body
            }
            Button {
              id: startBackendButton
              anchors.horizontalCenter: parent.horizontalCenter
              visible: !!(root.service && root.service.daemon.runtimeAvailable
                && !root.service.daemon.running)
              text: "Start backend"
              focusable: true
              // Reached through cycleFocus like the panel controls; Qt's own
              // tab chain would otherwise compete for Tab.
              activeFocusOnTab: false
              foreground: root.foreground
              onClicked: if (root.service) root.service.startBackend()
            }
          }

          // Login: Scan QR (default) or paste a token; the QR view takes over
          // while a flow runs.
          Column {
            anchors.centerIn: parent
            visible: root.showLogin
            spacing: Style.spacing.lg
            width: Math.min(parent.width, Style.space(520))

            Text {
              width: parent.width
              text: root.lifecycle === "reauth_needed" ? "Session expired — log in again" : "Log in to Discord"
              color: root.foreground
              font.family: root.fontFamily
              font.pixelSize: Style.font.heading
            }

            // --- QR view ---
            Column {
              id: qrColumn
              width: parent.width
              visible: root.qrView
              spacing: Style.spacing.lg

              Item {
                id: qrFrame
                anchors.horizontalCenter: parent.horizontalCenter
                // Half the backend's 512 px PNG: an exact 2:1 downscale keeps
                // every module crisp for the phone camera.
                width: Style.space(256)
                height: width
                visible: root.qrStage === "code" || root.qrStage === ""

                // The PNG is rewritten per qr_code; the revision query defeats
                // Qt's pixmap cache so a fresh code always reloads.
                Rectangle {
                  anchors.fill: parent
                  radius: Style.cornerRadius
                  color: Util.alpha(root.foreground, 0.06)
                  visible: qrImage.status !== Image.Ready
                  Text {
                    anchors.centerIn: parent
                    width: parent.width - Style.spacing.lg * 2
                    wrapMode: Text.WrapAnywhere
                    horizontalAlignment: Text.AlignHCenter
                    text: root.qr && root.qr.url && qrImage.status !== Image.Loading
                      ? String(root.qr.url) : (root.qrMissing ? "" : "Requesting a code…")
                    color: root.muted
                    font.family: root.fontFamily
                    font.pixelSize: Style.font.caption
                  }
                }
                Image {
                  id: qrImage
                  anchors.fill: parent
                  cache: false
                  asynchronous: true
                  fillMode: Image.PreserveAspectFit
                  source: root.qr && root.qr.imagePath
                    ? "file://" + String(root.qr.imagePath) + "?r=" + String(root.qr.revision || 0) : ""
                }
              }

              // Scanned: who is logging in.
              Row {
                anchors.horizontalCenter: parent.horizontalCenter
                visible: root.qrStage === "scanned"
                spacing: Style.spacing.md

                Item {
                  width: Style.space(40)
                  height: width
                  anchors.verticalCenter: parent.verticalCenter
                  Rectangle {
                    anchors.fill: parent
                    radius: width / 2
                    color: Util.alpha(root.foreground, 0.1)
                    visible: !qrAvatarEffect.visible
                    Text {
                      anchors.centerIn: parent
                      text: root.qr && root.qr.user && root.qr.user.username
                        ? String(root.qr.user.username).charAt(0).toUpperCase() : "?"
                      color: root.muted
                      font.family: root.fontFamily
                      font.pixelSize: Style.font.subtitle
                      font.bold: true
                    }
                  }
                  Rectangle { id: qrAvatarMask; anchors.fill: parent; radius: width / 2; visible: false; layer.enabled: true }
                  Image {
                    id: qrAvatarImage
                    anchors.fill: parent
                    visible: false
                    asynchronous: true
                    fillMode: Image.PreserveAspectCrop
                    sourceSize.width: width * 2
                    sourceSize.height: height * 2
                    readonly property string path: root.service ? root.service.mediaPath(root.qrAvatarUrl(), 64) : ""
                    source: path ? "file://" + path : ""
                    onStatusChanged: if (status === Image.Error && root.service) root.service.mediaError(path)
                  }
                  MultiEffect {
                    id: qrAvatarEffect
                    anchors.fill: qrAvatarImage
                    source: qrAvatarImage
                    maskEnabled: true
                    maskSource: qrAvatarMask
                    visible: qrAvatarImage.path !== "" && qrAvatarImage.status === Image.Ready
                  }
                }
                Text {
                  anchors.verticalCenter: parent.verticalCenter
                  text: root.qr && root.qr.user ? String(root.qr.user.username || "") : ""
                  color: root.foreground
                  font.family: root.fontFamily
                  font.pixelSize: Style.font.title
                  font.bold: true
                }
              }

              Text {
                id: qrStatus
                width: parent.width
                horizontalAlignment: Text.AlignHCenter
                wrapMode: Text.WordWrap
                text: root.qrStatusText()
                color: root.qrStage === "cancelled" ? Color.urgent : root.foreground
                font.family: root.fontFamily
                font.pixelSize: Style.font.body
              }
              Text {
                width: parent.width
                horizontalAlignment: Text.AlignHCenter
                visible: root.qrStage === "code" || root.qrStage === "scanned"
                text: root.qrSecondsLeft > 0
                  ? "Code expires in " + Math.floor(root.qrSecondsLeft / 60) + ":"
                    + (root.qrSecondsLeft % 60 < 10 ? "0" : "") + (root.qrSecondsLeft % 60)
                  : "Code expiring…"
                color: root.muted
                font.family: root.fontFamily
                font.pixelSize: Style.font.caption
              }
              Button {
                id: qrActionButton
                anchors.horizontalCenter: parent.horizontalCenter
                text: root.qrStage === "cancelled" || root.qrMissing ? "Try again" : "Cancel"
                focusable: true
                activeFocusOnTab: false
                foreground: root.foreground
                enabled: !(root.service && root.service.qrBusy)
                onClicked: {
                  if (root.qrMissing) root.service.restartQrLogin()
                  else if (root.qrStage === "cancelled") root.startQrLogin()
                  else root.leaveQr()
                }
              }
            }

            // --- choices: Scan QR / token ---
            Column {
              width: parent.width
              visible: !root.qrView
              spacing: Style.spacing.lg

              Row {
                spacing: Style.spacing.md
                Button {
                  id: scanQrButton
                  text: "Scan QR"
                  focusable: true
                  activeFocusOnTab: false
                  foreground: root.foreground
                  enabled: !(root.service && (root.service.loginBusy || root.service.qrBusy))
                  onClicked: root.startQrLogin()
                }
                Text {
                  anchors.verticalCenter: parent.verticalCenter
                  width: parent.parent.width - scanQrButton.width - parent.spacing
                  wrapMode: Text.WordWrap
                  text: "Shows a code to scan with the Discord mobile app. No password, no captcha."
                  color: root.muted
                  font.family: root.fontFamily
                  font.pixelSize: Style.font.body
                }
              }
              Text {
                width: parent.width
                wrapMode: Text.WordWrap
                text: "Or paste a user token and press Enter. It goes straight to the backend and into the keyring; it is never written to disk or shown here."
                color: root.muted
                font.family: root.fontFamily
                font.pixelSize: Style.font.body
              }
              TextField {
                id: tokenField
                width: parent.width
                password: true
                placeholderText: "Discord user token"
                activeFocusOnTab: false
                enabled: !(root.service && root.service.loginBusy)
                onAccepted: root.submitToken()
              }
              Row {
                spacing: Style.spacing.controlGap
                Button {
                  id: loginButton
                  text: root.service && root.service.loginBusy ? "Logging in" : "Log in"
                  focusable: true
                  activeFocusOnTab: false
                  foreground: root.foreground
                  enabled: !(root.service && root.service.loginBusy)
                  onClicked: root.submitToken()
                }
                Text {
                  anchors.verticalCenter: parent.verticalCenter
                  text: "Terminal alternative: omarchy-discord-backend login"
                  color: root.muted
                  font.family: root.fontFamily
                  font.pixelSize: Style.font.caption
                }
              }
            }
          }

          // Three-column client
          Row {
            anchors.fill: parent
            visible: root.ready
            spacing: Style.spacing.panelGap

            // Guild rail
            BorderSurface {
              id: railPane
              width: Style.space(64)
              height: parent.height
              radius: Style.cornerRadius
              color: Color.popups.background
              borderSpec: root.focusedZone === "sidebar" && root.column === "rail"
                ? Border.controlSpec("focus", root.foreground, root.accent)
                : root.panelBorderSpec
              padding: Style.spacing.sm

              ListView {
                id: guildList
                anchors.fill: parent
                anchors.margins: Style.spacing.sm
                clip: true
                reuseItems: true
                cacheBuffer: Style.space(150)
                boundsBehavior: Flickable.StopAtBounds
                spacing: Style.spacing.sm
                model: root.guildRows.length
                ScrollBar.vertical: ScrollBar { policy: ScrollBar.AsNeeded }

                delegate: Item {
                  id: guildRow
                  required property int index
                  readonly property var row: root.guildRows[index] || ({})
                  readonly property bool isDms: String(row.id) === "dms"
                  readonly property bool hasCursor: root.focusedZone === "sidebar" && root.column === "rail"
                    && index === root.guildCursor
                  readonly property bool selected: String(row.id) === root.selectedGuildId
                  readonly property int mentions: Number(row.mention_count) || 0
                  readonly property bool unread: Api.isUnread(row)
                  width: guildList.width
                  height: Style.space(48)

                  // Selected-guild indicator along the left edge.
                  Rectangle {
                    anchors.left: parent.left
                    anchors.verticalCenter: parent.verticalCenter
                    width: Style.spacing.xs
                    height: guildRow.selected ? Style.space(28) : (guildRow.unread ? Style.spacing.lg : 0)
                    radius: width / 2
                    color: guildRow.selected ? root.accent : root.foreground
                    visible: height > 0
                    Behavior on height { NumberAnimation { duration: 120 } }
                  }

                  BorderSurface {
                    id: guildTile
                    anchors.centerIn: parent
                    width: Style.space(40)
                    height: width
                    radius: guildRow.selected || guildRow.hasCursor ? Style.cornerRadius + Style.spacing.sm : width / 2
                    color: guildRow.hasCursor
                      ? Style.hoverFillFor(root.foreground, root.accent)
                      : (guildRow.selected ? Style.selectedFillFor(root.foreground, root.accent)
                        : (guildMouse.containsMouse ? Style.hoverFillFor(root.foreground, root.accent)
                          : Style.normalFillFor(root.foreground, root.accent)))
                    borderSpec: guildRow.hasCursor
                      ? Border.controlSpec("hover-cursor", root.foreground, root.accent)
                      : Border.none()
                    Behavior on radius { NumberAnimation { duration: 120 } }

                    Text {
                      anchors.centerIn: parent
                      visible: !guildIconEffect.visible
                      text: guildRow.isDms ? "@" : Api.initials(guildRow.row.name)
                      color: guildRow.unread || guildRow.selected ? root.foreground : root.muted
                      font.family: root.fontFamily
                      font.pixelSize: guildRow.isDms ? Style.font.title : Style.font.bodySmall
                      font.bold: guildRow.unread
                    }

                    // Guild icon through the media cache, masked to the tile
                    // shape; the initials stay until it lands.
                    Rectangle {
                      id: guildIconMask
                      anchors.fill: parent
                      radius: guildTile.radius
                      visible: false
                      layer.enabled: true
                    }
                    Image {
                      id: guildIcon
                      anchors.fill: parent
                      visible: false
                      asynchronous: true
                      fillMode: Image.PreserveAspectCrop
                      sourceSize.width: width * 2
                      sourceSize.height: height * 2
                      readonly property string path: !guildRow.isDms && root.service && guildRow.row.icon_url
                        ? root.service.mediaPath(String(guildRow.row.icon_url), 64) : ""
                      source: path ? "file://" + path : ""
                      onStatusChanged: if (status === Image.Error && root.service) root.service.mediaError(path)
                    }
                    MultiEffect {
                      id: guildIconEffect
                      anchors.fill: guildIcon
                      source: guildIcon
                      maskEnabled: true
                      maskSource: guildIconMask
                      opacity: guildRow.unread || guildRow.selected || guildRow.hasCursor ? 1 : 0.7
                      visible: guildIcon.path !== "" && guildIcon.status === Image.Ready
                    }
                  }

                  // Mention badge, bottom-right of the tile.
                  Rectangle {
                    visible: guildRow.mentions > 0
                    anchors.right: guildTile.right
                    anchors.bottom: guildTile.bottom
                    anchors.margins: -Style.spacing.xxs
                    width: Math.max(height, guildBadge.implicitWidth + Style.spacing.sm * 2)
                    height: Style.space(16)
                    radius: height / 2
                    color: Color.urgent

                    Text {
                      id: guildBadge
                      anchors.centerIn: parent
                      text: guildRow.mentions > 99 ? "99+" : String(guildRow.mentions)
                      color: Color.popups.background
                      font.family: root.fontFamily
                      font.pixelSize: Style.font.caption
                      font.bold: true
                    }
                  }

                  MouseArea {
                    id: guildMouse
                    anchors.fill: parent
                    hoverEnabled: true
                    onClicked: {
                      root.zone = "sidebar"
                      root.column = "rail"
                      root.setGuildCursor(guildRow.index)
                      root.enterChannels()
                      root.focusZone()
                    }
                  }

                  PanelToolTip {
                    text: String(guildRow.row.name || "")
                    visible: guildMouse.containsMouse
                  }
                }
              }
            }

            // Channel list
            BorderSurface {
              id: channelPane
              width: Style.space(230)
              height: parent.height
              radius: Style.cornerRadius
              color: Color.popups.background
              borderSpec: root.focusedZone === "sidebar" && root.column === "channels"
                ? Border.controlSpec("focus", root.foreground, root.accent)
                : root.panelBorderSpec
              padding: Style.spacing.sm

              Column {
                anchors.fill: parent
                anchors.margins: Style.spacing.sm
                // The call bar is pinned to the bottom of this pane, so the
                // list is short by exactly its height while a call is up.
                anchors.bottomMargin: Style.spacing.sm
                  + (callBar.visible ? callBar.height + Style.spacing.sm : 0)
                spacing: Style.spacing.xs

                PanelSectionHeader {
                  width: parent.width
                  text: root.selectedGuildName || "Channels"
                  foreground: root.foreground
                }

                Text {
                  width: parent.width
                  visible: !root.channelRows.length
                  wrapMode: Text.WordWrap
                  text: !root.selectedGuildId ? "Pick a server with Enter or l."
                    : (root.channelsLoading ? "Loading channels"
                      : (root.dmsSelected ? "No direct messages." : "No text channels."))
                  color: root.muted
                  font.family: root.fontFamily
                  font.pixelSize: Style.font.body
                  leftPadding: Style.spacing.rowPaddingX
                }

                ListView {
                  id: channelList
                  width: parent.width
                  height: parent.height - Style.spacing.controlHeight
                  visible: root.channelRows.length > 0
                  clip: true
                  reuseItems: true
                  cacheBuffer: Style.space(150)
                  boundsBehavior: Flickable.StopAtBounds
                  spacing: Style.spacing.xxs
                  model: root.channelRows.length
                  ScrollBar.vertical: ScrollBar { policy: ScrollBar.AsNeeded }

                  delegate: Item {
                    id: channelRow
                    required property int index
                    readonly property var row: root.channelRows[index] || ({})
                    readonly property string type: String(row.type || "")
                    readonly property bool category: type === "category"
                    readonly property bool hasCursor: root.focusedZone === "sidebar" && root.column === "channels"
                      && index === root.channelCursor && !category
                    readonly property bool open: String(row.id || "") === root.currentChannelId
                    readonly property bool unread: Api.isUnread(row)
                    readonly property int mentions: Number(row.mention_count) || 0
                    readonly property bool voice: type === "voice"
                    // The call is on this row (Enter focuses the call bar
                    // instead of joining again).
                    readonly property bool joined: voice && root.activeVoiceChannelId !== ""
                      && root.activeVoiceChannelId === String(row.id || "")
                    // Occupants come straight off voice_members: each user
                    // carries its own name and avatar, so nothing is resolved.
                    readonly property var occupants: voice && root.service
                      ? root.service.voiceUsers(root.selectedGuildId, String(row.id || "")) : []
                    readonly property bool dim: !!row.muted || (!unread && !open && !joined)
                    readonly property bool thread: type === "thread"
                    readonly property int threadCount: Api.hasThreads(type) ? (Number(root.threadCounts[String(row.id || "")]) || 0) : 0
                    readonly property bool expanded: threadCount > 0 && !!root.expandedThreads[String(row.id || "")]
                    width: channelList.width
                    height: (category ? Style.spacing.controlHeight : Style.spacing.popupRowHeight)
                      + channelRow.occupants.length * root.voiceOccupantHeight

                    PanelSectionHeader {
                      visible: channelRow.category
                      anchors.left: parent.left
                      anchors.right: parent.right
                      anchors.bottom: parent.bottom
                      text: String(channelRow.row.name || "").toUpperCase()
                      foreground: root.foreground
                    }

                    BorderSurface {
                      id: channelSurface
                      visible: !channelRow.category
                      anchors.left: parent.left
                      anchors.right: parent.right
                      anchors.top: parent.top
                      height: Style.spacing.popupRowHeight
                      radius: Style.cornerRadius
                      color: channelRow.hasCursor || channelMouse.containsMouse
                        ? Style.hoverFillFor(root.foreground, root.accent)
                        : (channelRow.open || channelRow.joined
                          ? Style.selectedFillFor(root.foreground, root.accent) : "transparent")
                      borderSpec: channelRow.hasCursor
                        ? Border.controlSpec("hover-cursor", root.foreground, root.accent)
                        : Border.none()

                      Text {
                        id: channelGlyph
                        anchors.left: parent.left
                        anchors.leftMargin: Style.spacing.rowPaddingX
                          + (channelRow.row.parent_id && !root.dmsSelected ? Style.spacing.lg : 0)
                          + (channelRow.thread ? Style.spacing.lg : 0)
                        anchors.verticalCenter: parent.verticalCenter
                        text: Api.channelGlyph(channelRow.type)
                        color: channelRow.dim ? root.muted : root.foreground
                        font.family: root.fontFamily
                        font.pixelSize: Style.font.body
                      }
                      Text {
                        anchors.left: channelGlyph.right
                        anchors.right: rowHint.visible ? rowHint.left : (channelBadge.visible ? channelBadge.left
                          : (channelDot.visible ? channelDot.left : parent.right))
                        anchors.leftMargin: Style.spacing.sm
                        anchors.rightMargin: Style.spacing.sm
                        anchors.verticalCenter: parent.verticalCenter
                        text: String(channelRow.row.name || "")
                        elide: Text.ElideRight
                        color: channelRow.dim ? root.muted : root.foreground
                        font.family: root.fontFamily
                        font.pixelSize: Style.font.body
                        font.bold: channelRow.unread
                      }
                      // The right-hand count: "⌥ N threads" on a channel
                      // that has them (t lists them beneath), the occupant
                      // count on a voice channel. A voice channel never
                      // carries threads, so the slot is never contested.
                      Text {
                        id: rowHint
                        anchors.right: channelBadge.visible ? channelBadge.left
                          : (channelDot.visible ? channelDot.left : parent.right)
                        anchors.rightMargin: Style.spacing.sm
                        anchors.verticalCenter: parent.verticalCenter
                        visible: channelRow.threadCount > 0 || channelRow.occupants.length > 0
                        text: channelRow.occupants.length > 0
                          ? String(channelRow.occupants.length)
                          : (channelRow.expanded ? "▼ " : "⌥ ") + channelRow.threadCount
                            + (channelRow.threadCount === 1 ? " thread" : " threads")
                        color: root.muted
                        font.family: root.fontFamily
                        font.pixelSize: Style.font.caption
                      }
                      Rectangle {
                        id: channelDot
                        anchors.right: parent.right
                        anchors.rightMargin: Style.spacing.rowPaddingX
                        anchors.verticalCenter: parent.verticalCenter
                        visible: channelRow.unread && channelRow.mentions === 0
                        width: Style.spacing.lg
                        height: width
                        radius: width / 2
                        color: root.foreground
                      }
                      Rectangle {
                        id: channelBadge
                        anchors.right: parent.right
                        anchors.rightMargin: Style.spacing.sm
                        anchors.verticalCenter: parent.verticalCenter
                        visible: channelRow.mentions > 0
                        width: Math.max(height, channelBadgeText.implicitWidth + Style.spacing.sm * 2)
                        height: Style.space(16)
                        radius: height / 2
                        color: Color.urgent

                        Text {
                          id: channelBadgeText
                          anchors.centerIn: parent
                          text: channelRow.mentions > 99 ? "99+" : String(channelRow.mentions)
                          color: Color.popups.background
                          font.family: root.fontFamily
                          font.pixelSize: Style.font.caption
                          font.bold: true
                        }
                      }
                      MouseArea {
                        id: channelMouse
                        anchors.fill: parent
                        hoverEnabled: true
                        onClicked: {
                          root.zone = "sidebar"
                          root.column = "channels"
                          root.activateChannel(channelRow.index)
                          // A forum row only expands; the zone must still
                          // take the keyboard.
                          root.focusZone()
                        }
                      }
                    }

                    // Who is in the voice channel, indented beneath it. The
                    // ring is voice_speaking; the rows are not focus stops
                    // (there is nothing to do to a participant).
                    Column {
                      anchors.top: channelSurface.bottom
                      anchors.left: parent.left
                      anchors.right: parent.right
                      visible: channelRow.occupants.length > 0

                      Repeater {
                        model: channelRow.occupants.length

                        delegate: Item {
                          id: occupantRow
                          required property int index
                          readonly property var user: channelRow.occupants[index] || ({})
                          readonly property string displayName: String(user.display_name || user.username || "")
                          readonly property bool talking: !!(root.service
                            && root.service.speaking[String(user.id || "")])
                          width: channelList.width
                          height: root.voiceOccupantHeight

                          Item {
                            id: occupantAvatar
                            anchors.left: parent.left
                            anchors.leftMargin: Style.spacing.rowPaddingX + Style.spacing.lg * 2
                            anchors.verticalCenter: parent.verticalCenter
                            width: root.voiceAvatarSize + Style.spacing.xxs * 2
                            height: width

                            Rectangle {
                              anchors.fill: parent
                              radius: width / 2
                              color: "transparent"
                              border.width: occupantRow.talking ? Style.spacing.hairline * 2 : 0
                              border.color: root.accent
                            }
                            Components.Avatar {
                              anchors.centerIn: parent
                              width: root.voiceAvatarSize
                              height: root.voiceAvatarSize
                              service: root.service
                              url: String(occupantRow.user.avatar_url || "")
                              name: occupantRow.displayName
                              foreground: root.foreground
                              fontFamily: root.fontFamily
                            }
                          }
                          Text {
                            anchors.left: occupantAvatar.right
                            anchors.right: parent.right
                            anchors.leftMargin: Style.spacing.sm
                            anchors.rightMargin: Style.spacing.rowPaddingX
                            anchors.verticalCenter: parent.verticalCenter
                            text: occupantRow.displayName
                            elide: Text.ElideRight
                            color: occupantRow.talking ? root.foreground : root.muted
                            font.family: root.fontFamily
                            font.pixelSize: Style.font.caption
                          }
                        }
                      }
                    }
                  }
                }
              }

              // The call, while there is one: pinned to the bottom of the
              // channel column (the list above shrinks by exactly its
              // height), so the channel you are reading and the channel you
              // are talking in can differ without either one hiding, and the
              // composer is never crowded.
              Components.CallBar {
                id: callBar
                anchors.left: parent.left
                anchors.right: parent.right
                anchors.bottom: parent.bottom
                anchors.leftMargin: Style.spacing.sm
                anchors.rightMargin: Style.spacing.sm
                anchors.bottomMargin: Style.spacing.sm
                service: root.service
                channelName: root.service && root.service.voice
                  ? String(root.service.channelNames[String(root.service.voice.channelId || "")] || "")
                  : ""
                foreground: root.foreground
                secondary: root.muted
                accent: root.accent
                fontFamily: root.fontFamily
                // Hanging up while the bar holds the keyboard (Tab'd here
                // from the timeline, or Ctrl+Shift+H) takes the focused item
                // out from under the focus: hand it back to the zone, the
                // same place Esc would have put it.
                onVisibleChanged: if (!visible && activeFocus) root.focusZone()
              }
            }

            // Timeline column
            Column {
              width: parent.width - railPane.width - channelPane.width - parent.spacing * 2
                - (membersView.visible ? membersView.width + parent.spacing : 0)
              height: parent.height
              spacing: Style.spacing.xs

              // Channel header: name + topic on the left, the panel controls
              // (status + Members / Log out / Close) right-aligned on the
              // same row.
              Item {
                id: channelHeader
                width: parent.width
                height: root.controlsRowHeight
                // The controls cannot overflow this row (controlsInHeader
                // hosts them elsewhere long before that), but they are
                // reparented in and out of it and a single frame of stale
                // geometry would paint over the channel list and the rail.
                clip: true

                Text {
                  id: channelTitle
                  anchors.left: parent.left
                  anchors.leftMargin: Style.spacing.sm
                  anchors.verticalCenter: parent.verticalCenter
                  // Thread titles ("#parent › thread") can be long: elide
                  // before the controls, leaving the topic what is left.
                  // channelTitleFloor keeps this above zero while the
                  // controls are hosted here; the clamp covers the rest.
                  width: Math.min(implicitWidth, Math.max(0, parent.width
                    - Style.spacing.sm * 2 - channelControlsSlot.width
                    - Style.spacing.controlGap))
                  elide: Text.ElideRight
                  text: root.currentChannelTitle || "No channel open"
                  color: root.currentChannelId ? root.foreground : root.muted
                  font.family: root.fontFamily
                  font.pixelSize: Style.font.title
                  font.bold: !!root.currentChannelId
                }
                Text {
                  anchors.left: channelTitle.right
                  anchors.right: channelControlsSlot.left
                  anchors.leftMargin: Style.spacing.controlGap
                  anchors.rightMargin: Style.spacing.controlGap
                  anchors.verticalCenter: parent.verticalCenter
                  visible: root.currentTopic !== "" && width > 0
                  text: "· " + root.currentTopic
                  elide: Text.ElideRight
                  color: root.muted
                  font.family: root.fontFamily
                  font.pixelSize: Style.font.bodySmall
                }
                // Reserved seat for the panel controls, declared last so they
                // paint above the title and topic. Zero-width while the
                // controls live in the top strip, and always present so the
                // two Texts have a legal sibling to anchor against.
                Item {
                  id: channelControlsSlot
                  anchors.right: parent.right
                  anchors.rightMargin: Style.spacing.sm
                  anchors.verticalCenter: parent.verticalCenter
                  width: root.controlsInHeader ? headerControls.width : 0
                  height: headerControls.height
                }
              }

              Components.Timeline {
                id: timelineView
                width: parent.width
                height: parent.height - channelHeader.height - typingLine.height
                  - composerView.height - parent.spacing * 3
                messages: root.currentMessages
                hasMore: !!(root.currentEntry && root.currentEntry.hasMore)
                loading: !!(root.currentEntry && root.currentEntry.loading)
                channelId: root.currentChannelId
                selfId: root.service ? root.service.selfId : ""
                lastReadMessageId: root.currentEntry ? String(root.currentEntry.unreadMarkerId || "") : ""
                active: root.focusedZone === "timeline" && root.ready
                viewing: (root.focusedZone === "timeline" || root.focusedZone === "composer") && root.ready
                ctx: root.service ? root.service.markdownCtx : ({})

                onRequestHistory: function(beforeId) {
                  if (root.service) root.service.loadHistory(root.currentChannelId)
                }
                onEscapeRequested: root.leaveTimeline(true)
                onMoveZone: function(direction) { root.moveZone(direction) }
                onOpenLink: function(url) { Quickshell.execDetached(["xdg-open", String(url)]) }
                onCopied: if (root.service) root.service.succeed("Copied to clipboard")
                onLinkCopied: if (root.service) root.service.succeed("Copied link to clipboard")
                onReachedBottom: if (root.opened && (root.focusedZone === "timeline" || root.focusedZone === "composer")) ackTimer.restart()
                onActiveFocusChanged: if (activeFocus && root.zone !== "timeline") root.zone = "timeline"
                onPinnedChanged: root.publishPinned()
                onReplyRequested: function(messageId) { root.replyTo(messageId) }
                onDeleteRequested: function(messageId) {
                  if (root.service) root.service.deleteMessage(root.currentChannelId, messageId)
                }
                onReactRequested: function(messageId) { root.openPicker(messageId) }
                onReactionToggled: function(messageId, emoji) { root.applyReaction(messageId, emoji) }
              }

              Text {
                id: typingLine
                width: parent.width
                height: Style.font.caption * 1.6
                leftPadding: Style.spacing.sm
                verticalAlignment: Text.AlignVCenter
                text: root.typingText
                elide: Text.ElideRight
                color: root.muted
                font.family: root.fontFamily
                font.pixelSize: Style.font.caption
                font.italic: true
              }

              Components.Composer {
                id: composerView
                width: parent.width
                height: implicitHeight
                service: root.service
                channelId: root.currentChannelId
                channelName: root.currentChannel
                  ? Api.channelGlyph(root.currentChannel.type) + String(root.currentChannel.name || "") : ""
                active: root.focusedZone === "composer" && root.ready

                onLeave: root.leaveComposer(true)
                onMoveZone: function(direction) { root.moveZone(direction) }
                onCycleFocus: function(delta) { root.cycleFocus(delta) }
                onSwitcherRequested: root.openSwitcher()
                onCheatsheetRequested: root.toggleCheatsheet()
                onMembersRequested: root.toggleMembers()
                onVoiceRequested: function(action) { root.voiceAction(action) }
                onActiveFocusChanged: if (activeFocus && root.zone !== "composer") root.zone = "composer"
              }
            }

            // Member pane (m): a zone only while visible.
            Components.MemberList {
              id: membersView
              visible: root.membersVisible
              width: Style.space(220)
              height: parent.height
              service: root.service
              list: root.service && root.service.memberList
                && String(root.service.memberList.channel_id || "") === root.currentChannelId
                ? root.service.memberList : null
              loading: !!(root.service && root.service.membersChannelId === root.currentChannelId
                && root.service.memberList === null && !root.service.membersTimedOut)
              timedOut: !!(root.service && root.service.membersTimedOut)
              active: root.focusedZone === "members" && root.ready
              onEscapeRequested: root.leaveMembers()
              onMoveZone: function(direction) { root.moveZone(direction) }
              onCopied: if (root.service) root.service.succeed("Copied @username to clipboard")
              onActiveFocusChanged: if (activeFocus && root.zone !== "members") root.zone = "members"
            }
          }
        }

        // Footer: key hints + redacted error
        Column {
          id: footer
          width: parent.width
          spacing: Style.spacing.xs

          Text {
            width: parent.width
            visible: root.errorText !== ""
            wrapMode: Text.WordWrap
            text: root.errorText
            color: Color.urgent
            font.family: root.fontFamily
            font.pixelSize: Style.font.bodySmall
          }
          Text {
            width: parent.width
            visible: !!(root.service && root.service.notice !== "")
            wrapMode: Text.WordWrap
            text: root.service ? root.service.notice : ""
            color: root.muted
            font.family: root.fontFamily
            font.pixelSize: Style.font.bodySmall
          }
          Text {
            width: parent.width
            visible: root.hint !== ""
            text: root.hint
            color: root.muted
            font.family: root.fontFamily
            font.pixelSize: Style.font.bodySmall
          }
          Text {
            width: parent.width
            visible: root.service && root.service.statusMessage !== ""
            text: root.service ? root.service.statusMessage : ""
            color: root.accent
            font.family: root.fontFamily
            font.pixelSize: Style.font.bodySmall
          }
          // Key hints come from the same table as the cheatsheet (Keymap.js),
          // so the two cannot drift.
          Text {
            width: parent.width
            elide: Text.ElideRight
            text: {
              if (!root.ready) {
                if (root.qrView) {
                  if (root.qrMissing) return Keymap.footer("qrMissing")
                  return Keymap.footer(root.qrCancelable ? "qrRunning" : "qrDone")
                }
                return Keymap.footer(root.showLogin ? "login" : "down")
              }
              if (root.callBarFocused) return Keymap.footer("voice")
              if (root.buttonFocused) {
                // Esc goes where focusZone() goes: the last zone, unless it
                // needs an open channel that is gone.
                var back = (root.zone === "timeline" || root.zone === "composer" || root.zone === "members") && !root.currentChannelId ? "sidebar"
                  : (root.zone === "composer" ? "the composer" : (root.zone === "members" ? "the member list" : root.zone))
                return Keymap.footer("global.activate", "global.tabCycle", { id: "global.escBack", hint: "back to " + back })
              }
              if (root.zone === "composer") {
                if (root.composer.chipFocused) return Keymap.footer("chips")
                if (root.composer.editing) return Keymap.footer("composerEdit")
                return Keymap.footer("composer", root.composer.chips.length ? "composerChips" : "",
                  root.membersVisible ? "composerMembers" : "", "composerTail")
              }
              if (root.zone === "members") return Keymap.footer("members")
              if (root.zone === "timeline")
                return Keymap.footer(root.timeline.hasSelection ? "timelineSelection" : "", "timeline",
                  root.canToggleCurrentThreads ? "timelineThreads" : "", "timelineTail")
              if (root.column === "rail") return Keymap.footer("rail")
              return Keymap.footer("channels", root.currentChannelId ? "channelsTimeline" : "",
                root.currentChannelId ? "channelsMembers" : "", "channelsTail")
            }
            color: root.muted
            font.family: root.fontFamily
            font.pixelSize: Style.font.caption
          }
        }
      }
    }
  }
}
