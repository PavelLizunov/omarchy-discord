pragma ComponentBehavior: Bound
import QtQuick
import Quickshell
import Quickshell.Io
import qs.Commons

import "Api.js" as Api
import "Markdown.js" as Markdown
import "Emoji.js" as Emoji

Item {
  id: root

  visible: false
  width: 0
  height: 0

  property var shell: null
  property var manifest: null
  property var pluginRegistry: null
  property var omarchyPath: null

  readonly property string pluginId: manifest && manifest.id
    ? String(manifest.id) : "quickshell.discord"
  readonly property string pluginDir: manifest && manifest.__sourceDir
    ? String(manifest.__sourceDir)
    : decodeURIComponent(String(Qt.resolvedUrl(".")).replace(/^file:\/\//, "").replace(/\/$/, ""))

  readonly property alias daemon: daemonManager
  readonly property alias backend: backendClient

  readonly property var defaultSettingValues: ({
    stayConnected: "On",
    notifications: "Mentions and DMs",
    showMentionCount: "On",
    middleClick: "Last unread DM",
    window: "On demand",
    imagePreviews: "Off",
    mediaCacheMB: 512
  })
  property var settings: defaults()
  readonly property bool stayConnected: settings.stayConnected !== "Off"
  readonly property string notificationMode: settings.notifications
  readonly property bool showMentionCount: settings.showMentionCount !== "Off"
  readonly property string middleClickAction: settings.middleClick
  readonly property bool persistentWindow: settings.window === "Persistent"
  readonly property bool imagePreviews: false
  readonly property bool textOnly: true
  readonly property int mediaCacheMB: settings.mediaCacheMB
  readonly property int idleDisconnectMinutes: 15

  property var backendState: null
  readonly property string lifecycle: backendState ? String(backendState.lifecycle || "") : ""
  readonly property bool connected: backendClient.connected
  readonly property bool ready: connected && lifecycle === "ready"
  property bool reconnectGraceActive: false
  readonly property bool showStructure: connected && (lifecycle === "ready"
    || (lifecycle === "connecting" && reconnectGraceActive))
  readonly property bool loggedOut: connected
    && (lifecycle === "logged_out" || lifecycle === "reauth_needed")
  readonly property var user: backendState && backendState.user ? backendState.user : null
  readonly property int totalMentionCount: backendState
    ? Math.max(0, Number(backendState.total_mention_count) || 0) : 0
  readonly property string unreadDmChannelId: backendState && backendState.unread_dm_channel_id
    ? String(backendState.unread_dm_channel_id) : ""
  readonly property string statusText: Api.lifecycleLabel(lifecycle, connected)
  property var guilds: []
  property var dms: []
  property var channelsByGuild: ({})
  property var channelsLoading: ({})
  property var threadsByParent: ({})
  property var threadsLoading: ({})
  property var channelsSeq: ({})
  property var threadsSeq: ({})
  property var dirtyGuilds: ({})
  property var dirtyThreadParents: ({})
  property bool structureBusy: false
  property bool loginBusy: false
  property double lastGeneration: -1

  property string lastError: ""
  property string statusMessage: ""
  property string notice: ""
  signal operationFailed(string reason)

  property string panelScreenName: ""
  property bool panelActive: false

  property var mediaPaths: ({})
  property var mediaWanted: ({})
  property var mediaPending: ({})
  property var mediaFailed: ({})
  property var mediaRetried: ({})
  readonly property int avatarSize: 64
  readonly property int emojiSize: 32
  readonly property color secondaryColor: Api.secondaryColor(Color.muted, Color.foreground, Color.background)

  property var notifyCommand: function(args) { return ["notify-send"].concat(args) }
  readonly property int notifyWindowMs: 3000
  property var notifyLastAt: ({})
  property var notifyHeld: ({})

  property var qr: null
  property bool qrBusy: false
  property int qrRevision: 0
  readonly property int qrReplayMs: 3000
  property bool qrMissing: false

  property string selectedGuildId: ""
  property string currentChannelId: ""
  property var lastChannels: []
  readonly property string lastChannelKey: "lastChannels"
  property string pendingGuildEntry: ""

  property var openChannels: []
  property var channelData: ({})
  readonly property int messageWindowCap: 500
  readonly property int messageWindowSlack: 100
  property bool timelinePinned: true
  property var readState: ({})
  property var typers: ({})
  property var knownUsers: ({})
  readonly property string selfId: user ? String(user.id || "") : ""

  property var drafts: ({})
  property var staged: ({})
  property var pendingByNonce: ({})
  property var uploadChannels: ({})
  property var lastTypingAt: ({})
  readonly property int typingThrottleMs: 8000
  property int pendingSeq: 0
  property var clipboardCommand: function(args) { return ["wl-paste"].concat(args) }
  readonly property string stagedDir: {
    var runtime = String(Quickshell.env("XDG_RUNTIME_DIR") || "")
    return runtime ? runtime + "/omarchy-discord/staged" : ""
  }
  signal draftRestored(string channelId)
  signal guildEntered(string guildId, string channelId)
  readonly property bool anyUnread: {
    for (var g = 0; g < guilds.length; g++)
      if (String(guilds[g].unread || "read") !== "read") return true
    for (var d = 0; d < dms.length; d++)
      if (String(dms[d].unread || "read") !== "read") return true
    return false
  }
  readonly property var channelNames: {
    var map = ({})
    for (var gid in channelsByGuild) {
      var list = channelsByGuild[gid]
      if (!Array.isArray(list)) continue
      for (var i = 0; i < list.length; i++) map[String(list[i].id || "")] = String(list[i].name || "")
    }
    for (var d = 0; d < dms.length; d++) map[String(dms[d].id || "")] = String(dms[d].name || "")
    return map
  }
  readonly property var markdownCtx: ({
    users: knownUsers,
    channels: channelNames,
    roles: ({}),
    selfId: selfId,
    mentionColor: Color.accent,
    mentionBg: Util.alpha(Color.accent, 0.18),
    linkColor: Color.accent,
    codeBg: Util.alpha(Color.foreground, 0.08),
    spoilerColor: Api.blend(secondaryColor, Color.background, secondaryColor.a),
    mutedColor: secondaryColor,
    monoFamily: Style.font.family,
    fontSize: Style.font.body,
    emojiSize: Math.round(Style.font.body * 1.4),
    imagePreviews: imagePreviews,
    mediaPaths: mediaPaths,
    mediaPath: function(url, size) { return root.mediaPath(url, size) },
    mediaError: function(path) { return root.mediaError(path) },
    emojiPath: function(id, animated) { return root.emojiPath(id, animated) }
  })

  property var frequentEmoji: []
  readonly property string frequentEmojiKey: "frequentEmoji"
  property var emojiCatalog: []
  property bool emojiCatalogRequested: false
  property var serverEmoji: []
  property bool serverEmojiBusy: false

  property bool membersWanted: false
  property string membersChannelId: ""
  property string membersSubscribedId: ""
  property var memberList: null
  property bool membersTimedOut: false
  readonly property int membersTimeoutMs: 15000
  property int quickSwitchSeq: 0

  property var voice: emptyVoice()
  readonly property bool inCall: String(voice.status || "idle") !== "idle"
  property var voiceMembers: ({})
  property var speaking: ({})

  property var visibleSurfaces: ({})
  readonly property bool uiVisible: Object.keys(visibleSurfaces).length > 0
  property bool panelMapped: false
  readonly property bool mediaAllowed: false
  onMediaAllowedChanged: if (mediaAllowed) flushMediaRequests()
  property double lastActivityAt: Date.now()

  function noteActivity() { lastActivityAt = Date.now() }

  function fail(reason) {
    lastError = Api.redact(String(reason || "Something went wrong"))
    operationFailed(lastError)
  }

  function succeed(message) {
    lastError = ""
    statusMessage = String(message || "")
    if (statusMessage) statusClearTimer.restart()
  }

  function setUiVisible(key, value) {
    var name = String(key || "surface")
    var next = ({})
    for (var oldKey in visibleSurfaces)
      if (oldKey !== name && visibleSurfaces[oldKey]) next[oldKey] = true
    if (value) next[name] = true
    visibleSurfaces = next
    noteActivity()
    if (value) ensureBackend()
    if (name === "full-panel") syncMembers()
  }

  function defaults() {
    var fallback = Api.shallowCopy(defaultSettingValues)
    var source = manifest && manifest.barWidget && manifest.barWidget.defaults
      ? manifest.barWidget.defaults : null
    return source ? Api.assign(fallback, source) : fallback
  }

  function normalizedSettings(values) {
    var next = defaults()
    var source = values || {}
    var keys = Object.keys(defaultSettingValues)
    for (var i = 0; i < keys.length; i++)
      if (source[keys[i]] !== undefined) next[keys[i]] = source[keys[i]]
    next.stayConnected = Api.onOff(next.stayConnected, "On")
    next.notifications = Api.oneOf(next.notifications,
      ["All", "Mentions and DMs", "Off"], "Mentions and DMs")
    next.showMentionCount = Api.onOff(next.showMentionCount, "On")
    next.middleClick = Api.oneOf(next.middleClick,
      ["Last unread DM", "Raise panel"], "Last unread DM")
    next.window = Api.oneOf(next.window, ["On demand", "Persistent"], "On demand")
    next.imagePreviews = "Off"
    next.mediaCacheMB = Api.clampInt(next.mediaCacheMB, 64, 4096, 512)
    return next
  }

  function applySettings(values) {
    var next = normalizedSettings(values)
    if (JSON.stringify(next) !== JSON.stringify(settings)) settings = next
  }

  function persistSettings(values) {
    var entry = Api.shallowCopy(configuredEntry() || {})
    var next = normalizedSettings(Api.assign(Api.shallowCopy(entry), values || {}))
    applySettings(next)
    if (shell && typeof shell.updateEntryInline === "function")
      shell.updateEntryInline(pluginId, Api.assign(entry, next))
  }

  function configuredEntry() {
    var config = shell ? { bar: shell.barConfig } : null
    if (!config) return null
    var layout = config.bar && config.bar.layout ? config.bar.layout : null
    var sections = ["left", "center", "right"]
    if (layout) {
      for (var s = 0; s < sections.length; s++) {
        var rows = Array.isArray(layout[sections[s]]) ? layout[sections[s]] : []
        for (var i = 0; i < rows.length; i++)
          if (rows[i] && String(rows[i].id || "") === pluginId) return rows[i]
      }
    }
    var plugins = Array.isArray(config.plugins) ? config.plugins : []
    for (var p = 0; p < plugins.length; p++)
      if (plugins[p] && String(plugins[p].id || "") === pluginId) return plugins[p]
    return null
  }

  function syncSettings() {
    var entry = configuredEntry() || {}
    applySettings(entry)
    var frequent = Emoji.parseFrequent(entry[frequentEmojiKey])
    if (JSON.stringify(frequent) !== JSON.stringify(frequentEmoji)) frequentEmoji = frequent
    var visits = Api.parseLastChannels(entry[lastChannelKey])
    if (JSON.stringify(visits) !== JSON.stringify(lastChannels)) lastChannels = visits
  }

  function persistOpaque(key, value) {
    var current = configuredEntry()
    if (!current) return
    var entry = Api.shallowCopy(current)
    entry[key] = value
    if (shell && typeof shell.updateEntryInline === "function")
      shell.updateEntryInline(pluginId, entry)
  }

  function ensureBackend() {
    if (!daemonManager.runtimeAvailable || daemonManager.running) return
    daemonManager.start()
  }

  function startBackend() {
    noteActivity()
    ensureBackend()
  }

  function stopBackend() {
    daemonManager.stop()
  }

  function stopBackendIfDisabled() {
    if (!pluginRegistry || typeof pluginRegistry.isEnabled !== "function") return
    if (shell && shell.pluginReloading) return
    if (pluginRegistry.isEnabled(pluginId)) return
    stopBackend()
  }

  function send(name, fields, callback) {
    return backendClient.sendCommand(name, fields, function(ok, result, error) {
      if (!ok) fail(error)
      if (typeof callback === "function") callback(ok, result, error)
    })
  }

  function login(token) {
    var value = String(token || "").trim()
    token = ""
    if (!value) {
      fail("Paste a Discord user token first")
      return false
    }
    if (!connected) {
      fail("The Discord backend is not connected yet")
      return false
    }
    loginBusy = true
    lastError = ""
    backendClient.sendCommand("login", { token: value }, function(ok, result, error) {
      loginBusy = false
      if (!ok) {
        fail(error || "Discord rejected the token")
        return
      }
      succeed("Logged in")
      notice = result && result.keyring_stored === false
        ? "Logged in, but the token could not be saved to the keyring; you'll need to log in again after a restart"
        : ""
    })
    value = ""
    return true
  }

  function logout() {
    send("logout", null, function(ok) {
      if (ok) {
        guilds = []
        dms = []
        channelsByGuild = ({})
        threadsByParent = ({})
        serverEmoji = []
        clearMessages()
        notice = ""
        succeed("Logged out")
      }
    })
  }

  function refresh() {
    noteActivity()
    if (!connected) return
    send("get_state", null, function(ok, result) {
      if (ok && result && result.lifecycle) applyState(result)
    })
    if (lifecycle === "ready") refreshStructure()
  }

  function refreshStructure() {
    if (structureBusy || !ready) return
    structureBusy = true
    var remaining = 2
    var done = function() { if (--remaining === 0) root.structureBusy = false }
    send("list_guilds", null, function(ok, result) {
      if (ok && result && Array.isArray(result.guilds)) root.guilds = result.guilds
      done()
    })
    send("list_dms", null, function(ok, result) {
      if (ok && result && Array.isArray(result.channels)) root.dms = result.channels
      done()
    })
  }

  function channelsFor(guildId) {
    var list = channelsByGuild[String(guildId || "")]
    return Array.isArray(list) ? list : []
  }

  function isLoadingChannels(guildId) {
    return channelsLoading[String(guildId || "")] === true
  }

  function enterGuild(guildId) {
    Api.browseGuild(root, guildId)
  }

  function resolveGuildEntry() {
    // Channel-list responses must never select a conversation.
    pendingGuildEntry = ""
  }

  function loadChannels(guildId, force) {
    var id = String(guildId || "")
    if (!id || !ready) return
    if (!force && (channelsByGuild[id] !== undefined || channelsLoading[id])) return
    var loading = Api.shallowCopy(channelsLoading)
    loading[id] = true
    channelsLoading = loading
    var seq = (Number(channelsSeq[id]) || 0) + 1
    channelsSeq[id] = seq
    send("list_channels", { guild_id: id }, function(ok, result) {
      if (root.channelsSeq[id] !== seq) return
      var nextLoading = Api.shallowCopy(root.channelsLoading)
      delete nextLoading[id]
      root.channelsLoading = nextLoading
      if (ok && result && Array.isArray(result.channels)) {
        var next = Api.shallowCopy(root.channelsByGuild)
        next[id] = result.channels
        root.channelsByGuild = next
      }
      root.resolveGuildEntry()
    })
  }

  function threadsFor(parentId, guildId) {
    var pid = String(parentId || "")
    var known = threadsByParent[pid]
    if (Array.isArray(known)) return known
    return Api.threadsOf(channelsFor(guildId), pid)
  }

  function listThreads(parentId, force) {
    var pid = String(parentId || "")
    if (!pid || !ready) return
    if (!force && (threadsByParent[pid] !== undefined || threadsLoading[pid])) return
    threadsLoading[pid] = true
    var seq = (Number(threadsSeq[pid]) || 0) + 1
    threadsSeq[pid] = seq
    send("list_threads", { channel_id: pid }, function(ok, result) {
      if (root.threadsSeq[pid] !== seq) return
      delete root.threadsLoading[pid]
      if (!ok || !result || !Array.isArray(result.threads)) return
      var next = Api.shallowCopy(root.threadsByParent)
      next[pid] = result.threads
      root.threadsByParent = next
    })
  }

  function noteChannelUpdate(channel) {
    var guildId = String(channel.guild_id || "")
    if (!guildId) {
      send("list_dms", null, function(ok, result) {
        if (ok && result && Array.isArray(result.channels)) root.dms = result.channels
      })
      return
    }
    if (channelsByGuild[guildId] !== undefined) dirtyGuilds[guildId] = true
    if (String(channel.type || "") === "thread" && channel.parent_id) {
      var pid = String(channel.parent_id)
      if (threadsByParent[pid] !== undefined) dirtyThreadParents[pid] = true
    }
    if (!structureFlushTimer.running) structureFlushTimer.start()
  }

  function flushStructureUpdates() {
    var guilds_ = dirtyGuilds
    var parents = dirtyThreadParents
    dirtyGuilds = ({})
    dirtyThreadParents = ({})
    for (var gid in guilds_) loadChannels(gid, true)
    for (var pid in parents) listThreads(pid, true)
  }

  function setMembersWanted(value) {
    membersWanted = !!value
    syncMembers()
  }

  function syncMembers() {
    var want = membersWanted && visibleSurfaces["full-panel"] && currentChannelId ? currentChannelId : ""
    if (want !== membersChannelId) {
      membersChannelId = want
      memberList = null
      membersTimedOut = false
      membersTimer.stop()
    }
    if (want === membersSubscribedId) return
    if (membersSubscribedId && connected)
      backendClient.sendCommand("unsubscribe_members", { channel_id: membersSubscribedId }, null)
    membersSubscribedId = ""
    if (!want) return
    if (!ready) return
    var id = want
    membersSubscribedId = id
    send("subscribe_members", { channel_id: id }, function(ok) {
      if (ok || root.membersSubscribedId !== id) return
      root.membersSubscribedId = ""
      if (root.membersChannelId === id) root.membersTimedOut = true
    })
    membersTimer.restart()
  }

  function applyMemberListUpdate(message) {
    if (String(message.channel_id || "") !== membersChannelId || !membersChannelId) return
    memberList = { channel_id: membersChannelId, guild_id: message.guild_id ? String(message.guild_id) : null,
      groups: Array.isArray(message.groups) ? message.groups : [],
      members: Array.isArray(message.members) ? message.members : [] }
    membersTimedOut = false
    membersTimer.stop()
  }

  function applyPresenceUpdate(message) {
    if (!memberList) return
    var userId = String(message.user_id || "")
    if (!userId) return
    var list = memberList.members
    var next = null
    for (var i = 0; i < list.length; i++) {
      var row = list[i]
      if (!row || !row.user || String(row.user.id || "") !== userId) continue
      if (!next) next = list.slice()
      next[i] = Api.assign(Api.shallowCopy(row), { status: String(message.status || row.status || "offline"),
        activity: message.activity !== undefined ? String(message.activity || "") : row.activity })
    }
    if (next) memberList = Api.assign(Api.shallowCopy(memberList), { members: next })
  }

  function emptyVoice() {
    return { status: "idle", guildId: "", channelId: "", muted: false, deafened: false, error: "" }
  }

  function readVoice(raw) {
    var v = raw && typeof raw === "object" ? raw : ({})
    return { status: String(v.status || "idle"),
      guildId: v.guild_id ? String(v.guild_id) : "",
      channelId: v.channel_id ? String(v.channel_id) : "",
      muted: !!v.muted, deafened: !!v.deafened,
      error: Api.redact(String(v.error || "")) }
  }

  function applyVoiceState(raw) {
    var next = readVoice(raw)
    if (next.status !== "connected") speaking = ({})
    voice = next
    if (next.guildId && channelsByGuild[next.guildId] === undefined) loadChannels(next.guildId)
  }

  function applyVoiceMembers(message) {
    var guildId = String(message.guild_id || "")
    if (!guildId) return
    var next = Api.shallowCopy(voiceMembers)
    next[guildId] = Array.isArray(message.channels) ? message.channels : []
    voiceMembers = next
  }

  function applyVoiceSpeaking(message) {
    var userId = String(message.user_id || "")
    if (!userId) return
    var next = Api.shallowCopy(speaking)
    if (message.speaking) next[userId] = true
    else delete next[userId]
    speaking = next
  }

  function voiceUsers(guildId, channelId) {
    return Api.voiceOccupants(voiceMembers[String(guildId || "")], channelId)
  }

  function voiceJoin(guildId, channelId) {
    if (!ready) return false
    var channel = String(channelId || "")
    if (!channel) return false
    send("voice_join", { guild_id: String(guildId || ""), channel_id: channel }, null)
    return true
  }

  function voiceLeave() {
    if (!ready || !inCall) return false
    send("voice_leave", null, null)
    return true
  }

  function voiceSet(options) {
    if (!ready || !inCall) return false
    var opts = options || ({})
    var fields = ({})
    if (opts.muted !== undefined) fields.muted = !!opts.muted
    if (opts.deafened !== undefined) fields.deafened = !!opts.deafened
    if (!Object.keys(fields).length) return false
    send("voice_set", fields, null)
    return true
  }

  function toggleMute() { return voiceSet({ muted: !voice.muted }) }

  function toggleDeafen() { return voiceSet({ deafened: !voice.deafened }) }

  function loadServerEmoji() {
    if (!ready || serverEmojiBusy) return
    serverEmojiBusy = true
    backendClient.sendCommand("list_emoji", null, function(ok, result) {
      root.serverEmojiBusy = false
      if (ok && result && Array.isArray(result.guilds)) root.serverEmoji = result.guilds
    })
  }

  function channelEntry(channelId) {
    var entry = channelData[String(channelId || "")]
    return entry ? entry : null
  }

  function messagesFor(channelId) {
    var entry = channelEntry(channelId)
    return entry && Array.isArray(entry.messages) ? entry.messages : []
  }

  function isOpen(channelId) {
    return openChannels.indexOf(String(channelId || "")) >= 0
  }

  function setChannelEntry(channelId, entry) {
    var next = Api.shallowCopy(channelData)
    if (entry) next[channelId] = entry
    else delete next[channelId]
    channelData = next
  }

  function patchChannelEntry(channelId, fields) {
    var current = channelEntry(channelId)
    if (!current) return
    setChannelEntry(channelId, Api.assign(Api.shallowCopy(current), fields))
  }

  function noteAuthors(messages) {
    var next = null
    for (var i = 0; i < messages.length; i++) {
      var author = messages[i] && messages[i].author ? messages[i].author : null
      if (!author || !author.id) continue
      var id = String(author.id)
      var name = String(author.display_name || author.username || "")
      if (!name || knownUsers[id] === name || (next && next[id] === name)) continue
      if (!next) next = Api.shallowCopy(knownUsers)
      next[id] = name
    }
    if (next) knownUsers = next
  }

  function showChannel(channelId, guildId) {
    var id = String(channelId || "")
    if (!id) return
    var previous = currentChannelId
    currentChannelId = id
    if (guildId !== undefined && guildId !== null && String(guildId) !== "")
      selectedGuildId = String(guildId)
    if (previous && previous !== id) closeChannel(previous)
    openChannel(id)
  }

  function openChannel(channelId) {
    var id = String(channelId || "")
    if (!id) return
    if (!isOpen(id)) openChannels = openChannels.concat([id])
    var existing = channelEntry(id)
    if (!existing) {
      setChannelEntry(id, { channel: null, messages: [], hasMore: false, loading: true,
        oldestId: "", unreadMarkerId: readMarker(id, null), opened: false })
    } else if (!existing.loading) patchChannelEntry(id, { loading: true })
    if (!ready) return
    send("open_channel", { channel_id: id }, function(ok, result) {
      if (!root.isOpen(id)) return
      if (!ok) {
        root.forgetChannel(id)
        return
      }
      var rows = result && Array.isArray(result.messages) ? result.messages : []
      var channel = result && result.channel ? result.channel : null
      root.noteAuthors(rows)
      var entry = root.channelEntry(id) || ({})
      var loaded = entry.messages || []
      var reopened = !!entry.opened
      var adoptTail = reopened && loaded.length && rows.length
        && Api.compareIds(rows[0].id, loaded[loaded.length - 1].id) > 0
      var merged = adoptTail ? rows.slice() : root.mergeTail(loaded, rows)
      root.setChannelEntry(id, {
        channel: channel,
        messages: merged,
        hasMore: reopened && !adoptTail ? !!entry.hasMore : !!(result && result.has_more),
        loading: false,
        oldestId: merged.length ? String(merged[0].id || "") : "",
        unreadMarkerId: entry.unreadMarkerId || root.readMarker(id, channel),
        opened: true
      })
      if (channel && root.currentChannelId === id) {
        var guildId = channel.guild_id ? String(channel.guild_id) : "dms"
        if (root.selectedGuildId !== guildId && !root.pendingGuildEntry)
          root.selectedGuildId = guildId
        root.noteChannelVisit(guildId, id)
        if (guildId !== "dms") root.loadChannels(guildId)
      }
    })
  }

  function readMarker(channelId, channel) {
    var state = readState[channelId]
    if (state && state.last_read_message_id) return String(state.last_read_message_id)
    return channel && channel.last_read_message_id ? String(channel.last_read_message_id) : ""
  }

  function mergeTail(loaded, tail) {
    if (!loaded.length) return tail.slice()
    if (!tail.length) return loaded.slice()
    var indexById = ({})
    for (var i = 0; i < loaded.length; i++) indexById[String(loaded[i].id || "")] = i
    var out = loaded.slice()
    var added = false
    for (var j = 0; j < tail.length; j++) {
      var tid = String(tail[j].id || "")
      var at = indexById[tid]
      if (at !== undefined) {
        if (JSON.stringify(out[at]) !== JSON.stringify(tail[j])) out[at] = tail[j]
        continue
      }
      out.push(tail[j])
      added = true
    }
    if (added) out.sort(function(a, b) { return Api.compareIds(a.id, b.id) })
    return out
  }

  function closeChannel(channelId) {
    var id = String(channelId || "")
    if (!isOpen(id)) return
    forgetChannel(id)
    if (connected) backendClient.sendCommand("close_channel", { channel_id: id }, null)
  }

  function forgetChannel(id) {
    openChannels = openChannels.filter(function(open) { return open !== id })
    setChannelEntry(id, null)
    var nextTypers = Api.shallowCopy(typers)
    delete nextTypers[id]
    typers = nextTypers
    if (currentChannelId === id) currentChannelId = ""
  }

  function reopenChannels() {
    for (var i = 0; i < openChannels.length; i++) openChannel(openChannels[i])
  }

  function loadHistory(channelId) {
    var id = String(channelId || "")
    var entry = channelEntry(id)
    if (!entry || entry.loading || !entry.hasMore || !entry.oldestId || !ready) return
    var before = entry.oldestId
    patchChannelEntry(id, { loading: true })
    send("history", { channel_id: id, before_id: before }, function(ok, result) {
      var current = root.channelEntry(id)
      if (!current) return
      if (!ok) {
        root.patchChannelEntry(id, { loading: false })
        return
      }
      var rows = result && Array.isArray(result.messages) ? result.messages : []
      root.noteAuthors(rows)
      var known = ({})
      var loaded = current.messages || []
      for (var i = 0; i < loaded.length; i++) known[String(loaded[i].id || "")] = true
      var fresh = rows.filter(function(row) { return !known[String(row.id || "")] })
      var merged = fresh.concat(loaded)
      root.setChannelEntry(id, Api.assign(Api.shallowCopy(current), {
        messages: merged,
        hasMore: !!(result && result.has_more),
        loading: false,
        oldestId: merged.length ? String(merged[0].id || "") : current.oldestId
      }))
    })
  }

  function ack(channelId, messageId) {
    var channel = String(channelId || "")
    var message = String(messageId || "")
    if (!channel || !message || !ready) return false
    var state = readState[channel]
    if (state && String(state.last_read_message_id || "") === message && !state.unread) return false
    send("ack", { channel_id: channel, message_id: message }, null)
    return true
  }

  function markChannelRead(channelId) {
    var rows = messagesFor(channelId)
    if (!rows.length) return false
    return ack(channelId, rows[rows.length - 1].id)
  }

  function draftFor(channelId) {
    var text = drafts[String(channelId || "")]
    return text === undefined ? "" : String(text)
  }

  function setDraft(channelId, text) {
    var id = String(channelId || "")
    if (!id) return
    if (text) drafts[id] = String(text)
    else delete drafts[id]
  }

  function isOwn(message) {
    return !!(message && message.author && selfId && String(message.author.id || "") === selfId)
  }

  function lastOwnMessageId(channelId) {
    var rows = messagesFor(channelId)
    for (var i = rows.length - 1; i >= 0; i--)
      if (isOwn(rows[i]) && !rows[i].pending && !rows[i].system) return String(rows[i].id || "")
    return ""
  }

  function findMessage(channelId, messageId) {
    var rows = messagesFor(channelId)
    var id = String(messageId || "")
    for (var i = rows.length - 1; i >= 0; i--) if (String(rows[i].id || "") === id) return rows[i]
    return null
  }

  function replyPreview(channelId, messageId) {
    var target = findMessage(channelId, messageId)
    if (!target) return { message_id: String(messageId || ""), author_display_name: "", preview: "" }
    var author = target.author || {}
    var text = String(target.content || "").replace(/\s+/g, " ").trim()
    if (!text && Array.isArray(target.attachments) && target.attachments.length)
      text = String(target.attachments[0].filename || "attachment")
    if (text.length > 120) text = text.slice(0, 120)
    return { message_id: String(messageId || ""),
      author_display_name: String(author.display_name || author.username || ""), preview: text }
  }

  function pendingRow(channelId, content, replyTo) {
    var me = user || {}
    return {
      id: "pending-" + (++pendingSeq),
      channel_id: channelId,
      guild_id: null,
      author: { id: selfId, username: String(me.username || ""), display_name: String(me.display_name || me.username || ""),
        avatar_url: me.avatar_url || null, bot: false },
      content: String(content || ""),
      timestamp: new Date().toISOString(),
      edited_timestamp: null,
      nonce: "",
      reply_to: replyTo ? replyPreview(channelId, replyTo) : null,
      attachments: [], embeds: [], reactions: [],
      mentions_self: false, system: false,
      pending: true
    }
  }

  function removeRow(channelId, rowId) {
    var entry = channelEntry(channelId)
    if (!entry) return
    var rows = entry.messages || []
    var next = rows.filter(function(row) { return String(row.id || "") !== rowId })
    if (next.length !== rows.length) patchChannelEntry(channelId, { messages: next })
  }

  function replaceRow(channelId, rowId, row) {
    var entry = channelEntry(channelId)
    if (!entry) return false
    var rows = entry.messages || []
    for (var i = rows.length - 1; i >= 0; i--) {
      if (String(rows[i].id || "") !== rowId) continue
      var next = rows.slice()
      next[i] = row
      patchChannelEntry(channelId, { messages: next })
      return true
    }
    return false
  }

  function forgetNonceFor(rowId) {
    var next = ({})
    var changed = false
    for (var nonce in pendingByNonce) {
      if (pendingByNonce[nonce] === rowId) { changed = true; continue }
      next[nonce] = pendingByNonce[nonce]
    }
    if (changed) pendingByNonce = next
  }

  function sendMessage(channelId, content, replyTo) {
    var id = String(channelId || "")
    var text = String(content || "")
    if (!id || !text.trim()) return false
    if (!ready || !isOpen(id)) { fail("Not connected to Discord"); return false }
    var row = pendingRow(id, text, replyTo)
    var rowId = row.id
    var fields = { channel_id: id, content: text }
    if (replyTo) fields.reply_to = String(replyTo)
    patchChannelEntry(id, { messages: messagesFor(id).concat([row]) })
    send("send", fields, function(ok, result) {
      if (!ok) {
        root.removeRow(id, rowId)
        root.setDraft(id, text)
        root.draftRestored(id)
        return
      }
      var messageId = result && result.message_id ? String(result.message_id) : ""
      var nonce = result && result.nonce ? String(result.nonce) : ""
      if (messageId && root.findMessage(id, messageId)) { root.removeRow(id, rowId); return }
      if (!nonce || !root.findMessage(id, rowId)) { root.removeRow(id, rowId); return }
      var next = Api.shallowCopy(root.pendingByNonce)
      next[nonce] = rowId
      root.pendingByNonce = next
    })
    noteActivity()
    return true
  }

  function editMessage(channelId, messageId, content, callback) {
    var text = String(content || "")
    if (!text.trim() || !ready) return false
    send("edit", { channel_id: String(channelId || ""), message_id: String(messageId || ""), content: text },
      function(ok) { if (typeof callback === "function") callback(ok) })
    return true
  }

  function deleteMessage(channelId, messageId) {
    if (!ready) return false
    send("delete", { channel_id: String(channelId || ""), message_id: String(messageId || "") }, null)
    return true
  }

  function react(channelId, messageId, emoji) {
    if (!ready) return false
    send("react", { channel_id: String(channelId || ""), message_id: String(messageId || ""), emoji: String(emoji || "") }, null)
    return true
  }

  function unreact(channelId, messageId, emoji) {
    if (!ready) return false
    send("unreact", { channel_id: String(channelId || ""), message_id: String(messageId || ""), emoji: String(emoji || "") }, null)
    return true
  }

  function hasOwnReaction(channelId, messageId, emoji) {
    var message = findMessage(channelId, messageId)
    var list = message && Array.isArray(message.reactions) ? message.reactions : []
    var value = String(emoji || "")
    for (var i = 0; i < list.length; i++)
      if (list[i] && String(list[i].emoji || "") === value) return !!list[i].me
    return false
  }

  function toggleReaction(channelId, messageId, emoji) {
    var value = String(emoji || "")
    if (!value) return false
    if (hasOwnReaction(channelId, messageId, value)) return unreact(channelId, messageId, value)
    if (!react(channelId, messageId, value)) return false
    noteEmojiUse(value)
    return true
  }

  function noteEmojiUse(emoji) {
    frequentEmoji = Emoji.bumpFrequent(frequentEmoji, emoji)
    persistOpaque(frequentEmojiKey, JSON.stringify(frequentEmoji))
  }

  function noteChannelVisit(guildId, channelId) {
    var next = Api.bumpLastChannel(lastChannels, guildId, channelId)
    if (next === lastChannels) return
    lastChannels = next
    persistOpaque(lastChannelKey, JSON.stringify(lastChannels))
  }

  function ensureEmojiCatalog() {
    if (emojiCatalogRequested) return
    emojiCatalogRequested = true
    var base = String(Quickshell.env("OMARCHY_PATH") || "")
    if (!base) { emojiCatalog = Emoji.FALLBACK; return }
    emojiFile.path = base + "/shell/plugins/emojis/emojis.json"
  }

  function quickSwitch(query, callback) {
    var seq = ++quickSwitchSeq
    var done = function(entries, error) { if (typeof callback === "function") callback(entries, error) }
    if (!connected) { done([], "The Discord backend is not connected"); return false }
    backendClient.sendCommand("quick_switch", { query: String(query || ""), limit: 20 }, function(ok, result, error) {
      if (seq !== root.quickSwitchSeq) return
      if (!ok) { done([], Api.redact(String(error || "Search failed"))); return }
      done(result && Array.isArray(result.entries) ? result.entries : [], "")
    })
    return true
  }

  function switcher() {
    switcherLoader.active = true
    var item = switcherLoader.item
    return item ? item : null
  }

  function toggleSwitcher() {
    var item = switcher()
    return item ? item.toggle() : "unavailable"
  }

  function openSwitcher() {
    var item = switcher()
    return item ? item.open() : "unavailable"
  }

  function closeSwitcher() {
    var item = switcherLoader.item
    return item ? item.close() : "closed"
  }

  function typing(channelId) {
    var id = String(channelId || "")
    if (!id || !ready || !isOpen(id)) return false
    var now = Date.now()
    var last = Number(lastTypingAt[id]) || 0
    if (now - last < typingThrottleMs) return false
    lastTypingAt[id] = now
    backendClient.sendCommand("typing", { channel_id: id }, null)
    return true
  }

  function stagedFor(channelId) {
    var list = staged[String(channelId || "")]
    return Array.isArray(list) ? list : []
  }

  function setStaged(channelId, list) {
    var next = Api.shallowCopy(staged)
    if (list && list.length) next[String(channelId || "")] = list
    else delete next[String(channelId || "")]
    staged = next
  }

  function stageFile(channelId, path, size) {
    var file = String(path || "")
    var filename = file.slice(file.lastIndexOf("/") + 1)
    setStaged(channelId, stagedFor(channelId).concat([{ path: file, filename: filename,
      size: Math.max(0, Number(size) || 0), sent: 0, total: 0, uploading: false }]))
  }

  function unstage(channelId, path) {
    var file = String(path || "")
    var kept = stagedFor(channelId).filter(function(item) { return item.path !== file })
    setStaged(channelId, kept)
    if (file) Quickshell.execDetached(["rm", "-f", file])
  }

  function patchStaged(channelId, fields, filter) {
    var list = stagedFor(channelId)
    if (!list.length) return
    setStaged(channelId, list.map(function(item) {
      return filter && !filter(item) ? item : Api.assign(Api.shallowCopy(item), fields)
    }))
  }

  function upload(channelId, content, replyTo, callback) {
    var id = String(channelId || "")
    var files = stagedFor(id)
    if (!id || !files.length) return false
    if (!ready || !isOpen(id)) { fail("Not connected to Discord"); return false }
    var paths = files.map(function(item) { return item.path })
    var fields = { channel_id: id, paths: paths }
    var text = String(content || "")
    if (text) fields.content = text
    if (replyTo) fields.reply_to = String(replyTo)
    patchStaged(id, { uploading: true, sent: 0 })
    var requestId = send("upload", fields, function(ok) {
      var remaining = Api.shallowCopy(root.uploadChannels)
      delete remaining[String(requestId)]
      root.uploadChannels = remaining
      if (ok) {
        root.setStaged(id, root.stagedFor(id).filter(function(item) { return paths.indexOf(item.path) < 0 }))
        if (text && root.draftFor(id) === text) root.setDraft(id, "")
        Quickshell.execDetached(["rm", "-f"].concat(paths))
      } else {
        root.patchStaged(id, { uploading: false }, function(item) { return paths.indexOf(item.path) >= 0 })
        if (text) {
          root.setDraft(id, text)
          root.draftRestored(id)
        }
      }
      if (typeof callback === "function") callback(ok)
    })
    if (!requestId) return false
    var next = Api.shallowCopy(uploadChannels)
    next[String(requestId)] = id
    uploadChannels = next
    noteActivity()
    return true
  }

  function applyUploadProgress(message) {
    var channelId = uploadChannels[String(message.upload_id)]
    if (!channelId) return
    var filename = String(message.filename || "")
    patchStaged(channelId, { sent: Math.max(0, Number(message.bytes_sent) || 0),
      total: Math.max(0, Number(message.bytes_total) || 0) },
      function(item) { return item.filename === filename })
  }

  function stageClipboardImage(channelId, callback) {
    var id = String(channelId || "")
    var done = function(staged) { if (typeof callback === "function") callback(staged) }
    if (!id || !stagedDir || clipboardList.running || clipboardSave.running) { done(false); return }
    clipboardList.onDone = function(types) {
      var mime = Api.bestImageType(types)
      if (!mime) { done(false); return }
      var ext = Api.imageExtension(mime)
      var target = root.stagedDir + "/paste-" + Qt.formatDateTime(new Date(), "yyyyMMdd-HHmmss")
        + "-" + (++root.pendingSeq) + "." + ext
      clipboardSave.onDone = function(ok, size) {
        if (!ok) { root.fail("Could not read the image from the clipboard"); done(false); return }
        root.stageFile(id, target, size)
        done(true)
      }
      clipboardSave.target = target
      clipboardSave.command = ["sh", "-c",
        'umask 077; mkdir -p "$(dirname "$OD_OUT")" && chmod 700 "$(dirname "$OD_OUT")" && exec "$0" "$@" > "$OD_OUT"']
        .concat(root.clipboardCommand(["--type", mime]))
      clipboardSave.environment = ({ OD_OUT: target })
      clipboardSave.running = true
    }
    clipboardList.command = clipboardCommand(["--list-types"])
    clipboardList.running = true
  }

  function typersFor(channelId) {
    var list = typers[String(channelId || "")]
    return Array.isArray(list) ? list : []
  }

  function pruneTypers() {
    var cutoff = Date.now() - 10000
    var next = ({})
    var changed = false
    var any = false
    for (var id in typers) {
      var kept = typers[id].filter(function(t) { return t.at > cutoff })
      if (kept.length !== typers[id].length) changed = true
      if (kept.length) { next[id] = kept; any = true }
    }
    if (changed) typers = next
    typerTimer.running = any
  }

  function removeTyper(channelId, userId) {
    var list = typers[channelId]
    if (!list) return
    var kept = list.filter(function(t) { return t.user_id !== userId })
    if (kept.length === list.length) return
    var next = Api.shallowCopy(typers)
    if (kept.length) next[channelId] = kept
    else delete next[channelId]
    typers = next
  }

  function applyMessageCreate(message) {
    var channelId = String(message.channel_id || "")
    var row = message.message
    if (!row || !row.id) return
    maybeNotify(message)
    var entry = channelEntry(channelId)
    if (!entry || !isOpen(channelId)) return
    if (row.author && row.author.id) removeTyper(channelId, String(row.author.id))
    var rows = entry.messages || []
    var id = String(row.id)
    for (var i = rows.length - 1; i >= 0; i--)
      if (String(rows[i].id || "") === id) return
    noteAuthors([row])
    var nonce = String(row.nonce || "")
    var pendingId = nonce ? pendingByNonce[nonce] : undefined
    if (pendingId && isOwn(row)) {
      forgetNonceFor(pendingId)
      if (replaceRow(channelId, pendingId, row)) return
    }
    var next = rows.concat([row])
    var lastReal = rows.length - 1
    while (lastReal >= 0 && rows[lastReal].pending) lastReal--
    if (lastReal >= 0 && Api.compareIds(rows[lastReal].id, id) > 0)
      next.sort(function(a, b) { return Api.compareRows(a, b) })
    var fields = { messages: next }
    var viewed = channelId === currentChannelId && !timelinePinned
    if (next.length > messageWindowCap && !viewed) {
      fields.messages = next.slice(next.length - (messageWindowCap - messageWindowSlack))
      fields.hasMore = true
    }
    fields.oldestId = fields.messages.length ? String(fields.messages[0].id || "") : ""
    patchChannelEntry(channelId, fields)
  }

  function applyMessageUpdate(message) {
    var channelId = String(message.channel_id || "")
    var row = message.message
    var entry = channelEntry(channelId)
    if (!entry || !row || !row.id) return
    var rows = entry.messages || []
    var id = String(row.id)
    for (var i = rows.length - 1; i >= 0; i--) {
      if (String(rows[i].id || "") !== id) continue
      var next = rows.slice()
      next[i] = row
      patchChannelEntry(channelId, { messages: next })
      return
    }
  }

  function applyMessageDelete(message) {
    var channelId = String(message.channel_id || "")
    var entry = channelEntry(channelId)
    if (!entry) return
    var id = String(message.message_id || "")
    var rows = entry.messages || []
    var next = rows.filter(function(row) { return String(row.id || "") !== id })
    if (next.length === rows.length) return
    patchChannelEntry(channelId, { messages: next,
      oldestId: next.length ? String(next[0].id || "") : entry.oldestId })
  }

  function applyTypingStart(message) {
    var channelId = String(message.channel_id || "")
    if (!isOpen(channelId)) return
    var userId = String(message.user_id || "")
    if (!userId || userId === selfId) return
    var list = typersFor(channelId).filter(function(t) { return t.user_id !== userId })
    list.push({ user_id: userId, display_name: String(message.display_name || knownUsers[userId] || "Someone"),
      at: Date.now() })
    var next = Api.shallowCopy(typers)
    next[channelId] = list
    typers = next
    typerTimer.running = true
  }

  function applyReadState(message) {
    var channelId = String(message.channel_id || "")
    if (!channelId) return
    var unread = !!message.unread
    var mentions = Math.max(0, Number(message.mention_count) || 0)
    var marker = unread ? (mentions > 0 ? "mentioned" : "unread") : "read"
    var previous = readState[channelId] || null
    var nextState = Api.shallowCopy(readState)
    nextState[channelId] = { unread: unread, mention_count: mentions,
      last_read_message_id: message.last_read_message_id ? String(message.last_read_message_id) : "" }
    readState = nextState

    if (backendState && message.total_mention_count !== undefined
        && Number(backendState.total_mention_count) !== Number(message.total_mention_count)) {
      var nextBackend = Api.shallowCopy(backendState)
      nextBackend.total_mention_count = message.total_mention_count
      backendState = nextBackend
    }

    var guildId = message.guild_id ? String(message.guild_id) : ""
    if (!guildId) {
      var dmIndex = indexOfId(dms, channelId)
      if (dmIndex >= 0) dms = patchedList(dms, dmIndex, { unread: marker, mention_count: mentions })
      return
    }
    var list = channelsByGuild[guildId]
    var guildUnread = marker
    var guildMentions = mentions
    var guildIndex = indexOfId(guilds, guildId)
    for (var pid in threadsByParent) {
      var threads = threadsByParent[pid]
      var tIndex = Array.isArray(threads) ? indexOfId(threads, channelId) : -1
      if (tIndex < 0) continue
      var nextThreads = Api.shallowCopy(threadsByParent)
      nextThreads[pid] = patchedList(threads, tIndex, { unread: marker, mention_count: mentions })
      threadsByParent = nextThreads
      break
    }
    if (Array.isArray(list)) {
      var index = indexOfId(list, channelId)
      if (index >= 0) {
        var nextMap = Api.shallowCopy(channelsByGuild)
        nextMap[guildId] = patchedList(list, index, { unread: marker, mention_count: mentions })
        channelsByGuild = nextMap
        list = nextMap[guildId]
      }
      guildUnread = "read"
      guildMentions = 0
      for (var i = 0; i < list.length; i++) {
        var row = list[i]
        if (String(row.type || "") === "category" || row.muted) continue
        guildMentions += Math.max(0, Number(row.mention_count) || 0)
        var state = String(row.unread || "read")
        if (state === "mentioned") guildUnread = "mentioned"
        else if (state === "unread" && guildUnread === "read") guildUnread = "unread"
      }
    } else if (guildIndex >= 0) {
      var guild = guilds[guildIndex]
      var before = previous ? previous.mention_count : 0
      guildMentions = Math.max(0, (Number(guild.mention_count) || 0) + mentions - before)
      var current = String(guild.unread || "read")
      if (marker === "read") guildUnread = guildMentions > 0 ? "mentioned" : (current === "read" ? "read" : "unread")
      else if (marker === "unread") guildUnread = current === "mentioned" && guildMentions > 0 ? "mentioned" : "unread"
    }
    if (guildIndex >= 0) {
      var g = guilds[guildIndex]
      if (String(g.unread || "read") !== guildUnread || (Number(g.mention_count) || 0) !== guildMentions)
        guilds = patchedList(guilds, guildIndex, { unread: guildUnread, mention_count: guildMentions })
    }
  }

  function indexOfId(list, id) {
    for (var i = 0; i < list.length; i++) if (String(list[i].id || "") === id) return i
    return -1
  }

  function patchedList(list, index, fields) {
    var next = list.slice()
    next[index] = Api.assign(Api.shallowCopy(list[index]), fields)
    return next
  }

  function mediaKey(url, size) {
    return String(url) + "|" + (Math.max(0, Number(size) || 0))
  }

  function mediaPath(url, size) {
    return ""
  }

  function requestMedia(url, size) {
    if (textOnly) return
    var u = String(url || "")
    if (!u) return
    var key = mediaKey(u, size)
    if (mediaPaths[key] || mediaWanted[key]) return
    mediaWanted[key] = { url: u, size: Math.max(0, Number(size) || 0) }
    Qt.callLater(flushMediaRequests)
  }

  function flushMediaRequests() {
    if (textOnly) return
    if (!connected || !mediaAllowed) return
    for (var key in mediaWanted) {
      if (mediaPaths[key]) { delete mediaWanted[key]; continue }
      if (mediaPending[key] || mediaFailed[key]) continue
      mediaPending[key] = true
      fetchMedia(key, mediaWanted[key].url, mediaWanted[key].size)
    }
  }

  function fetchMedia(key, url, size) {
    if (textOnly) return
    var fields = { url: url }
    if (size > 0) fields.size = size
    backendClient.sendCommand("fetch_media", fields, function(ok, result, error) {
      if (!ok) { root.mediaFailed[key] = true; delete root.mediaPending[key]; return }
      if (result && result.cached && result.path) root.resolveMedia(key, String(result.path))
    })
  }

  function resolveMedia(key, path) {
    delete mediaPending[key]
    delete mediaWanted[key]
    if (mediaPaths[key] === path) return
    var next = Api.shallowCopy(mediaPaths)
    next[key] = path
    mediaPaths = next
  }

  function mediaError(path) {
    var file = String(path || "")
    if (!file) return false
    var dropped = []
    for (var key in mediaPaths)
      if (String(mediaPaths[key]) === file && !mediaRetried[key]) dropped.push(key)
    if (!dropped.length) return false
    var next = Api.shallowCopy(mediaPaths)
    for (var i = 0; i < dropped.length; i++) {
      mediaRetried[dropped[i]] = true
      delete next[dropped[i]]
    }
    mediaPaths = next
    for (var d = 0; d < dropped.length; d++) {
      var at = dropped[d].lastIndexOf("|")
      requestMedia(dropped[d].slice(0, at), Number(dropped[d].slice(at + 1)) || 0)
    }
    return true
  }

  function applyMediaReady(message) {
    if (textOnly) return
    var url = String(message.url || "")
    if (!url) return
    var prefix = url + "|"
    var keys = []
    for (var key in mediaPending) if (key.indexOf(prefix) === 0) keys.push(key)
    if (!keys.length) return
    if (!message.ok) {
      for (var f = 0; f < keys.length; f++) { mediaFailed[keys[f]] = true; delete mediaPending[keys[f]] }
      return
    }
    if (keys.length === 1 && message.path) { resolveMedia(keys[0], String(message.path)); return }
    for (var k = 0; k < keys.length; k++) fetchMedia(keys[k], url, Number(keys[k].slice(prefix.length)) || 0)
  }

  function emojiUrl(id) {
    return "https://cdn.discordapp.com/emojis/" + String(id) + ".png"
  }

  function emojiPath(id, animated) {
    var value = String(id || "")
    if (!/^\d+$/.test(value)) return ""
    return mediaPath(emojiUrl(value), emojiSize)
  }

  function sendConfig() {
    if (!connected) return
    backendClient.sendCommand("set_config", { media_cache_mb: mediaCacheMB }, null)
  }

  function notifySkipReason(message) {
    if (!message || !message.notify) return "not-notify"
    var row = message.message
    if (!row || !row.author) return "no-message"
    if (lifecycle !== "ready") return "not-ready"
    if (notificationMode === "Off") return "mode-off"
    if (isOwn(row)) return "own"
    if (backendState && String(backendState.presence || "") === "dnd") return "dnd"
    var isDm = message.guild_id === null || message.guild_id === undefined || String(message.guild_id) === ""
    if (notificationMode !== "All" && !row.mentions_self && !isDm) return "mode-mentions"
    var channelId = String(message.channel_id || "")
    if (panelActive && visibleSurfaces["full-panel"] && channelId === currentChannelId) return "viewing"
    return ""
  }

  function maybeNotify(message) {
    if (notifySkipReason(message) !== "") return
    var channelId = String(message.channel_id || "")
    var now = Date.now()
    var last = Number(notifyLastAt[channelId]) || 0
    var isDm = message.guild_id === null || message.guild_id === undefined || String(message.guild_id) === ""
    if (now - last < notifyWindowMs) {
      var held = notifyHeld[channelId]
      notifyHeld[channelId] = { message: message.message, channel_name: String(message.channel_name || ""),
        isDm: isDm, count: held ? held.count + 1 : 1 }
      if (!notifyFlushTimer.running) scheduleNotifyFlush(last + notifyWindowMs - now)
      return
    }
    notifyLastAt[channelId] = now
    fireNotification(notificationArgs(message.message, String(message.channel_name || ""), isDm, 1))
  }

  function scheduleNotifyFlush(ms) {
    notifyFlushTimer.interval = Math.max(1, Math.ceil(ms))
    notifyFlushTimer.restart()
  }

  function flushNotifications() {
    var now = Date.now()
    var nextDue = -1
    for (var channelId in notifyHeld) {
      var held = notifyHeld[channelId]
      var remaining = (Number(notifyLastAt[channelId]) || 0) + notifyWindowMs - now
      if (remaining > 0) {
        if (nextDue < 0 || remaining < nextDue) nextDue = remaining
        continue
      }
      delete notifyHeld[channelId]
      notifyLastAt[channelId] = now
      fireNotification(notificationArgs(held.message, held.channel_name, held.isDm, held.count))
    }
    if (nextDue >= 0) scheduleNotifyFlush(nextDue)
  }

  function notificationArgs(row, channelName, isDm, count) {
    var author = row.author || {}
    var name = String(author.display_name || author.username || "Someone")
    var channel = String(channelName || "")
    var summary = isDm && (!channel || channel === name) ? name
      : name + " in " + (isDm ? channel : "#" + channel)
    var body = Api.redact(Markdown.plainText(row.content, markdownCtx))
    if (body.length > 200) body = body.slice(0, 199) + "…"
    if (Array.isArray(row.attachments) && row.attachments.length) body += (body ? " " : "") + "📎"
    if (count > 1) body += (body ? " " : "") + "(+" + (count - 1) + " more)"
    var args = ["--app-name=Omarchy Discord", "--urgency=normal"]
    var avatar = author.avatar_url ? mediaPath(String(author.avatar_url), avatarSize) : ""
    if (avatar) args.push("--icon=" + avatar)
    args.push("--", summary.replace(/[<>]/g, ""), Markdown.escapeHtml(body))
    return args
  }

  function fireNotification(args) {
    var argv = notifyCommand(args)
    if (Array.isArray(argv) && argv.length) Quickshell.execDetached(argv)
  }

  function startQrLogin() {
    if (!connected) { fail("The Discord backend is not connected yet"); return false }
    if (qrBusy || lifecycle === "qr_pending") return false
    qrBusy = true
    lastError = ""
    qr = null
    backendClient.sendCommand("start_qr_login", null, function(ok, result, error) {
      root.qrBusy = false
      if (!ok) root.fail(error || "QR login is unavailable right now")
      else root.watchQrReplay()
    })
    return true
  }

  function cancelQrLogin(callback) {
    if (!connected) return false
    backendClient.sendCommand("cancel_qr_login", null, function(ok, result, error) {
      if (typeof callback === "function") callback(ok)
    })
    return true
  }

  function restartQrLogin() {
    if (lifecycle !== "qr_pending") return startQrLogin()
    qrMissing = false
    return cancelQrLogin(function() { root.startQrLogin() })
  }

  function watchQrReplay() {
    if (lifecycle === "qr_pending" && qr === null && !qrBusy) {
      if (!qrReplayTimer.running) qrReplayTimer.restart()
    } else {
      qrReplayTimer.stop()
      qrMissing = false
    }
  }

  function dismissQr() {
    qr = null
  }

  function applyQrEvent(name, message) {
    var current = qr || ({})
    qrReplayTimer.stop()
    qrMissing = false
    switch (name) {
      case "qr_code":
        qr = { stage: "code", url: String(message.url || ""), fingerprint: String(message.fingerprint || ""),
          imagePath: String(message.image_path || ""),
          expiresAt: Date.now() + Math.max(0, Number(message.expires_in_ms) || 0),
          revision: ++qrRevision, user: null, reason: "", error: "" }
        break
      case "qr_scanned":
        qr = Api.assign(Api.shallowCopy(current), { stage: "scanned", user: message.user || null })
        break
      case "qr_approved":
        qr = Api.assign(Api.shallowCopy(current), { stage: "approved" })
        break
      case "qr_cancelled":
        qr = Api.assign(Api.shallowCopy(current), { stage: "cancelled",
          reason: String(message.reason || "cancelled"), error: Api.redact(String(message.error || "")) })
        break
      default:
        break
    }
  }

  function acceptGeneration(generation) {
    if (generation === undefined || generation === null) return true
    var value = Number(generation)
    if (!isFinite(value)) return true
    if (value < lastGeneration) return false
    lastGeneration = value
    return true
  }

  function applyState(next) {
    if (!next || typeof next !== "object") return
    if (!acceptGeneration(next.generation)) return
    var previous = backendState
    var was = previous ? String(previous.lifecycle || "") : ""
    var now = String(next.lifecycle || "")
    if (now === "connecting" && was === "ready") {
      reconnectGraceActive = true
      reconnectGraceTimer.restart()
    } else if (now !== "connecting") {
      reconnectGraceTimer.stop()
      reconnectGraceActive = false
    }
    backendState = next
    applyVoiceState(next.voice)
    if (next.error) lastError = Api.redact(String(next.error))
    if (qr && (now === "connecting" || now === "ready")) qr = null
    watchQrReplay()
    if (now === "ready" && was !== "ready") {
      refreshStructure()
      loadServerEmoji()
      reopenChannels()
      membersSubscribedId = ""
      syncMembers()
    }
    if (now !== "ready" && was === "ready" && now !== "connecting") {
      guilds = []
      dms = []
      channelsByGuild = ({})
      threadsByParent = ({})
      serverEmoji = []
      voiceMembers = ({})
      clearMessages()
    }
  }

  function handleEvent(name, message) {
    switch (name) {
      case "guilds_synced": {
        if (!acceptGeneration(message.generation)) break
        if (Array.isArray(message.guilds)) guilds = message.guilds
        if (Array.isArray(message.dms)) dms = message.dms
        var present = ({})
        for (var g = 0; g < guilds.length; g++) present[String(guilds[g].id || "")] = true
        var kept = ({})
        var reload = []
        for (var loaded in channelsByGuild) {
          if (!present[loaded]) continue
          kept[loaded] = channelsByGuild[loaded]
          reload.push(loaded)
        }
        channelsByGuild = kept
        for (var r = 0; r < reload.length; r++) loadChannels(reload[r], true)
        loadServerEmoji()
        break
      }
      case "channel_update":
        noteChannelUpdate(message.channel || {})
        break
      case "member_list_update":
        applyMemberListUpdate(message)
        break
      case "voice_members":
        applyVoiceMembers(message)
        break
      case "voice_speaking":
        applyVoiceSpeaking(message)
        break
      case "presence_update":
        applyPresenceUpdate(message)
        break
      case "read_state_changed":
        applyReadState(message)
        break
      case "message_create":
        applyMessageCreate(message)
        break
      case "message_update":
        applyMessageUpdate(message)
        break
      case "message_delete":
        applyMessageDelete(message)
        break
      case "typing_start":
        applyTypingStart(message)
        break
      case "upload_progress":
        applyUploadProgress(message)
        break
      case "media_ready":
        applyMediaReady(message)
        break
      case "qr_code":
      case "qr_scanned":
      case "qr_approved":
      case "qr_cancelled":
        applyQrEvent(name, message)
        break
      default:
        break
    }
  }

  function clearMessages() {
    pendingGuildEntry = ""
    channelData = ({})
    readState = ({})
    typers = ({})
    knownUsers = ({})
    pendingByNonce = ({})
    uploadChannels = ({})
    typerTimer.running = false
  }

  function togglePanel() {
    if (!shell || typeof shell.toggle !== "function") return "unavailable"
    var open = persistentWindow ? (panelMapped && panelActive)
      : (typeof shell.isPluginOpen === "function" && shell.isPluginOpen(pluginId))
    if (open) {
      shell.hide(pluginId)
      return "closed"
    }
    shell.summon(pluginId, "{}")
    return "opened"
  }

  function openPanel(payload) {
    if (!shell || typeof shell.summon !== "function") return "unavailable"
    var encoded = JSON.stringify(payload || ({}))
    if (!persistentWindow && typeof shell.isPluginOpen === "function" && shell.isPluginOpen(pluginId)
        && typeof shell.hide === "function") {
      shell.hide(pluginId)
      Qt.callLater(function() { if (root.shell) root.shell.summon(root.pluginId, encoded) })
      return "opened"
    }
    shell.summon(pluginId, encoded)
    return "opened"
  }

  function closePanel() {
    if (!shell || typeof shell.hide !== "function") return "unavailable"
    shell.hide(pluginId)
    return "closed"
  }

  onShellChanged: settingsSync.restart()
  onPluginDirChanged: daemonManager.pluginDir = pluginDir
  onMediaCacheMBChanged: sendConfig()
  onCurrentChannelIdChanged: syncMembers()
  onDmsChanged: resolveGuildEntry()

  Component.onCompleted: {
    settingsSync.start()
  }

  Timer {
    id: settingsSync
    interval: 0
    onTriggered: {
      root.syncSettings()
      daemonManager.pluginDir = root.pluginDir
      daemonManager.checkRequirements()
      daemonManager.refreshStatus()
    }
  }

  Timer {
    id: qrReplayTimer
    interval: root.qrReplayMs
    onTriggered: root.qrMissing = root.lifecycle === "qr_pending" && root.qr === null && !root.qrBusy
  }

  Timer {
    id: structureFlushTimer
    interval: 300
    onTriggered: root.flushStructureUpdates()
  }

  Timer {
    id: membersTimer
    interval: root.membersTimeoutMs
    onTriggered: root.membersTimedOut = root.membersChannelId !== "" && root.memberList === null
  }

  Timer {
    id: reconnectGraceTimer
    interval: 3000
    onTriggered: root.reconnectGraceActive = false
  }

  Timer {
    id: typerTimer
    interval: 1000
    repeat: true
    onTriggered: root.pruneTypers()
  }

  Timer {
    id: notifyFlushTimer
    interval: root.notifyWindowMs
    onTriggered: root.flushNotifications()
  }

  Timer {
    id: statusClearTimer
    interval: 4500
    onTriggered: root.statusMessage = ""
  }

  Timer {
    id: keepAliveTimer
    interval: 5000
    repeat: true
    running: daemonManager.runtimeAvailable && !backendClient.connected
      && (root.stayConnected || root.uiVisible)
    triggeredOnStart: true
    onTriggered: {
      if (!daemonManager.running) daemonManager.start()
      else daemonManager.refreshStatus()
    }
  }

  Timer {
    id: idleTimer
    interval: 60000
    repeat: true
    running: daemonManager.running && !root.stayConnected && !root.uiVisible
    onTriggered: {
      if (Date.now() - root.lastActivityAt >= root.idleDisconnectMinutes * 60000)
        root.stopBackend()
    }
  }

  Connections {
    target: root.shell
    ignoreUnknownSignals: true
    function onBarConfigChanged() { root.syncSettings() }
  }

  Connections {
    target: root.pluginRegistry
    ignoreUnknownSignals: true
    function onPluginsChanged() { root.stopBackendIfDisabled() }
  }

  Connections {
    target: daemonManager
    function onSetupSucceeded() { root.ensureBackend() }
    function onSetupFailed(reason) { root.fail(reason) }
    function onLastErrorChanged() {
      if (daemonManager.lastError) root.fail(daemonManager.lastError)
    }
  }

  Connections {
    target: backendClient
    function onStateReceived(state) { root.applyState(state) }
    function onEventReceived(name, message) { root.handleEvent(name, message) }
    function onConfigurationFailed(reason) { root.fail(reason) }
    function onConnectedChanged() {
      root.mediaPending = ({})
      root.mediaFailed = ({})
      root.mediaRetried = ({})
      if (backendClient.connected) {
        root.sendConfig()
        root.flushMediaRequests()
      }
      else {
        root.qr = null
        root.qrMissing = false
        qrReplayTimer.stop()
        root.notifyHeld = ({})
      }
      if (!backendClient.connected) {
        root.membersSubscribedId = ""
        root.membersChannelId = ""
        root.memberList = null
        root.membersTimedOut = false
        membersTimer.stop()
        root.dirtyGuilds = ({})
        root.dirtyThreadParents = ({})
        root.threadsLoading = ({})
        root.backendState = null
        root.voice = root.emptyVoice()
        root.voiceMembers = ({})
        root.speaking = ({})
        root.lastGeneration = -1
        reconnectGraceTimer.stop()
        root.reconnectGraceActive = false
        daemonManager.refreshStatus()
      }
    }
  }

  IpcHandler {
    target: root.pluginId + ".panel"

    function toggle(): string { return root.togglePanel() }
    function open(): string { return root.openPanel(null) }
    function close(): string { return root.closePanel() }
    function status(): string {
      return JSON.stringify({ pluginDir: root.pluginDir, textOnly: root.textOnly,
        mediaQueued: Object.keys(root.mediaWanted).length, mediaPending: Object.keys(root.mediaPending).length,
        runtimeAvailable: daemonManager.runtimeAvailable,
        running: daemonManager.running, connected: root.connected, lifecycle: root.lifecycle,
        error: Api.redact(daemonManager.lastError || root.lastError) })
    }
  }

  IpcHandler {
    target: root.pluginId + ".voice"

    function mute(): string { return root.toggleMute() ? "ok" : "no call" }
    function deafen(): string { return root.toggleDeafen() ? "ok" : "no call" }
    function leave(): string { return root.voiceLeave() ? "ok" : "no call" }
  }

  IpcHandler {
    target: root.pluginId + ".switcher"

    function toggle(): string { return root.toggleSwitcher() }
    function open(): string { return root.openSwitcher() }
    function close(): string { return root.closeSwitcher() }
  }

  Component {
    id: switcherComponent
    QuickSwitch {
      service: root
    }
  }

  Loader {
    id: switcherLoader
    active: false
    sourceComponent: switcherComponent
  }

  FileView {
    id: emojiFile
    path: ""
    onLoaded: {
      var parsed = Emoji.parseCatalog(text())
      root.emojiCatalog = parsed.length ? parsed : Emoji.FALLBACK
    }
    onLoadFailed: root.emojiCatalog = Emoji.FALLBACK
  }

  Process {
    id: clipboardList
    property var onDone: null
    stdout: StdioCollector {
      onStreamFinished: {
        var types = String(text || "").split("\n").map(function(t) { return t.trim() })
          .filter(function(t) { return t !== "" })
        var cb = clipboardList.onDone
        clipboardList.onDone = null
        if (typeof cb === "function") cb(types)
      }
    }
    onExited: function(code) {
      if (code === 0) return
      var cb = clipboardList.onDone
      clipboardList.onDone = null
      if (typeof cb === "function") cb([])
    }
  }

  Process {
    id: clipboardSave
    property var onDone: null
    property string target: ""
    onExited: function(code) {
      var cb = clipboardSave.onDone
      clipboardSave.onDone = null
      if (code !== 0) {
        Quickshell.execDetached(["rm", "-f", clipboardSave.target])
        if (typeof cb === "function") cb(false, 0)
        return
      }
      sizeProbe.onDone = cb
      sizeProbe.command = ["stat", "-c", "%s", clipboardSave.target]
      sizeProbe.running = true
    }
  }

  Process {
    id: sizeProbe
    property var onDone: null
    stdout: StdioCollector {
      onStreamFinished: {
        var cb = sizeProbe.onDone
        sizeProbe.onDone = null
        var size = Number(String(text || "").trim())
        if (typeof cb === "function") cb(true, isFinite(size) ? size : 0)
      }
    }
  }

  DaemonManager {
    id: daemonManager
  }

  BackendClient {
    id: backendClient
    wanted: daemonManager.running
  }
}
