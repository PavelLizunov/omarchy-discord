pragma ComponentBehavior: Bound
import QtQuick
import QtQuick.Controls
import QtQuick.Window
import "ui"

import "Api.js" as Api
import "Keymap.js" as Keymap
import "ServerList.js" as ServerList
import "components" as Components
import "ui/Icons.js" as Icons

Item {
  id: root

  property var shell: null
  property var manifest: null
  property var service: null
  property bool compactMode: false
  readonly property bool compactView: compactMode
  property bool compactChatOpen: false
  signal layoutModeChanged(bool compact)
  function setCompactMode(value) {
    var chatWasVisible = !!currentChannelId && (!narrowLayout || compactChatOpen)
    compactMode = value
    compactChatOpen = chatWasVisible
    navigationShown = false
    layoutModeChanged(value)
    Qt.callLater(focusZone)
  }
  property bool hostOpened: false
  readonly property bool opened: hostOpened
  property bool windowActive: false
  property bool mapped: false
  readonly property bool reading: opened && mapped && windowActive && !cameraView.shown
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
  property bool cameraPreviousChat: false
  function openCamera() {cameraPreviousChat=compactChatOpen;cameraView.open();if(narrowLayout)compactChatOpen=true}
  readonly property alias camera: cameraView
  readonly property bool qrImageReady: qrImage.status === Image.Ready
  readonly property alias logoutConfirmation: logoutConfirm
  readonly property alias serverMenu: serverMenu
  readonly property alias optionsMenu: optionsMenu
  readonly property alias helpButton: optionsHelp
  readonly property alias logoutButton: optionsLogout
  readonly property alias optionsButton: optionsButton
  readonly property bool overlayShown: cameraView.shown || cheatsheetView.shown || pickerView.shown || logoutConfirm.shown || serverMenu.shown || optionsMenu.shown
  readonly property bool compactMembers: false
  readonly property bool narrowLayout: compactView || width < Style.space(900)
  property bool navigationShown: false

  readonly property string pluginId: manifest && manifest.id
    ? String(manifest.id) : "quickshell.discord"
  readonly property color foreground: Color.foreground
  readonly property color muted: Api.secondaryColor(Color.muted, Color.foreground,
    Api.blend(Style.selectedFillFor(Color.foreground, Color.accent), Color.popups.background, Style.selectedFillAlpha))
  readonly property color background: Color.background
  readonly property color accent: Color.accent
  readonly property string fontFamily: Style.font.family
  readonly property int voiceAvatarSize: Style.space(16)
  readonly property int voiceOccupantHeight: Style.space(22)
  readonly property var panelBorderSpec: Border.flat(Color.popups.border,
    Math.max(1, Style.normalBorderWidth))
  readonly property real controlsRowHeight: Math.max(Style.spacing.controlHeight, headerControls.height)
  readonly property real controlButtonsWidth:
    (ready && currentChannelId !== "" ? membersButton.width + Style.spacing.controlGap : 0)
    + (cameraButton.visible ? cameraButton.width + Style.spacing.controlGap : 0)
    + (ready ? searchButton.width + Style.spacing.controlGap : 0)
    + (ready ? optionsButton.width + Style.spacing.controlGap : 0)
    + navigationButton.width + Style.spacing.controlGap + modeButton.width + Style.spacing.controlGap + closeButton.width
  readonly property real channelHeaderRoom: channelHeader.width - Style.spacing.sm * 2
    - Style.spacing.controlGap
  readonly property real channelTitleFloor: Style.space(120)
  readonly property bool controlsInHeader: false
  readonly property real statusWidthBudget: {
    if (!controlsInHeader) return Math.max(0, Math.min(Style.space(200),
      root.width - Style.spacing.panelPadding * 2 - controlButtonsWidth - Style.spacing.controlGap))
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
  function dismissError() {
    if (service) service.lastError = ""
    Qt.callLater(focusZone)
  }
  readonly property var qr: service ? service.qr : null
  readonly property string qrStage: qr ? String(qr.stage || "") : ""
  readonly property bool qrView: showLogin && (lifecycle === "qr_pending"
    || !!(service && service.qrBusy) || qr !== null)
  readonly property bool qrCancelable: lifecycle === "qr_pending" && qrStage !== "approved"
  readonly property bool qrMissing: !!(service && service.qrMissing)
  property int qrSecondsLeft: 0

  property string zone: "sidebar"
  property string column: "rail"
  readonly property bool buttonFocused: navigationButton.activeFocus || optionsButton.activeFocus || closeButton.activeFocus || modeButton.activeFocus
    || cameraButton.activeFocus || membersButton.activeFocus || searchButton.activeFocus || optionsHelp.activeFocus || optionsLogout.activeFocus
    || startBackendButton.activeFocus || dismissErrorButton.activeFocus || joinVoiceButton.activeFocus || callBarFocused
    || filterButtons(serverFilterChips).some(function(button) { return button.activeFocus })
    || filterButtons(channelFilterChips).some(function(button) { return button.activeFocus })
  readonly property bool callBarFocused: callBar.visible && callBar.activeFocus
  readonly property string activeVoiceChannelId: {
    if (!service || !service.voice) return ""
    var status = String(service.voice.status || "")
    if (status !== "connected" && status !== "connecting") return ""
    return String(service.voice.channelId || "")
  }
  readonly property bool voicePeopleMode: false
  readonly property bool voicePanelPinned: narrowLayout && !!(service && service.voice && String(service.voice.status || "idle") !== "idle")
  readonly property bool voiceRoomVisible: !!(service && service.voice && String(service.voice.status || "idle") !== "idle")
  readonly property bool voiceNavigation: narrowLayout && !compactChatOpen && voiceRoomVisible
  readonly property real voicePanelWidth: voiceRoomVisible ? Style.space(narrowLayout ? 155 : 180) : 0
  readonly property var activeVoicePeople: service && activeVoiceChannelId
    ? service.voiceUsers(String(service.voice.guildId || ""), activeVoiceChannelId) : []
  readonly property bool membersVisible: ready && !voiceRoomVisible
    && (!narrowLayout || compactChatOpen)
    && !!(service && service.membersWanted) && currentChannelId !== ""
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

  property bool serverToolsShown: false
  property string serverQuery: ""
  property string serverFilter: "all"
  property string serverSort: "position"
  property string channelFilter: "all"
  property var serverActionsOverride: null
  readonly property var serverActions: serverActionsOverride || defaultServerActions
  Components.ServerActions { id: defaultServerActions; service: root.service }
  readonly property int unreadServersCount: ServerList.unreadCount(service ? service.guilds : [], serverActions.archivedGuilds)
  readonly property int mentionedServersCount: ServerList.mentionsCount(service ? service.guilds : [], serverActions.archivedGuilds)
  readonly property int voiceServersCount: ServerList.voiceGuildsCount(service ? service.guilds : [], serverActions.archivedGuilds, service ? service.voiceMembers : null)
  readonly property var filteredGuilds: ServerList.rows(service && service.guilds ? service.guilds : [],
    serverQuery, serverFilter, serverSort, serverActions.guildStats, serverActions.archivedGuilds, service ? service.voiceMembers : null)
  function showServerMenu(row, x, y) {
    if (!row || String(row.id) === "dms" || row.kind === "archive_entry" || row.kind === "archive_back") return
    serverMenu.show(row, x, y)
  }
  readonly property var guildRows: {
    var rows = []
    if (serverFilter === "all") {
      rows.push({ kind: "dms", id: "dms", name: "Direct Messages",
        mention_count: dmMentionCount(), unread: dmUnread() })
      if (serverActions.archivedCount > 0) {
        rows.push({ kind: "archive_entry", id: "archive", name: "Archived servers (" + serverActions.archivedCount + ")",
          mention_count: 0, unread: false })
      }
    } else if (serverFilter === "unread") {
      if (dmUnread()) {
        rows.push({ kind: "dms", id: "dms", name: "Direct Messages",
          mention_count: dmMentionCount(), unread: dmUnread() })
      }
    } else if (serverFilter === "mentions") {
      if (dmMentionCount() > 0) {
        rows.push({ kind: "dms", id: "dms", name: "Direct Messages",
          mention_count: dmMentionCount(), unread: dmUnread() })
      }
    } else if (serverFilter === "archive") {
      rows.push({ kind: "archive_back", id: "archive_back", name: "All servers",
        mention_count: 0, unread: false })
    }
    for (var i = 0; i < filteredGuilds.length; i++) rows.push(filteredGuilds[i])
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
  readonly property var filteredChannelRows: dmsSelected ? channelRows : Api.filterChannels(channelRows, channelFilter)
  readonly property int guildCursor: indexOfId(guildRows, guildCursorId)
  readonly property int channelCursor: indexOfId(filteredChannelRows, channelCursorId)
  readonly property bool channelsLoading: !!(service && selectedGuildId
    && !dmsSelected && service.isLoadingChannels(selectedGuildId))
  readonly property string selectedGuildName: {
    if (selectedGuildId === "dms") return "Direct Messages"
    var list = service && service.guilds ? service.guilds : []
    for (var i = 0; i < list.length; i++)
      if (String(list[i].id) === selectedGuildId) return String(list[i].name || "")
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
    return tokenField.activeFocus || composerView.inputFocused || serverSearch.activeFocus
      || serverFilterControl.activeFocus || serverSortControl.activeFocus
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
    var stops = qrView ? [qrActionButton, closeButton] : [scanQrButton, tokenField, loginButton, closeButton]
    if (errorText) stops.push(dismissErrorButton)
    return stops
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
    if (service) { service.refresh(); service.setMembersWanted(true) }
    restoreView()
    if (requested && service) {
      service.showChannel(requested, guildIdForChannel(requested))
      enterComposer()
    }
  }

  function close() {
    optionsMenu.shown = false
    logoutConfirm.shown = false
    serverMenu.shown = false
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
    compactChatOpen = false
    if (currentChannelId && !narrowLayout) {
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
    if (index < 0 || index >= filteredChannelRows.length) return
    channelCursorId = String(filteredChannelRows[index].id || "")
    channelList.positionViewAtIndex(index, ListView.Contain)
  }

  function findChannel(from, delta, accept) {
    var count = filteredChannelRows.length
    if (!count) return -1
    var index = from
    for (var step = 0; step < count; step++) {
      index = clampCursor(index + delta, count)
      if (accept(filteredChannelRows[index])) return index
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
    if (channelCursor >= 0 && Api.isSelectableChannel(filteredChannelRows[channelCursor])) return
    var at = indexOfId(filteredChannelRows, currentChannelId)
    if (at < 0) at = findChannel(-1, 1, Api.isSelectableChannel)
    if (at >= 0) setChannelCursor(at)
    else channelCursorId = ""
  }

  function selectGuild(index) {
    if (index < 0 || index >= guildRows.length || !service) return
    setGuildCursor(index)
    var row = guildRows[index]
    var id = String(row.id || "")
    if (row.kind === "archive_entry" || id === "archive") {
      serverFilter = "archive"
      setGuildCursor(0)
      return
    }
    if (row.kind === "archive_back" || id === "archive_back") {
      serverFilter = "all"
      setGuildCursor(0)
      return
    }
    compactChatOpen = false
    channelCursorId = ""
    channelFilter = "all"
    Api.browseGuild(service, id)
  }

  function enterChannels() {
    var index = guildCursor < 0 ? 0 : guildCursor
    var row = index >= 0 && index < guildRows.length ? guildRows[index] : null
    if (row && (row.kind === "archive_entry" || row.kind === "archive_back")) {
      selectGuild(index)
      return
    }
    selectGuild(index)
    zone = "sidebar"
    column = "channels"
    hint = ""
    if (currentChannelId && indexOfId(filteredChannelRows, currentChannelId) >= 0)
      channelCursorId = currentChannelId
    ensureCursors()
    focusZone()
  }

  function leaveChannels() {
    column = "rail"
    hint = ""
  }

  function activateChannel(index, origin) {
    var row = filteredChannelRows[index]
    if (!row || !service) return
    if (String(row.type || "") === "forum") { toggleThreads(index); return }
    setChannelCursor(indexOfId(filteredChannelRows, row.id))
    if (String(row.type || "") !== "voice" && !Api.isOpenableChannel(row)) return
    hint = ""
    service.showChannel(String(row.id || ""), selectedGuildId)
    if (narrowLayout) compactChatOpen = true
    navigationShown = false
    timelineView.focusNewest()
    if (origin === "timeline") enterTimeline()
    else if (origin === "members" && membersVisible) enterMembers()
    else enterComposer()
  }

  function joinVoice(index) {
    var row = filteredChannelRows[index]
    if (!row) return
    setChannelCursor(index)
    joinVoiceChannel(row)
  }

  function joinVoiceChannel(row) {
    if (!row || String(row.type || "") !== "voice" || !service) return
    hint = ""
    var id = String(row.id || "")
    if (!id) return
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
    var row = filteredChannelRows[index]
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
    if (voicePeopleMode) { enterMembers(); return }
    if (!service) return
    if (!currentChannelId) { hint = "Open a channel first"; return }
    if (narrowLayout) compactChatOpen = true
    service.setMembersWanted(!service.membersWanted)
    hint = ""
    if (service.membersWanted && narrowLayout) enterMembers()
    if (!service.membersWanted && zone === "members") { zone = "composer"; focusZone() }
  }

  function enterMembers() {
    if (narrowLayout) compactChatOpen = true
    if (!membersVisible) return
    zone = "members"
    hint = ""
    focusZone()
  }

  function leaveMembers() {
    if (narrowLayout && !voicePeopleMode && service) service.setMembersWanted(false)
    zone = "composer"
    focusZone()
  }

  function enterTimeline() {
    if (!currentChannelId) return
    if (narrowLayout) compactChatOpen = true
    zone = "timeline"
    timelineView.focusNewest()
    focusZone()
  }

  function leaveTimeline(markRead) {
    if (narrowLayout) compactChatOpen = false
    if (markRead && service && currentChannelId) service.markChannelRead(currentChannelId)
    zone = "sidebar"
    column = "channels"
    if (currentChannelId && indexOfId(filteredChannelRows, currentChannelId) >= 0)
      channelCursorId = currentChannelId
    focusZone()
  }

  function enterComposer() {
    if (!currentChannelId) return
    if (narrowLayout) compactChatOpen = true
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
    var from = indexOfId(filteredChannelRows, currentChannelId)
    if (from < 0) from = channelCursor >= 0 ? channelCursor - delta : (delta > 0 ? -1 : 0)
    var accept = unreadOnly
      ? function(row) { return Api.isOpenableChannel(row) && Api.isUnread(row) }
      : Api.isOpenableChannel
    var next = findChannel(from, delta, accept)
    if (next < 0 || next === from) return
    activateChannel(next, zone)
  }

  function focusZone() {
    if (narrowLayout && !compactChatOpen && (zone === "timeline" || zone === "composer")) {
      zone = "sidebar"
      column = "channels"
    }
    if (zone === "members" && !membersVisible) zone = "composer"
    if ((zone === "timeline" || zone === "composer") && !currentChannelId) {
      zone = "sidebar"
      column = selectedGuildId ? "channels" : "rail"
    }
    if (zone === "members") membersView.forceActiveFocus()
    else if (zone === "composer") composerView.focusInput()
    else if (zone === "timeline") {
      var actions = timelineView.messageActions.children
      for (var i = 0; i < actions.length; i++) actions[i].focus = false
      timelineView.forceActiveFocus()
    }
    else sidebarFocus.forceActiveFocus()
  }

  function filterButtons(group) {
    return group.children.filter(function(item) { return item.visible && typeof item.focusable === "boolean" })
  }

  function cycleFocus(delta) {
    var groups = [serverFilterChips, channelFilterChips]
    var groupNames = ["serverChips", "channelChips"]
    var focusedGroup = ""
    for (var g = 0; g < groups.length; g++) {
      var buttons = filterButtons(groups[g])
      for (var b = 0; b < buttons.length; b++) if (buttons[b].activeFocus) {
        if (b + delta >= 0 && b + delta < buttons.length) { buttons[b + delta].forceActiveFocus(); return }
        focusedGroup = groupNames[g]
      }
    }
    var stops = ["rail", "serverChips", "serverTools", "serverSearch", "serverFilter", "serverSort", "serverRefresh", "channels", "channelChips", "timeline", "composer", "joinVoice", "callbar", "members", "startBackend",
      "navigation", "search", "camera", "membersButton", "options", "mode", "close", "dismissError"]
    var current = focusedGroup || (joinVoiceButton.activeFocus ? "joinVoice" : dismissErrorButton.activeFocus ? "dismissError" : navigationButton.activeFocus ? "navigation" : serverToolsButton.activeFocus ? "serverTools" : serverSearch.activeFocus ? "serverSearch" : serverFilterControl.activeFocus ? "serverFilter"
      : serverSortControl.activeFocus ? "serverSort" : serverRefresh.activeFocus ? "serverRefresh" : callBarFocused ? "callbar"
      : (buttonFocused
        ? (cameraButton.activeFocus ? "camera" : modeButton.activeFocus ? "mode" : closeButton.activeFocus ? "close" : searchButton.activeFocus ? "search" : optionsButton.activeFocus ? "options"
          : (membersButton.activeFocus ? "membersButton"
            : (startBackendButton.activeFocus ? "startBackend" : "options")))
        : (zone === "sidebar" ? column : zone)))
    var index = stops.indexOf(current)
    for (var step = 0; step < stops.length; step++) {
      index = clampCursor(index + delta, stops.length)
      var stop = stops[index]
      if (stop === "joinVoice" && (!joinVoiceButton.visible || !chatColumn.visible)) continue
      if (stop === "serverChips" && (!railPane.visible || !filterButtons(serverFilterChips).length)) continue
      if (stop === "channelChips" && (!channelPane.visible || !channelFilterChips.visible || !filterButtons(channelFilterChips).length)) continue
      if ((stop === "rail" || stop === "channels" || stop === "serverSearch" || stop === "serverFilter" || stop === "serverSort") && !ready) continue
      if ((stop === "timeline" || stop === "composer") && !currentChannelId) continue
      if (narrowLayout && !compactChatOpen && (stop === "timeline" || stop === "composer" || stop === "members")) continue
      if (narrowLayout && compactChatOpen && (stop === "rail" || stop === "channels" || stop === "serverTools" || stop === "serverSearch" || stop === "serverFilter" || stop === "serverSort" || stop === "serverRefresh")) continue
      if (["serverSearch", "serverFilter", "serverSort"].indexOf(stop) >= 0 && !serverToolsShown) continue
      if (stop === "serverRefresh" && !serverRefresh.visible) continue
      if (stop === "callbar" && !(ready && callBar.visible)) continue
      if (stop === "members" && !membersVisible) continue
      if (stop === "startBackend" && !startBackendButton.visible) continue
      if (stop === "camera" && !cameraButton.visible) continue
      if (stop === "membersButton" && !membersButton.visible) continue
      if (stop === "navigation" && !navigationButton.visible) continue
      if (stop === "search" && !searchButton.visible) continue
      if (stop === "options" && !optionsButton.visible) continue
      if (stop === "mode" && !modeButton.visible) continue
      if (stop === "close" && !closeButton.visible) continue
      if (stop === "dismissError") {
        if (!dismissErrorButton.visible) continue
        dismissErrorButton.forceActiveFocus()
      } else focusStop(stop, delta)
      return
    }
  }

  function focusStop(stop, delta) {
    hint = ""
    if (stop === "serverChips" || stop === "channelChips") {
      var buttons = filterButtons(stop === "serverChips" ? serverFilterChips : channelFilterChips)
      if (buttons.length) buttons[delta < 0 ? buttons.length - 1 : 0].forceActiveFocus()
      return
    }
    if (stop === "joinVoice") { joinVoiceButton.forceActiveFocus(); return }
    if (stop === "navigation") { navigationButton.forceActiveFocus(); return }
    if (stop === "serverTools") { serverToolsButton.forceActiveFocus(); return }
    if (stop === "serverSearch") { serverSearch.forceActiveFocus(); return }
    if (stop === "serverFilter") { serverFilterControl.forceActiveFocus(); return }
    if (stop === "serverSort") { serverSortControl.forceActiveFocus(); return }
    if (stop === "serverRefresh") { serverRefresh.forceActiveFocus(); return }
    if (stop === "mode") { modeButton.forceActiveFocus(); return }
    if (stop === "search") { searchButton.forceActiveFocus(); return }
    if (stop === "options") { optionsButton.forceActiveFocus(); return }
    if (stop === "close") { closeButton.forceActiveFocus(); return }
    if (stop === "camera") { cameraButton.forceActiveFocus(); return }
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
    if (narrowLayout && compactChatOpen && key === Qt.Key_Escape) {
      compactChatOpen = false
      zone = "sidebar"
      column = "channels"
      focusZone()
      event.accepted = true
      return
    }
    if (navigationShown && key === Qt.Key_Escape) { navigationShown = false; event.accepted = true; return }
    if (handleGlobalKey(event)) return
    if (showLogin) {
      if (key === Qt.Key_Tab || key === Qt.Key_Backtab) cycleLoginFocus(key === Qt.Key_Backtab || shift ? -1 : 1)
      else if (key === Qt.Key_Escape) { if (qrView) leaveQr(); else root.requestClose() }
      else return
      event.accepted = true
      return
    }
    if (textInputFocused() && !composerFocused) return
    if (key === Qt.Key_Menu || (shift && key === Qt.Key_F10)) {
      var row = guildRows[guildCursor]
      if (row && String(row.id) !== "dms") showServerMenu(row, Style.space(160), Style.space(90))
      event.accepted = true
      return
    }
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
    if (optionsMenu.shown) {
      if (event.key === Qt.Key_Escape) optionsMenu.hide()
      event.accepted = true
      return true
    }
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
  onFilteredChannelRowsChanged: ensureCursors()
  onShowLoginChanged: if (showLogin && opened) Qt.callLater(focusLogin)
  onQrViewChanged: if (showLogin && opened) Qt.callLater(focusLogin)
  onWindowActiveChanged: publishActive()
  onReadingChanged: if (reading && ready && timelineView.pinned
    && (focusedZone === "timeline" || focusedZone === "composer")) ackTimer.restart()
  onOpenedChanged: {if (!opened && cameraView.shown) cameraView.hide(); opened ? enter() : leave()}

  onMappedChanged: publishMapped()
  onScreenNameChanged: publishScreen()
  Component.onCompleted: publishMapped()
  onQrChanged: updateQrCountdown()
  onReadyChanged: {
    if (!ready) { zone = "sidebar"; column = "rail" }
    if (opened) focusZone()
  }
  onMembersVisibleChanged: {
    if (!membersVisible && zone === "members" && opened) { zone = "composer"; focusZone() }
  }
  onSelectedGuildIdChanged: expandedThreads = ({})
  onCurrentChannelIdChanged: {
    navigationShown = false
    if (currentChannelId) compactChatOpen = true
  }

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
        objectName: "header-controls"
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
          id: navigationButton
          objectName: "navigation-button"
          iconOnly: true
          iconName: root.compactChatOpen ? "back" : "navigation"
          text: root.compactChatOpen ? "Channels" : "Servers and channels"
          tooltipText: root.compactChatOpen ? "Back to servers & channels (Esc)" : "Servers and channels"
          visible: root.ready && root.narrowLayout && (root.compactChatOpen || root.currentChannelId !== "")
          active: !root.compactChatOpen
          focusable: true
          onClicked: {
            if (root.compactChatOpen) {
              root.compactChatOpen = false
              root.zone = "sidebar"
              root.column = "channels"
              root.focusZone()
            } else if (root.currentChannelId !== "") {
              root.compactChatOpen = true
              root.zone = "composer"
              root.focusZone()
            }
          }
        }
        Button {
          id: searchButton
          objectName: "search-button"
          visible: root.ready
          iconOnly: true
          text: "Search"
          iconName: "search"
          focusable: true
          activeFocusOnTab: false
          tooltipText: "Find a channel or direct message (Ctrl+K)"
          onClicked: root.openSwitcher()
        }
        Button {
          id: cameraButton
          objectName: "camera-button"
          visible: root.ready && root.service && root.service.voice && root.service.voice.status === "connected"
          text: "Cameras"
          tooltipText: "Choose a participant's webcam to watch (H.264 MVP)"
          focusable: true
          onClicked: root.openCamera()
        }
        Button {
          id: membersButton
          objectName: "members-button"
          visible: root.ready && (root.currentChannelId !== "" || root.voicePeopleMode)
          iconOnly: true
          text: root.voicePeopleMode ? "Voice participants" : "Members"
          iconName: "members"
          active: root.membersVisible
          focusable: true
          activeFocusOnTab: false
          foreground: root.foreground
          tooltipText: root.voicePeopleMode ? "People in the current voice channel (m)" : "Show / hide the member list (m)"
          onClicked: root.toggleMembers()
        }
        Button {
          id: optionsButton
          objectName: "options-button"
          visible: root.ready
          iconOnly: true
          text: "Settings"
          iconName: "settings"
          active: optionsMenu.shown
          focusable: true
          activeFocusOnTab: false
          foreground: root.foreground
          onClicked: optionsMenu.toggle("top-right")
        }
        Button {
          id: modeButton
          objectName: "layout-mode-button"
          iconOnly: true
          iconName: root.compactView ? "expand" : "compact"
          tooltipText: root.compactView ? "Full layout" : "Compact layout"
          text: root.compactView ? "Full" : "Compact"
          focusable: true
          activeFocusOnTab: false
          onClicked: root.setCompactMode(!root.compactView)
        }
        Button {
          id: closeButton
          iconOnly: true
          tooltipText: "Close window"
          text: "Close"
          iconName: "close"
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
          var at = root.indexOfId(root.filteredChannelRows, channelId)
          if (at >= 0) root.setChannelCursor(at)
        }
      }

      Components.ServerMenu {
        id: serverMenu
        anchors.fill: parent
        z: 25
        service: root.serverActions
        onDismissed: Qt.callLater(root.focusZone)
      }

      Rectangle {
        id: optionsBackdrop
        anchors.fill: parent
        z: 21
        color: Color.menu.scrim
        opacity: optionsMenu.shown ? 1 : 0
        visible: optionsMenu.shown
        Behavior on opacity { NumberAnimation { duration: 140 } }
        MouseArea {
          anchors.fill: parent
          onClicked: optionsMenu.hide()
        }
      }

      BorderSurface {
        id: optionsMenu
        objectName: "options-menu"
        property bool shown: false
        property string anchorCorner: "top-right"
        visible: shown
        z: 22
        Keys.onEscapePressed: hide()
        x: anchorCorner === "bottom-left" ? Style.spacing.panelPadding : (parent.width - width - Style.spacing.panelPadding)
        y: anchorCorner === "bottom-left"
          ? Math.max(Style.spacing.panelPadding, parent.height - height - Style.space(50))
          : (root.controlsRowHeight + Style.spacing.panelPadding + Style.spacing.xs)
        width: Math.min(Style.space(260), parent.width - Style.spacing.panelPadding * 2)
        height: optionsBody.implicitHeight + Style.spacing.md * 2
        radius: Style.cornerRadius
        color: Color.popups.background
        borderSpec: Border.flat(Color.popups.border, Math.max(1, Style.normalBorderWidth))
        function toggle(corner) { if (shown) hide(); else showAt(corner || "top-right") }
        function show() { showAt("top-right") }
        function showAt(corner) {
          anchorCorner = corner || "top-right"
          shown = true
          Qt.callLater(function() { optionsHelp.forceActiveFocus() })
        }
        function hide() {
          shown = false
          Qt.callLater(function() {
            if (anchorCorner === "bottom-left") userBarSettingsBtn.forceActiveFocus()
            else optionsButton.forceActiveFocus()
          })
        }
        MouseArea { anchors.fill: parent }

        Column {
          id: optionsBody
          anchors.left: parent.left
          anchors.right: parent.right
          anchors.top: parent.top
          anchors.margins: Style.spacing.md
          spacing: Style.spacing.xs

          Row {
            width: parent.width
            spacing: Style.spacing.sm
            Components.Avatar {
              anchors.verticalCenter: parent.verticalCenter
              width: Style.space(28); height: Style.space(28)
              name: root.service && root.service.user ? (root.service.user.display_name || root.service.user.username || "") : ""
              service: root.service
            }
            Column {
              anchors.verticalCenter: parent.verticalCenter
              width: parent.width - Style.space(36)
              Text {
                width: parent.width; elide: Text.ElideRight
                text: root.service && root.service.user ? String(root.service.user.display_name || root.service.user.username || "Discord user") : "Discord user"
                color: Color.popups.text
                font.family: root.fontFamily; font.pixelSize: Style.font.body; font.bold: true
              }
              Text {
                width: parent.width; elide: Text.ElideRight
                text: root.service && root.service.user && root.service.user.username ? "@" + String(root.service.user.username) : (root.service ? root.service.statusText : "")
                color: root.muted
                font.family: root.fontFamily; font.pixelSize: Style.font.caption
              }
            }
          }

          Rectangle { width: parent.width; height: 1; color: Color.popups.border; opacity: 0.4 }

          Button {
            id: optionsNotifs
            objectName: "menu-notifications"
            width: parent.width
            leftAlign: true
            iconName: "settings"
            readonly property string currentVal: root.service && root.service.settings ? String(root.service.settings.notifications || "Mentions and DMs") : "Mentions and DMs"
            text: "Notifications: " + (currentVal === "All" ? "All" : (currentVal === "Off" ? "Off" : "Mentions"))
            focusable: true
            onClicked: {
              if (!root.service || typeof root.service.persistSettings !== "function") return
              var cycle = ["All", "Mentions and DMs", "Off"]
              var idx = cycle.indexOf(currentVal)
              var next = cycle[(idx + 1) % cycle.length]
              root.service.persistSettings({ notifications: next })
            }
          }

          Button {
            id: optionsStayConnected
            objectName: "menu-stay-connected"
            width: parent.width
            leftAlign: true
            iconName: "reconnect"
            readonly property bool isStay: root.service && root.service.settings ? root.service.settings.stayConnected !== "Off" : true
            text: "Stay connected: " + (isStay ? "On" : "Off")
            focusable: true
            onClicked: {
              if (!root.service || typeof root.service.persistSettings !== "function") return
              root.service.persistSettings({ stayConnected: isStay ? "Off" : "On" })
            }
          }

          Button {
            id: optionsArchive
            objectName: "menu-archive"
            width: parent.width
            leftAlign: true
            iconName: "archive"
            visible: root.serverActions.archivedCount > 0
            text: "Archived servers (" + root.serverActions.archivedCount + ")"
            focusable: true
            onClicked: {
              optionsMenu.hide()
              root.serverFilter = "archive"
              root.setGuildCursor(0)
            }
          }

          Rectangle { width: parent.width; height: 1; color: Color.popups.border; opacity: 0.4 }

          Button {
            id: optionsHelp
            objectName: "menu-help"
            width: parent.width
            leftAlign: true
            iconName: "help"
            text: "Keyboard shortcuts"
            focusable: true
            onClicked: { optionsMenu.hide(); root.toggleCheatsheet() }
          }

          Button {
            id: optionsLogout
            objectName: "menu-logout"
            width: parent.width
            leftAlign: true
            iconName: "logout"
            text: "Log out…"
            foreground: Color.urgent
            focusable: true
            onClicked: { optionsMenu.hide(); logoutConfirm.show() }
          }
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
                text: "Or paste a user token and press Enter. The backend stores it in your system keyring. Omacord does not keep a plaintext token file."
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

          Item {
            id: navigationDrawer
            objectName: "navigation-drawer"
            visible: false
            width: 0; height: 0
            x: 0
          }
          Item {
            id: conversationRow
            anchors.fill: parent
            anchors.leftMargin: 0
            visible: root.ready
            readonly property real spacing: Style.spacing.panelGap

            BorderSurface {
              id: railPane
              parent: conversationRow
              objectName: "server-rail"
              x: 0
              visible: !root.narrowLayout || !root.compactChatOpen
              width: root.narrowLayout ? Math.min(Style.space(180), Math.max(Style.space(160), conversationRow.width * 0.38))
                : Math.min(Style.space(180), Math.max(Style.space(140), body.width * 0.16))
              height: parent.height
              radius: Style.cornerRadius
              color: Color.popups.background
              borderSpec: root.focusedZone === "sidebar" && root.column === "rail"
                ? Border.controlSpec("focus", root.foreground, root.accent)
                : root.panelBorderSpec
              padding: Style.spacing.sm

              Flickable {
                id: serverControlsViewport
                objectName: "server-controls-viewport"
                anchors.left: parent.left; anchors.right: parent.right; anchors.top: parent.top
                anchors.margins: Style.spacing.sm
                height: Math.min(serverControls.implicitHeight, Math.max(0, railPane.height - userBar.height - Style.space(170)))
                contentWidth: width
                contentHeight: serverControls.implicitHeight
                clip: true
                boundsBehavior: Flickable.StopAtBounds
                ScrollBar.vertical: ScrollBar { policy: ScrollBar.AsNeeded }
                readonly property var focusedItem: Window.window ? Window.window.activeFocusItem : null
                onFocusedItemChanged: Qt.callLater(revealFocusedControl)
                function revealFocusedControl() {
                  var item = focusedItem
                  if (!item) return
                  var node = item
                  while (node && node !== serverControls) node = node.parent
                  if (!node) return
                  var y = item.mapToItem(serverControls, 0, 0).y
                  if (y < contentY) contentY = y
                  else if (y + item.height > contentY + height) contentY = y + item.height - height
                }
                Column {
                id: serverControls
                objectName: "server-list-controls"
                width: serverControlsViewport.width
                spacing: Style.spacing.xs
                Row {
                  width: parent.width
                  Text {
                    width: parent.width - serverToolsButton.width
                    anchors.verticalCenter: parent.verticalCenter
                    text: "Browse"; color: root.foreground
                    font.family: root.fontFamily; font.pixelSize: Style.font.body; font.bold: true
                  }
                  Button {
                    id: serverToolsButton; objectName: "server-tools-button"
                    Keys.onTabPressed: root.cycleFocus(1)
                    Keys.onBacktabPressed: root.cycleFocus(-1)
                    iconOnly: true; iconName: "sliders"; text: "Filter and sort servers"
                    tooltipText: "Filter and sort servers"; active: root.serverToolsShown
                    focusable: true
                    onClicked: root.serverToolsShown = !root.serverToolsShown
                  }
                }
                Flow {
                  id: serverFilterChips
                  objectName: "server-filter-chips"
                  width: parent.width
                  spacing: Style.spacing.xxs

                  Button {
                    iconName: "all"
                    text: "All"
                    tooltipText: "All active servers"
                    focusable: true
                    Keys.onTabPressed: root.cycleFocus(1)
                    Keys.onBacktabPressed: root.cycleFocus(-1)
                    active: root.serverFilter === "all"
                    fontSize: Style.font.caption
                    horizontalPadding: Style.spacing.xs
                    onClicked: root.serverFilter = "all"
                  }
                  Button {
                    iconName: "unread"
                    text: root.unreadServersCount > 0 ? String(root.unreadServersCount) : "Unread"
                    tooltipText: "Servers with unread messages"
                    focusable: true
                    Keys.onTabPressed: root.cycleFocus(1)
                    Keys.onBacktabPressed: root.cycleFocus(-1)
                    active: root.serverFilter === "unread"
                    fontSize: Style.font.caption
                    horizontalPadding: Style.spacing.xs
                    foreground: root.unreadServersCount > 0 ? root.accent : root.muted
                    onClicked: root.serverFilter = "unread"
                  }
                  Button {
                    iconName: "mention"
                    text: root.mentionedServersCount > 0 ? String(root.mentionedServersCount) : "Mentions"
                    tooltipText: "Servers with unread mention counts"
                    focusable: true
                    Keys.onTabPressed: root.cycleFocus(1)
                    Keys.onBacktabPressed: root.cycleFocus(-1)
                    active: root.serverFilter === "mentions"
                    fontSize: Style.font.caption
                    horizontalPadding: Style.spacing.xs
                    foreground: root.mentionedServersCount > 0 ? Color.urgent : root.muted
                    onClicked: root.serverFilter = "mentions"
                  }
                  Button {
                    iconName: "headphones"
                    text: root.voiceServersCount > 0 ? String(root.voiceServersCount) : "Voice"
                    tooltipText: "Servers with active voice rooms"
                    focusable: true
                    Keys.onTabPressed: root.cycleFocus(1)
                    Keys.onBacktabPressed: root.cycleFocus(-1)
                    active: root.serverFilter === "voice"
                    fontSize: Style.font.caption
                    horizontalPadding: Style.spacing.xs
                    onClicked: root.serverFilter = "voice"
                  }
                  Button {
                    iconName: "archive"
                    text: root.serverActions.archivedCount > 0 ? String(root.serverActions.archivedCount) : "Archive"
                    tooltipText: "Archived servers"
                    focusable: true
                    Keys.onTabPressed: root.cycleFocus(1)
                    Keys.onBacktabPressed: root.cycleFocus(-1)
                    active: root.serverFilter === "archive"
                    fontSize: Style.font.caption
                    horizontalPadding: Style.spacing.xs
                    visible: root.serverActions.archivedCount > 0 || root.serverFilter === "archive"
                    onClicked: root.serverFilter = (root.serverFilter === "archive" ? "all" : "archive")
                  }
                }
                TextField {
                  id: serverSearch; objectName: "server-search"
                  visible: root.serverToolsShown
                  width: parent.width; placeholderText: "Find server"
                  text: root.serverQuery
                  onTextEdited: root.serverQuery = text
                  Keys.onTabPressed: root.cycleFocus(1)
                  Keys.onBacktabPressed: root.cycleFocus(-1)
                  Keys.onEscapePressed: { root.serverQuery = ""; root.focusZone() }
                }
                Select {
                  id: serverFilterControl; objectName: "server-filter"
                  visible: root.serverToolsShown
                  width: parent.width
                  model: ["All servers", "Unread", "Mentions", "Archived (" + root.serverActions.archivedCount + ")"]
                  currentIndex: Math.max(0, ["all", "unread", "mentions", "archive"].indexOf(root.serverFilter))
                  font.family: root.fontFamily; font.pixelSize: Style.font.bodySmall
                  onActivated: function(index) { root.serverFilter = ["all", "unread", "mentions", "archive"][index] }
                  Keys.onTabPressed: root.cycleFocus(1)
                  Keys.onBacktabPressed: root.cycleFocus(-1)
                }
                Select {
                  id: serverSortControl; objectName: "server-sort"
                  visible: root.serverToolsShown
                  width: parent.width
                  model: ["Discord order", "Name", "Most mentions", "Most online"]
                  currentIndex: ["position", "name", "mentions", "online"].indexOf(root.serverSort)
                  Keys.onTabPressed: root.cycleFocus(1)
                  Keys.onBacktabPressed: root.cycleFocus(-1)
                  font.family: root.fontFamily; font.pixelSize: Style.font.bodySmall
                  onActivated: function(index) {
                    root.serverSort = ["position", "name", "mentions", "online"][index]
                    if (root.serverSort === "online") root.serverActions.fetchGuildStats()
                  }
                }
                Button {
                  id: serverRefresh
                  objectName: "server-count-refresh"
                  Keys.onTabPressed: root.cycleFocus(1)
                  Keys.onBacktabPressed: root.cycleFocus(-1)
                  width: parent.width; visible: root.serverToolsShown && root.serverSort === "online"; focusable: true
                  enabled: !root.serverActions.guildStatsBusy
                  text: enabled ? "Refresh counts" : "Loading counts…"
                  onClicked: root.serverActions.fetchGuildStats()
                }
                Text {
                  width: parent.width; wrapMode: Text.WordWrap
                  visible: root.serverToolsShown && root.serverSort === "online"
                  text: "Approximate online · last fetch"
                  color: root.muted; font.family: root.fontFamily; font.pixelSize: Style.font.caption
                }
              }
              }
              ListView {
                id: guildList
                objectName: "server-results"
                anchors.left: parent.left; anchors.right: parent.right
                anchors.bottom: userBar.top
                anchors.top: serverControlsViewport.bottom
                anchors.margins: Style.spacing.sm
                clip: true
                reuseItems: true
                cacheBuffer: Style.space(150)
                boundsBehavior: Flickable.StopAtBounds
                spacing: Style.spacing.sm
                model: root.guildRows.length
                footer: Text {
                  width: guildList.width; wrapMode: Text.WordWrap
                  visible: root.filteredGuilds.length === 0
                  text: root.serverFilter === "unread" ? "No unread servers."
                    : (root.serverFilter === "mentions" ? "No server mentions."
                      : (root.serverFilter === "voice" ? "No active voice rooms."
                        : (root.serverFilter === "archive" ? "Archive is empty."
                          : "No matching servers.")))
                  color: root.muted; font.family: root.fontFamily; font.pixelSize: Style.font.bodySmall
                  horizontalAlignment: Text.AlignHCenter
                  topPadding: Style.spacing.md
                }
                ScrollBar.vertical: ScrollBar { policy: ScrollBar.AsNeeded }

                delegate: Item {
                  id: guildRow
                  required property int index
                  readonly property var row: root.guildRows[index] || ({})
                  readonly property bool isDms: String(row.id) === "dms"
                  readonly property bool navigationEntry: isDms || row.kind === "archive_entry" || row.kind === "archive_back"
                  readonly property bool startsServers: !navigationEntry && (index === 0
                    || ["dms", "archive_entry", "archive_back"].indexOf(String(root.guildRows[index - 1].kind || "")) >= 0)
                  readonly property real sectionHeight: startsServers ? Style.space(28) : 0
                  objectName: (navigationEntry ? "navigation-entry-" : "guild-entry-") + String(row.id || "")
                  readonly property bool hasCursor: root.focusedZone === "sidebar" && root.column === "rail"
                    && index === root.guildCursor
                  readonly property bool selected: String(row.id) === root.selectedGuildId
                  readonly property int mentions: Number(row.mention_count) || 0
                  readonly property bool unread: Api.isUnread(row)
                  width: guildList.width
                  height: sectionHeight + Style.space(navigationEntry ? 40 : root.serverSort === "online" ? 64 : 48)

                  Item {
                    visible: guildRow.startsServers
                    width: parent.width
                    height: guildRow.sectionHeight
                    Rectangle {
                      anchors.left: parent.left; anchors.right: parent.right
                      height: 1; color: Color.popups.border
                    }
                    Text {
                      objectName: "guild-section-heading"
                      anchors.left: parent.left; anchors.leftMargin: Style.spacing.sm
                      anchors.bottom: parent.bottom; anchors.bottomMargin: Style.spacing.xs
                      text: root.serverFilter === "archive" ? "Archived servers" : "Servers"
                      color: root.muted; font.family: root.fontFamily; font.pixelSize: Style.font.caption
                    }
                  }

                  Rectangle {
                    anchors.left: parent.left
                    y: guildRow.sectionHeight + (parent.height - guildRow.sectionHeight - height) / 2
                    width: Style.spacing.xs
                    height: guildRow.selected ? Style.space(28) : (guildRow.unread ? Style.spacing.lg : 0)
                    radius: width / 2
                    color: guildRow.selected ? root.accent : root.foreground
                    visible: height > 0 && !guildRow.navigationEntry
                    Behavior on height { NumberAnimation { duration: 120 } }
                  }

                  BorderSurface {
                    id: guildTile
                    objectName: "guild-tile-" + String(guildRow.row.id || "")
                    x: Style.spacing.sm / 2
                    y: guildRow.sectionHeight
                    width: parent.width - Style.spacing.sm
                    height: parent.height - guildRow.sectionHeight
                    radius: Style.cornerRadius
                    color: guildRow.hasCursor
                      ? Style.hoverFillFor(root.foreground, root.accent)
                      : (guildRow.selected ? Style.selectedFillFor(root.foreground, root.accent)
                        : (guildMouse.containsMouse ? Style.hoverFillFor(root.foreground, root.accent)
                          : guildRow.navigationEntry ? "transparent" : Style.normalFillFor(root.foreground, root.accent)))
                    borderSpec: guildRow.hasCursor
                      ? Border.controlSpec("hover-cursor", root.foreground, root.accent)
                      : Border.none()
                    Behavior on radius { NumberAnimation { duration: 120 } }
                    Behavior on color { ColorAnimation { duration: 120 } }

                    Image {
                      id: archiveIcon
                      visible: guildRow.navigationEntry
                      anchors.left: parent.left
                      anchors.leftMargin: Style.spacing.sm
                      anchors.verticalCenter: parent.verticalCenter
                      width: Style.space(16); height: Style.space(16)
                      source: Icons.source(guildRow.isDms ? "user" : guildRow.row.kind === "archive_back" ? "back" : "archive", root.foreground)
                    }

                    Text {
                      objectName: "server-name-" + String(guildRow.row.id || "")
                      anchors.left: archiveIcon.visible ? archiveIcon.right : parent.left
                      anchors.leftMargin: archiveIcon.visible ? Style.spacing.xs : Style.spacing.sm
                      anchors.right: parent.right
                      anchors.verticalCenter: parent.verticalCenter
                      anchors.verticalCenterOffset: !guildRow.navigationEntry && root.serverSort === "online" ? -Style.space(9) : 0
                      anchors.rightMargin: guildRow.mentions > 0 ? guildMentionBadge.width + Style.spacing.sm * 2 : Style.spacing.sm
                      text: String(guildRow.row.name || "Unnamed server")
                      textFormat: Text.PlainText
                      wrapMode: Text.WordWrap
                      maximumLineCount: 2
                      elide: Text.ElideRight
                      color: guildRow.unread || guildRow.selected ? root.foreground : root.muted
                      font.family: root.fontFamily
                      font.pixelSize: Style.font.bodySmall
                      font.bold: guildRow.unread || guildRow.selected
                    }
                    Text {
                      anchors.left: parent.left; anchors.bottom: parent.bottom
                      anchors.margins: Style.spacing.sm
                      visible: !guildRow.navigationEntry && root.serverSort === "online"
                      readonly property var count: ServerList.knownCount(root.serverActions.guildStats, guildRow.row.id)
                      text: count === null ? "No data" : "≈ " + count + " online"
                      color: root.muted; font.family: root.fontFamily; font.pixelSize: Style.font.caption
                    }
                  }

                  Rectangle {
                    id: guildMentionBadge
                    visible: guildRow.mentions > 0
                    anchors.right: guildTile.right
                    anchors.verticalCenter: guildTile.verticalCenter
                    anchors.rightMargin: Style.spacing.xs
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
                    x: 0; y: guildRow.sectionHeight
                    width: parent.width; height: parent.height - y
                    hoverEnabled: true
                    acceptedButtons: Qt.LeftButton | Qt.RightButton
                    onClicked: function(mouse) {
                      if (mouse.button === Qt.RightButton) {
                        var pos = guildMouse.mapToItem(serverMenu, mouse.x, mouse.y)
                        root.showServerMenu(guildRow.row, pos.x, pos.y)
                        return
                      }
                      root.zone = "sidebar"
                      root.column = "rail"
                      root.setGuildCursor(guildRow.index)
                      root.enterChannels()
                      root.focusZone()
                    }
                  }

                }
              }

              BorderSurface {
                id: userBar
                objectName: "user-bar"
                anchors.left: parent.left
                anchors.right: parent.right
                anchors.bottom: parent.bottom
                anchors.margins: Style.spacing.xxs
                height: Style.space(42)
                radius: Style.cornerRadius
                color: userBarMouse.containsMouse || (optionsMenu.shown && optionsMenu.anchorCorner === "bottom-left")
                  ? Style.hoverFillFor(root.foreground, root.accent)
                  : "transparent"
                borderSpec: Border.none()

                Row {
                  anchors.fill: parent
                  anchors.leftMargin: Style.spacing.xs
                  anchors.rightMargin: Style.spacing.xs
                  spacing: Style.spacing.xs

                  Components.Avatar {
                    anchors.verticalCenter: parent.verticalCenter
                    width: Style.space(26); height: Style.space(26)
                    name: root.service && root.service.user ? (root.service.user.display_name || root.service.user.username || "") : ""
                    service: root.service

                    Rectangle {
                      anchors.right: parent.right
                      anchors.bottom: parent.bottom
                      width: Style.spacing.sm + 2; height: width
                      radius: width / 2
                      color: Color.popups.background
                      Rectangle {
                        anchors.centerIn: parent
                        width: Style.spacing.sm; height: width
                        radius: width / 2
                        color: root.connected ? Color.accent : Color.urgent
                      }
                    }
                  }

                  Column {
                    anchors.verticalCenter: parent.verticalCenter
                    width: Math.max(0, parent.width - Style.space(26) - userBarSettingsBtn.width - parent.spacing * 2)
                    spacing: 0

                    Text {
                      width: parent.width; elide: Text.ElideRight
                      text: root.service && root.service.user ? String(root.service.user.display_name || root.service.user.username || "User") : "Discord"
                      color: root.foreground
                      font.family: root.fontFamily; font.pixelSize: Style.font.caption; font.bold: true
                    }
                    Text {
                      width: parent.width; elide: Text.ElideRight
                      text: root.service && root.service.user && root.service.user.username ? "@" + String(root.service.user.username) : (root.connected ? "Online" : "Offline")
                      color: root.muted
                      font.family: root.fontFamily; font.pixelSize: Style.font.caption
                    }
                  }

                  Button {
                    id: userBarSettingsBtn
                    objectName: "user-bar-settings"
                    z: 2
                    anchors.verticalCenter: parent.verticalCenter
                    iconOnly: true
                    iconName: "settings"
                    text: "Settings"
                    focusable: true
                    activeFocusOnTab: false
                    onClicked: optionsMenu.toggle("bottom-left")
                  }
                }

                MouseArea {
                  id: userBarMouse
                  anchors.fill: parent
                  z: 1
                  hoverEnabled: true
                  cursorShape: Qt.PointingHandCursor
                  onClicked: optionsMenu.toggle("bottom-left")
                }
              }
            }

            BorderSurface {
              id: channelPane
              parent: conversationRow
              objectName: "channel-pane"
              x: railPane.width + conversationRow.spacing
              visible: !root.narrowLayout || !root.compactChatOpen
              width: root.narrowLayout ? Math.max(0, conversationRow.width - railPane.width - conversationRow.spacing)
                : Math.min(Style.space(210), Math.max(Style.space(140), body.width * 0.19))
              height: root.voiceNavigation ? parent.height - channelContent.height - conversationRow.spacing : parent.height
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
                spacing: Style.spacing.xs

                PanelSectionHeader {
                  width: parent.width
                  text: root.selectedGuildName || "Channels"
                  foreground: root.foreground
                }

                Flow {
                  id: channelFilterChips
                  objectName: "channel-filter-chips"
                  width: parent.width
                  spacing: Style.spacing.xxs
                  visible: !root.dmsSelected && root.channelRows.length > 0

                  Button {
                    iconName: "all"
                    text: "All"
                    tooltipText: "All channels"
                    iconOnly: root.voiceNavigation && body.height < Style.space(400)
                    focusable: true
                    Keys.onTabPressed: root.cycleFocus(1)
                    Keys.onBacktabPressed: root.cycleFocus(-1)
                    active: root.channelFilter === "all"
                    fontSize: Style.font.caption
                    horizontalPadding: Style.spacing.xs
                    onClicked: root.channelFilter = "all"
                  }
                  Button {
                    iconName: "textChannel"
                    text: "Text"
                    tooltipText: "Text & announcement channels"
                    iconOnly: root.voiceNavigation && body.height < Style.space(400)
                    focusable: true
                    Keys.onTabPressed: root.cycleFocus(1)
                    Keys.onBacktabPressed: root.cycleFocus(-1)
                    active: root.channelFilter === "text"
                    fontSize: Style.font.caption
                    horizontalPadding: Style.spacing.xs
                    onClicked: root.channelFilter = "text"
                  }
                  Button {
                    iconName: "headphones"
                    text: "Voice"
                    tooltipText: "Voice channels"
                    iconOnly: root.voiceNavigation && body.height < Style.space(400)
                    focusable: true
                    Keys.onTabPressed: root.cycleFocus(1)
                    Keys.onBacktabPressed: root.cycleFocus(-1)
                    active: root.channelFilter === "voice"
                    fontSize: Style.font.caption
                    horizontalPadding: Style.spacing.xs
                    onClicked: root.channelFilter = "voice"
                  }
                  Button {
                    iconName: "unread"
                    text: "Unread"
                    tooltipText: "Unread channels"
                    iconOnly: root.voiceNavigation && body.height < Style.space(400)
                    focusable: true
                    Keys.onTabPressed: root.cycleFocus(1)
                    Keys.onBacktabPressed: root.cycleFocus(-1)
                    active: root.channelFilter === "unread"
                    fontSize: Style.font.caption
                    horizontalPadding: Style.spacing.xs
                    onClicked: root.channelFilter = "unread"
                  }
                }

                Text {
                  width: parent.width
                  visible: !root.filteredChannelRows.length
                  wrapMode: Text.WordWrap
                  text: !root.selectedGuildId ? "Select a server."
                    : (root.channelsLoading ? "Loading channels…"
                      : (root.dmsSelected ? "No direct messages."
                        : (root.channelFilter === "voice" ? "No voice channels in this server."
                          : (root.channelFilter === "unread" ? "No unread channels in this server."
                            : (root.channelFilter === "text" ? "No text channels in this server." : "No channels.")))))
                  color: root.muted
                  font.family: root.fontFamily
                  font.pixelSize: Style.font.body
                  leftPadding: Style.spacing.rowPaddingX
                }

                ListView {
                  id: channelList
                  objectName: "channel-results"
                  width: parent.width
                  height: parent.height - Style.spacing.controlHeight - (channelFilterChips.visible ? channelFilterChips.height + parent.spacing : 0)
                  visible: root.filteredChannelRows.length > 0
                  clip: true
                  reuseItems: true
                  cacheBuffer: Style.space(150)
                  boundsBehavior: Flickable.StopAtBounds
                  spacing: Style.spacing.xxs
                  model: root.filteredChannelRows.length
                  ScrollBar.vertical: ScrollBar { policy: ScrollBar.AsNeeded }

                  delegate: Item {
                    id: channelRow
                    required property int index
                    readonly property var row: root.filteredChannelRows[index] || ({})
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
                        objectName: "open-channel-" + String(channelRow.row.id || "")
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
                objectName: "call-bar"
                parent: channelContent
                expanded: true
                compactRosterHeader: root.voiceNavigation
                z: 0
                anchors.left: parent.left
                anchors.bottom: parent.bottom
                width: root.voiceNavigation ? parent.width : root.voicePanelWidth
                height: parent.height
                anchors.leftMargin: 0
                anchors.bottomMargin: 0
                service: root.service
                onChatRequested: root.enterComposer()
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

            Item {
              id: channelContent
              objectName: "selected-channel-content"
              x: root.narrowLayout ? (root.compactChatOpen ? 0 : railPane.width + conversationRow.spacing)
                : railPane.width + channelPane.width + conversationRow.spacing * 2
              y: root.voiceNavigation ? parent.height - height : 0
              width: Math.max(0, parent.width - x)
              height: root.voiceNavigation ? Math.min(Style.space(210), parent.height * 0.55) : parent.height
            }
            Components.CameraView {
              id: cameraView
              objectName: "camera-view"
              parent: channelContent
              anchors.fill: parent
              z: 20
              service: root.service
              onClosed: {root.compactChatOpen=root.cameraPreviousChat;root.focusZone()}
            }
            Column {
              id: chatColumn
              parent: channelContent
              objectName: "chat-column"
              visible: !root.narrowLayout || root.compactChatOpen
              x: root.voiceRoomVisible ? callBar.width + conversationRow.spacing : 0
              width: Math.max(0, parent.width - x
                - (root.membersVisible ? membersView.width + conversationRow.spacing : 0))
              height: parent.height
              spacing: Style.spacing.xs

              Item {
                id: channelHeader
                width: parent.width
                height: root.controlsRowHeight
                clip: true

                Button {
                  id: backToChannelsBtn
                  objectName: "back-to-channels"
                  visible: root.narrowLayout && root.compactChatOpen
                  anchors.left: parent.left
                  anchors.leftMargin: Style.spacing.sm
                  anchors.verticalCenter: parent.verticalCenter
                  iconOnly: false
                  iconName: "back"
                  text: "Channels"
                  tooltipText: "Back to servers and channels (Esc)"
                  focusable: true
                  onClicked: {
                    root.compactChatOpen = false
                    root.zone = "sidebar"
                    root.column = "channels"
                    root.focusZone()
                  }
                }

                Text {
                  id: channelTitle
                  anchors.left: backToChannelsBtn.visible ? backToChannelsBtn.right : parent.left
                  anchors.leftMargin: Style.spacing.sm
                  anchors.verticalCenter: parent.verticalCenter
                  width: Math.min(implicitWidth, Math.max(0, parent.width
                    - (backToChannelsBtn.visible ? backToChannelsBtn.width + Style.spacing.sm : 0)
                    - Style.spacing.sm * 2 - channelControlsSlot.width
                    - Style.spacing.controlGap))
                  elide: Text.ElideRight
                  text: root.currentChannelTitle || (root.compactView ? "Search to open a conversation" : "No channel open")
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

              Button {
                id: joinVoiceButton
                objectName: "join-channel-voice"
                visible: !!root.currentChannel && String(root.currentChannel.type || "") === "voice"
                iconName: "headphones"
                text: root.activeVoiceChannelId === root.currentChannelId ? "In voice" : "Join voice"
                tooltipText: "Connect to this voice channel"
                bordered: true
                backgroundColor: Style.normalFillFor(root.foreground, root.accent)
                width: Math.min(parent.width, Math.max(Style.space(150), implicitWidth))
                focusable: true
                fontSize: Style.font.bodySmall
                horizontalPadding: Style.spacing.xs
                Keys.onTabPressed: root.cycleFocus(1)
                Keys.onBacktabPressed: root.cycleFocus(-1)
                onClicked: root.joinVoiceChannel(root.currentChannel)
              }
              Components.Timeline {
                id: timelineView
                objectName: "timeline"
                width: parent.width
                height: parent.height - channelHeader.height - typingLine.height
                  - composerView.height - parent.spacing * 3
                  - (joinVoiceButton.visible ? joinVoiceButton.height + parent.spacing : 0)

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
                onSwitcherRequested: root.openSwitcher()
                showNavigationAction: root.narrowLayout
                onNavigationRequested: {
                  root.compactChatOpen = false
                  root.zone = "sidebar"
                  root.column = root.selectedGuildId ? "channels" : "rail"
                  root.focusZone()
                }
              }

              Item {
                width: parent.width
                height: 0
                visible: false
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
              parent: channelContent
              x: parent.width - width
              visible: root.membersVisible
              width: Math.min(Style.space(210), Math.max(Style.space(120), body.width * (root.narrowLayout ? 0.30 : 0.19)))
              height: parent.height
              dismissible: root.compactMembers
              onCloseRequested: { root.toggleMembers(); root.focusZone() }
              service: root.service
              voiceMode: root.voicePeopleMode
              voiceUsers: root.activeVoicePeople
              channelName: root.voicePeopleMode && root.service
                ? String(root.service.channelNames[root.activeVoiceChannelId] || "Voice channel") : root.currentChannelId
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

          Item {
            objectName: "error-banner"
            width: parent.width
            visible: root.errorText !== ""
            height: visible ? Math.max(dismissErrorButton.height, Math.min(errorLabel.implicitHeight, Style.space(100))) : 0
            Flickable {
              id: errorScroll
              anchors.left: parent.left
              anchors.right: dismissErrorButton.left
              anchors.rightMargin: Style.spacing.sm
              height: parent.height
              contentWidth: width
              contentHeight: errorLabel.implicitHeight
              clip: true
              boundsBehavior: Flickable.StopAtBounds
              ScrollBar.vertical: ScrollBar { policy: ScrollBar.AsNeeded }
              Text {
                id: errorLabel
                objectName: "error-message"
                width: errorScroll.width
                wrapMode: Text.WordWrap
                textFormat: Text.PlainText
                text: root.errorText
                color: Color.urgent
                font.family: root.fontFamily
                font.pixelSize: Style.font.bodySmall
              }
            }
            Button {
              id: dismissErrorButton
              objectName: "dismiss-error"
              anchors.right: parent.right
              text: "Dismiss error"
              tooltipText: "Dismiss this message without retrying the failed action"
              iconName: "close"
              iconOnly: true
              focusable: true
              onClicked: root.dismissError()
            }
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
            visible: root.ready && root.zone === "composer" && !root.composer.chipFocused
            text: "Enter to send · Shift+Enter for a new line"
            color: root.muted
            font.family: root.fontFamily
            font.pixelSize: Style.font.caption
          }
        }
      }
    }
}
