pragma ComponentBehavior: Bound
import QtQuick
import QtQuick.Controls
import QtQuick.Window
import "ui"

import "Api.js" as Api
import "Keymap.js" as Keymap
import "components" as Components

Item {
  id: root

  property var shell: null
  property var manifest: null
  property var service: null
  property bool hostOpened: false
  readonly property bool opened: hostOpened
  property bool windowActive: false
  property bool mapped: false
  readonly property bool reading: opened && mapped && windowActive
  property string screenName: ""
  signal closeRequested()
  signal linkRequested(string url)
  signal copyRequested(string text)
  readonly property alias timeline: timelineView
  readonly property alias composer: composerView
  readonly property alias cheatsheet: cheatsheetView
  readonly property alias picker: pickerView
  readonly property alias members: membersView
  readonly property alias controls: headerControls
  readonly property bool qrImageReady: qrImage.status === Image.Ready
  readonly property alias logoutConfirmation: logoutConfirm
  readonly property bool overlayShown: cheatsheetView.shown || pickerView.shown || logoutConfirm.shown
  readonly property bool compactMembers: width < Style.space(900)

  readonly property string pluginId: manifest && manifest.id
    ? String(manifest.id) : "quickshell.discord"
  readonly property color foreground: Color.foreground
  readonly property color muted: Api.secondaryColor(Color.muted, Color.foreground, Color.background)
  readonly property color background: Color.background
  readonly property color accent: Color.accent
  readonly property string fontFamily: Style.font.family
  readonly property int voiceAvatarSize: Style.space(16)
  readonly property int voiceOccupantHeight: Style.space(22)
  readonly property var panelBorderSpec: Border.flat(Color.popups.border,
    Math.max(1, Style.normalBorderWidth))
  readonly property real controlsRowHeight: Math.max(Style.spacing.controlHeight, headerControls.height)
  readonly property real controlButtonsWidth:
    (currentChannelId !== "" ? membersButton.width + Style.spacing.controlGap : 0)
    + searchButton.width + helpButton.width + Style.spacing.controlGap * 2
    + logoutButton.width + Style.spacing.controlGap
    + closeButton.width
  readonly property real channelHeaderRoom: channelHeader.width - Style.spacing.sm * 2
    - Style.spacing.controlGap
  readonly property real channelTitleFloor: Style.space(120)
  readonly property bool controlsInHeader: ready
    && channelHeaderRoom - controlButtonsWidth >= channelTitleFloor
  readonly property real statusWidthBudget: {
    if (!controlsInHeader) return Style.space(200)
    var room = channelHeaderRoom - controlButtonsWidth - channelTitleFloor
      - Style.spacing.controlGap
    return room >= Style.space(80) ? Math.min(Style.space(200), room) : 0
  }

  readonly property string lifecycle: service ? service.lifecycle : ""
  readonly property bool connected: !!(service && service.connected)
  readonly property bool ready: !!(service && service.showStructure)
  readonly property bool showLogin: connected
    && (lifecycle === "logged_out" || lifecycle === "reauth_needed" || lifecycle === "qr_pending")
  readonly property string errorText: service ? Api.redact(service.lastError) : ""
  readonly property var qr: service ? service.qr : null
  readonly property string qrStage: qr ? String(qr.stage || "") : ""
  readonly property bool qrView: showLogin && (lifecycle === "qr_pending"
    || !!(service && service.qrBusy) || qr !== null)
  readonly property bool qrCancelable: lifecycle === "qr_pending" && qrStage !== "approved"
  readonly property bool qrMissing: !!(service && service.qrMissing)
  property int qrSecondsLeft: 0

  property string zone: "sidebar"
  property string column: "rail"
  readonly property bool buttonFocused: logoutButton.activeFocus || closeButton.activeFocus
    || membersButton.activeFocus || searchButton.activeFocus || helpButton.activeFocus
    || startBackendButton.activeFocus || callBarFocused
  readonly property bool callBarFocused: callBar.visible && callBar.activeFocus
  readonly property string activeVoiceChannelId: {
    if (!service || !service.voice) return ""
    var status = String(service.voice.status || "")
    if (status !== "connected" && status !== "connecting") return ""
    return String(service.voice.channelId || "")
  }
  readonly property bool membersVisible: !!(service && service.membersWanted) && currentChannelId !== "" && ready
  property var expandedThreads: ({})
  readonly property bool canToggleCurrentThreads: currentThreadParent() !== null
  readonly property string focusedZone: buttonFocused ? "" : zone
  readonly property bool composerFocused: composerView.activeFocus
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

  function openSwitcher() {
    if (service) service.openSwitcher()
  }

  function toggleCheatsheet() {
    if (cheatsheetView.shown) cheatsheetView.hide()
    else { pickerView.hide(); cheatsheetView.show() }
  }

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
    else if (!textInputFocused() && !showLogin && text === "/") openSwitcher()
    else if (!textInputFocused() && !showLogin && text === "?") toggleCheatsheet()
    else return false
    event.accepted = true
    return true
  }

  function publishActive() {
    if (service) service.panelActive = windowActive
  }

  function publishMapped() {
    if (service) service.panelMapped = mapped
  }

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

  function updateQrCountdown() {
    qrSecondsLeft = qr && qr.expiresAt ? Math.max(0, Math.ceil((Number(qr.expiresAt) - Date.now()) / 1000)) : 0
  }

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
    hostOpened = true
    if (service) service.refresh()
    restoreView()
    if (requested && service) {
      service.showChannel(requested, guildIdForChannel(requested))
      enterComposer()
    }
  }

  function close() {
    logoutConfirm.shown = false
    tokenField.clear()
    cheatsheetView.shown = false
    pickerView.shown = false
    hostOpened = false
  }





  function publishPinned() {
    if (service) service.timelinePinned = !opened || timelineView.pinned
  }

  function publishScreen() {
    if (!service) return
    service.panelScreenName = opened ? screenName : ""
  }

  function requestClose() {
    closeRequested()
  }

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
    var at = indexOfId(channelRows, currentChannelId)
    if (at < 0) at = findChannel(-1, 1, Api.isSelectableChannel)
    if (at >= 0) setChannelCursor(at)
    else channelCursorId = ""
  }

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
    var target = index >= 0 && index < guildRows.length ? String(guildRows[index].id || "") : ""
    var owned = target !== "" && target === selectedGuildId && currentChannelId !== ""
    selectGuild(index)
    zone = "sidebar"
    column = "channels"
    hint = ""
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

  function activateChannel(index, origin) {
    var row = channelRows[index]
    if (!row || !service) return
    if (String(row.type || "") === "forum") { toggleThreads(index); return }
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

  function voiceAction(action) {
    if (!service) return
    if (action === "mute") service.toggleMute()
    else if (action === "deafen") service.toggleDeafen()
    else if (action === "leave") service.voiceLeave()
  }

  function toggleThreads(index) {
    var row = channelRows[index]
    if (!row || !service || dmsSelected) return
    var id = String(row.id || "")
    if (String(row.type || "") === "thread") id = String(row.parent_id || "")
    toggleThreadsFor(id, String(row.id || ""))
  }

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
    channelCursorId = next[id] ? String(cursorId || id) : id
  }

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

  function toggleMembers() {
    if (!service) return
    if (!currentChannelId) { hint = "Open a channel first"; return }
    service.setMembersWanted(!service.membersWanted)
    hint = ""
    if (service.membersWanted && compactMembers) enterMembers()
    if (!service.membersWanted && zone === "members") { zone = "composer"; focusZone() }
  }

  function enterMembers() {
    if (!membersVisible) return
    zone = "members"
    hint = ""
    focusZone()
  }

  function leaveMembers() {
    if (compactMembers && service) service.setMembersWanted(false)
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

  function leaveComposer(markRead) {
    if (markRead && service && currentChannelId) service.markChannelRead(currentChannelId)
    enterTimeline()
  }

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

  function stepChannel(delta, unreadOnly) {
    var from = indexOfId(channelRows, currentChannelId)
    if (from < 0) from = channelCursor >= 0 ? channelCursor - delta : (delta > 0 ? -1 : 0)
    var accept = unreadOnly
      ? function(row) { return Api.isOpenableChannel(row) && Api.isUnread(row) }
      : Api.isOpenableChannel
    var next = findChannel(from, delta, accept)
    if (next < 0 || next === from) return
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

  function cycleFocus(delta) {
    var stops = ["rail", "channels", "timeline", "composer", "callbar", "members", "startBackend",
      "search", "help", "membersButton", "logout", "close"]
    var current = callBarFocused ? "callbar"
      : (buttonFocused
        ? (closeButton.activeFocus ? "close" : searchButton.activeFocus ? "search" : helpButton.activeFocus ? "help"
          : (membersButton.activeFocus ? "membersButton"
            : (startBackendButton.activeFocus ? "startBackend" : "logout")))
        : (zone === "sidebar" ? column : zone))
    var index = stops.indexOf(current)
    for (var step = 0; step < stops.length; step++) {
      index = clampCursor(index + delta, stops.length)
      var stop = stops[index]
      if ((stop === "rail" || stop === "channels") && !ready) continue
      if ((stop === "timeline" || stop === "composer") && !currentChannelId) continue
      if (stop === "callbar" && !(ready && callBar.visible)) continue
      if (stop === "members" && !membersVisible) continue
      if (stop === "startBackend" && !startBackendButton.visible) continue
      if (stop === "membersButton" && !membersButton.visible) continue
      if (stop === "search" && !searchButton.visible) continue
      if (stop === "help" && !helpButton.visible) continue
      if (stop === "logout" && !logoutButton.visible) continue
      focusStop(stop, delta)
      return
    }
  }

  function focusStop(stop, delta) {
    hint = ""
    if (stop === "search") { searchButton.forceActiveFocus(); return }
    if (stop === "help") { helpButton.forceActiveFocus(); return }
    if (stop === "logout") { logoutButton.forceActiveFocus(); return }
    if (stop === "close") { closeButton.forceActiveFocus(); return }
    if (stop === "membersButton") { membersButton.forceActiveFocus(); return }
    if (stop === "startBackend") { startBackendButton.forceActiveFocus(); return }
    if (stop === "callbar") { focusCallBar(); return }
    if (stop === "members") { enterMembers(); return }
    if (stop === "channels") { enterChannels(); return }
    if (stop === "composer") {
      zone = "composer"
      if (delta < 0 && composerView.chips.length) composerView.focusChip(composerView.chips.length - 1)
      else composerView.focusInput()
      return
    }
    if (stop === "timeline") zone = "timeline"
    else { zone = "sidebar"; column = "rail" }
    focusZone()
  }

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

  function handleKey(event) {
    var key = event.key
    var text = event.text
    var alt = (event.modifiers & Qt.AltModifier) !== 0
    var shift = (event.modifiers & Qt.ShiftModifier) !== 0
    if (overlayShown) return
    if (handleGlobalKey(event)) return
    if (showLogin) {
      if (key === Qt.Key_Tab || key === Qt.Key_Backtab) cycleLoginFocus(key === Qt.Key_Backtab || shift ? -1 : 1)
      else if (key === Qt.Key_Escape) { if (qrView) leaveQr(); else root.requestClose() }
      else return
      event.accepted = true
      return
    }
    if (tokenField.activeFocus) return
    if (!ready) {
      if (key === Qt.Key_Tab || key === Qt.Key_Backtab) cycleFocus(key === Qt.Key_Backtab || shift ? -1 : 1)
      else if (key === Qt.Key_Escape) root.requestClose()
      else if (text === "r") retry()
      else return
      event.accepted = true
      return
    }
    if (composerFocused) {
      if (alt && key === Qt.Key_Down) stepChannel(1, shift)
      else if (alt && key === Qt.Key_Up) stepChannel(-1, shift)
      else return
      event.accepted = true
      return
    }
    if (key === Qt.Key_Tab || key === Qt.Key_Backtab) cycleFocus(key === Qt.Key_Backtab || shift ? -1 : 1)
    else if (buttonFocused) {
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

  function dispatchKey(event) {
    if (logoutConfirm.shown) {
      if (event.key === Qt.Key_Escape) logoutConfirm.hide()
      event.accepted = true
      return true
    }
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
  onQrViewChanged: if (showLogin && opened) Qt.callLater(focusLogin)
  onWindowActiveChanged: publishActive()
  onReadingChanged: if (reading && ready && timelineView.pinned
    && (focusedZone === "timeline" || focusedZone === "composer")) ackTimer.restart()
  onOpenedChanged: opened ? enter() : leave()

  onMappedChanged: publishMapped()
  onScreenNameChanged: publishScreen()
  Component.onCompleted: publishMapped()
  onQrChanged: updateQrCountdown()
  onReadyChanged: {
    if (!ready) { zone = "sidebar"; column = "rail" }
    if (opened) focusZone()
  }
  onMembersVisibleChanged: if (!membersVisible && zone === "members" && opened) { zone = "composer"; focusZone() }
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

  Timer {
    id: ackTimer
    interval: 500
    onTriggered: {
      if (!root.reading || !root.currentChannelId) return
      if (root.focusedZone !== "timeline" && root.focusedZone !== "composer") return
      if (!timelineView.pinned || !root.service) return
      root.service.markChannelRead(root.currentChannelId)
    }
  }

    FocusScope {
      id: focusScope
      objectName: "client-content"
      anchors.fill: parent
      focus: true
      Keys.priority: Keys.BeforeItem
      Keys.onPressed: function(event) { root.handleKey(event) }

      Item {
        id: sidebarFocus
        focus: true
        width: 0
        height: 0
      }

      Row {
        id: headerControls
        parent: root.controlsInHeader ? channelControlsSlot : topControlsSlot
        spacing: Style.spacing.controlGap

        Text {
          anchors.verticalCenter: parent.verticalCenter
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

        Button {
          id: searchButton
          objectName: "search-button"
          visible: root.ready
          text: "Search"
          focusable: true
          activeFocusOnTab: false
          tooltipText: "Find a channel or direct message (Ctrl+K)"
          onClicked: root.openSwitcher()
        }
        Button {
          id: helpButton
          objectName: "help-button"
          text: "Help"
          focusable: true
          activeFocusOnTab: false
          tooltipText: "Keyboard shortcuts (Ctrl+/)"
          onClicked: root.toggleCheatsheet()
        }
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
          onClicked: logoutConfirm.show()
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

      Connections {
        target: root.service
        ignoreUnknownSignals: true
        function onGuildEntered(guildId, channelId) {
          if (guildId !== root.selectedGuildId) return
          var at = root.indexOfId(root.channelRows, channelId)
          if (at >= 0) root.setChannelCursor(at)
        }
      }

      Components.LogoutConfirm {
        id: logoutConfirm
        anchors.fill: parent
        z: 20
        onConfirmed: if (root.service) root.service.logout()
        onDismissed: Qt.callLater(function() { logoutButton.forceActiveFocus() })
      }

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

        Item {
          id: body
          width: parent.width
          height: parent.height - footer.height - parent.spacing
            - (topStrip.visible ? topStrip.height + parent.spacing : 0)

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
              activeFocusOnTab: false
              foreground: root.foreground
              onClicked: if (root.service) root.service.startBackend()
            }
          }

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

            Column {
              id: qrColumn
              width: parent.width
              visible: root.qrView
              spacing: Style.spacing.lg

              Item {
                id: qrFrame
                anchors.horizontalCenter: parent.horizontalCenter
                width: Style.space(256)
                height: width
                visible: root.qrStage === "code" || root.qrStage === ""

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
                    ? Util.fileUrl(String(root.qr.imagePath)) + "?r=" + String(root.qr.revision || 0) : ""
                }
              }

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

          Row {
            id: conversationRow
            anchors.fill: parent
            visible: root.ready
            spacing: Style.spacing.panelGap

            BorderSurface {
              id: railPane
              objectName: "server-rail"
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
                    radius: Style.cornerRadius
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
                      text: guildRow.isDms ? "@" : Api.initials(guildRow.row.name)
                      color: guildRow.unread || guildRow.selected ? root.foreground : root.muted
                      font.family: root.fontFamily
                      font.pixelSize: guildRow.isDms ? Style.font.title : Style.font.bodySmall
                      font.bold: guildRow.unread
                    }

                  }

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

            BorderSurface {
              id: channelPane
              objectName: "channel-pane"
              width: Math.min(Style.space(230), Math.max(Style.space(150), body.width * 0.23))
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
                    readonly property bool joined: voice && root.activeVoiceChannelId !== ""
                      && root.activeVoiceChannelId === String(row.id || "")
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
                          root.focusZone()
                        }
                      }
                    }

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
                           readonly property string displayName: Api.userLabel(user,
                             root.service ? root.service.knownUsers : null)
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
                               name: String(occupantRow.user.display_name || occupantRow.user.username || occupantRow.user.id || "?")
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
                onVisibleChanged: if (!visible && activeFocus) root.focusZone()
              }
            }

            Column {
              width: parent.width - railPane.width - channelPane.width - parent.spacing * 2
                - (root.membersVisible && !root.compactMembers ? membersView.width + parent.spacing : 0)
              height: parent.height
              spacing: Style.spacing.xs

              Item {
                id: channelHeader
                width: parent.width
                height: root.controlsRowHeight
                clip: true

                Text {
                  id: channelTitle
                  anchors.left: parent.left
                  anchors.leftMargin: Style.spacing.sm
                  anchors.verticalCenter: parent.verticalCenter
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
                objectName: "timeline"
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
                viewing: root.reading && (root.focusedZone === "timeline" || root.focusedZone === "composer") && root.ready
                ctx: root.service ? root.service.markdownCtx : ({})

                onRequestHistory: function(beforeId) {
                  if (root.service) root.service.loadHistory(root.currentChannelId)
                }
                onCycleFocus: function(delta) { root.cycleFocus(delta) }
                onEscapeRequested: root.leaveTimeline(true)
                onMoveZone: function(direction) { root.moveZone(direction) }
                onOpenLink: function(url) { root.linkRequested(String(url)) }
                onCopyRequested: function(text) { root.copyRequested(text) }
                onCopied: if (root.service) root.service.succeed("Copied to clipboard")
                onLinkCopied: if (root.service) root.service.succeed("Copied link to clipboard")
                onReachedBottom: if (root.reading && (root.focusedZone === "timeline" || root.focusedZone === "composer")) ackTimer.restart()
                onActiveFocusChanged: if (activeFocus && root.zone !== "timeline") root.zone = "timeline"
                onPinnedChanged: root.publishPinned()
                onReplyRequested: function(messageId) { root.replyTo(messageId) }
                onEditRequested: function(messageId) {
                  if (composerView.startEdit(messageId)) root.enterComposer()
                }
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
                objectName: "composer"
                width: parent.width
                height: implicitHeight
                service: root.service
                channelId: root.currentChannelId
                channelName: root.currentChannel
                  ? Api.channelGlyph(root.currentChannel.type) + String(root.currentChannel.name || "") : ""
                active: root.focusedZone === "composer" && root.ready
                attachmentHeightLimit: Math.min(Style.space(80), parent.height * 0.2)

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

            Components.MemberList {
              id: membersView
              objectName: "members"
              parent: root.compactMembers ? body : conversationRow
              visible: root.membersVisible
              z: root.compactMembers ? 5 : 0
              x: root.compactMembers ? body.width - width : 0
              width: root.compactMembers ? Math.min(Style.space(320), body.width * 0.65)
                : Math.min(Style.space(220), body.width * 0.22)
              height: parent.height
              dismissible: root.compactMembers
              onCloseRequested: { root.toggleMembers(); root.focusZone() }
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
              onCopyRequested: function(text) { root.copyRequested(text) }
              onActiveFocusChanged: if (activeFocus && root.zone !== "members") root.zone = "members"
            }
          }
        }

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
          Text {
            width: parent.width
            objectName: "shortcut-hint"
            elide: Text.ElideRight
            text: {
              if (!root.ready) {
                if (root.qrView) {
                  if (root.qrMissing) return Keymap.footer("qrMissing")
                  return Keymap.footer(root.qrCancelable ? "qrRunning" : "qrDone")
                }
                return Keymap.footer(root.showLogin ? "login" : "down")
              }
              if (root.width < Style.space(900)) {
                if (root.callBarFocused) return "Mute · Deafen · Leave · Help for keys"
                if (root.buttonFocused) return "Enter activates · Esc returns · Help for keys"
                if (root.zone === "composer" && root.composer.chipFocused) return "←/→ move · Delete removes · Esc returns"
                if (root.zone === "composer") return root.composer.editing
                  ? "Enter saves · Esc cancels · Help for keys"
                  : "Enter sends · Shift+Enter newline · Help for keys"
                if (root.zone === "members") return "↑/↓ move · Y copies name · Esc closes"
                if (root.zone === "timeline") return "↑/↓ move · R reply · E react · Help for keys"
                return "↑/↓ move · Enter opens · Help for keys"
              }
              if (root.callBarFocused) return Keymap.footer("voice")
              if (root.buttonFocused) {
                var back = (root.zone === "timeline" || root.zone === "composer" || root.zone === "members") && !root.currentChannelId ? "sidebar"
                  : (root.zone === "composer" ? "the composer" : (root.zone === "members" ? "the member list" : root.zone))
                return Keymap.footer("global.activate", "global.tabCycle", { id: "global.escBack", hint: "back to " + back })
              }
              if (root.zone === "composer") {
                if (root.composer.chipFocused) return Keymap.footer("chips")
                if (root.composer.editing) return Keymap.footer("composerEdit")
                return "Enter sends · Shift+Enter newline · Ctrl+V pastes · Help for all keys"
              }
              if (root.zone === "members") return Keymap.footer("members")
              if (root.zone === "timeline")
                return "↑/↓ move · R reply · E react · Y copy · Help for all keys"
              if (root.column === "rail") return "↑/↓ move · Enter opens channels · Ctrl+K search · Help for all keys"
              return "↑/↓ move · Enter opens · Ctrl+K search · Help for all keys"
            }
            color: root.muted
            font.family: root.fontFamily
            font.pixelSize: Style.font.caption
          }
        }
      }
    }
}
