pragma ComponentBehavior: Bound
import QtQuick
import Quickshell
import Quickshell.Io
import qs.Commons

import "Api.js" as Api
import "Markdown.js" as Markdown
import "Emoji.js" as Emoji

// Shared state for the bar widget and the lazy full panel. The Go backend is
// the source of truth; this is a mirror fed by its socket events. Runs while
// the plugin is enabled and survives panel destruction between summons.
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
    ? String(manifest.__sourceDir) : ""

  readonly property alias daemon: daemonManager
  readonly property alias backend: backendClient

  // --- settings (self-served from shell.json; see CONVENTIONS §2) ---
  readonly property var defaultSettingValues: ({
    stayConnected: "On",
    notifications: "Mentions and DMs",
    showMentionCount: "On",
    middleClick: "Last unread DM",
    imagePreviews: "On",
    mediaCacheMB: 512
  })
  property var settings: defaults()
  readonly property bool stayConnected: settings.stayConnected !== "Off"
  readonly property string notificationMode: settings.notifications
  readonly property bool showMentionCount: settings.showMentionCount !== "Off"
  readonly property string middleClickAction: settings.middleClick
  readonly property bool imagePreviews: settings.imagePreviews !== "Off"
  readonly property int mediaCacheMB: settings.mediaCacheMB
  readonly property int idleDisconnectMinutes: 15

  // --- mirrored backend state ---
  property var backendState: null
  readonly property string lifecycle: backendState ? String(backendState.lifecycle || "") : ""
  readonly property bool connected: backendClient.connected
  readonly property bool ready: connected && lifecycle === "ready"
  // A transient gateway reconnect surfaces as `connecting`; keep the browser
  // up for a short grace (BACKEND_PROTOCOL state notes) before tearing down.
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
  // guildId -> [channel]; replaced wholesale for reactivity.
  property var channelsByGuild: ({})
  property var channelsLoading: ({})
  property bool structureBusy: false
  property bool loginBusy: false
  // Highest `generation` seen on state / guilds_synced; stale ones are dropped.
  property double lastGeneration: -1

  property string lastError: ""
  property string statusMessage: ""
  // Persistent, non-fatal note (e.g. keyring unavailable); cleared on logout.
  property string notice: ""
  signal operationFailed(string reason)

  // Monitor name hosting the open full panel ("" when closed); set by Panel.qml.
  property string panelScreenName: ""
  // The panel window has keyboard focus (Panel.qml publishes Window.active);
  // gates notification suppression for the current channel.
  property bool panelActive: false

  // --- media cache mirror (backend fetch_media / media_ready) ---
  // "url|size" -> local path. Replaced wholesale: every avatar / image /
  // emoji binding reads it through mediaPath().
  property var mediaPaths: ({})
  // "url|size" -> { url, size } for every key a consumer has asked for and
  // that is not resolved yet. mediaPath() only records the miss here (in
  // place; nothing binds to it) and schedules flushMediaRequests() for the
  // next event-loop turn, so no socket write ever runs inside a binding.
  property var mediaWanted: ({})
  // "url|size" -> true while a fetch_media is outstanding; mutated in place
  // (nothing binds to it).
  property var mediaPending: ({})
  // "url|size" -> true after a failed fetch; cleared per connection so a
  // binding never loops on a dead URL.
  property var mediaFailed: ({})
  // "url|size" -> true once an evicted path has been dropped and re-fetched
  // (one retry per key per connection, so a broken file never loops).
  property var mediaRetried: ({})
  readonly property int avatarSize: 64
  readonly property int emojiSize: 32

  // --- notifications ---
  // notify-send argv builder; a harness substitutes a recorder. Only the
  // args after the binary are ours (see notificationArgs()).
  property var notifyCommand: function(args) { return ["notify-send"].concat(args) }
  readonly property int notifyWindowMs: 3000
  // channelId -> last notify-send time (in place; nothing binds to it).
  property var notifyLastAt: ({})
  // channelId -> { message, channel_name, isDm, count } held back by the
  // per-channel rate limit; flushed as one "+N more" notification.
  property var notifyHeld: ({})

  // --- QR login (start_qr_login / qr_* events) ---
  // null, or { stage: "code"|"scanned"|"approved"|"cancelled", url,
  // fingerprint, imagePath, expiresAt, revision, user, reason, error }.
  // `revision` bumps per qr_code so the panel reloads the rewritten PNG.
  property var qr: null
  property bool qrBusy: false
  property int qrRevision: 0
  // The backend replays the current qr_code on every connect while a flow
  // runs; if it has not arrived qrReplayMs after a reconnect landed us in
  // qr_pending with no code, the panel offers Cancel / Try again instead of
  // waiting forever.
  readonly property int qrReplayMs: 3000
  property bool qrMissing: false

  // --- panel view state (the panel is destroyed on hide; this restores it) ---
  property string selectedGuildId: ""
  property string currentChannelId: ""

  // --- message store ---
  // Channel ids this client has opened (per socket connection: re-sent to the
  // backend on every reconnect). Replaced wholesale.
  property var openChannels: []
  // channelId -> { channel, messages (ascending), hasMore, loading, oldestId,
  // unreadMarkerId, opened }. `opened` flips once an open_channel response
  // has landed (message_create may race ahead of it, so a non-empty
  // `messages` does not mean the channel was opened before). The map is
  // replaced wholesale on every change; the `messages` array is always a new
  // array whose elements are the same object references for unchanged rows
  // (Timeline diffs by id and keeps delegates).
  property var channelData: ({})
  // Live traffic in the open channel is kept to a rolling window: once the
  // array passes the cap it is trimmed from the top (to cap - windowSlack,
  // so the trim is not repeated on every arrival) and the dropped rows
  // become pageable history again. Never trimmed while the panel's
  // timeline is scrolled up (`timelinePinned`, maintained by Panel.qml).
  readonly property int messageWindowCap: 500
  readonly property int messageWindowSlack: 100
  property bool timelinePinned: true
  // channelId -> { unread, mention_count, last_read_message_id } from
  // read_state_changed events (never from snapshots: the structure mirror is
  // the display truth; this map is what ack/unread-marker logic consults).
  property var readState: ({})
  // channelId -> [{ user_id, display_name, at }] with 10 s expiry.
  property var typers: ({})
  // userId -> display_name, accumulated from message authors seen.
  property var knownUsers: ({})
  readonly property string selfId: user ? String(user.id || "") : ""

  // --- composer state (survives panel destruction) ---
  // channelId -> draft text. Mutated in place: nothing binds to it.
  property var drafts: ({})
  // channelId -> [{ path, filename, size, sent, total, uploading }] staged
  // attachments (files under $XDG_RUNTIME_DIR/omarchy-discord/staged/).
  // Replaced wholesale on change (chips bind to it).
  property var staged: ({})
  // Optimistic rows: request id -> pending row id, nonce -> pending row id.
  // Own echoes are matched on the nonces *we* were handed back (other
  // people's messages carry nonces too, BACKEND_PROTOCOL `message`).
  property var pendingByNonce: ({})
  // upload request id -> channelId, for routing upload_progress to chips.
  property var uploadChannels: ({})
  // channelId -> last typing command time (client-side throttle; the
  // backend throttles again to one Discord call per 10 s).
  property var lastTypingAt: ({})
  readonly property int typingThrottleMs: 8000
  property int pendingSeq: 0
  // Both clipboard steps go through this so a harness can substitute a
  // script: args are appended to wl-paste's argv.
  property var clipboardCommand: function(args) { return ["wl-paste"].concat(args) }
  readonly property string stagedDir: {
    var runtime = String(Quickshell.env("XDG_RUNTIME_DIR") || "")
    return runtime ? runtime + "/omarchy-discord/staged" : ""
  }
  // A failed send put its text back into the draft; the composer reloads it.
  signal draftRestored(string channelId)
  // Any plain-unread channel anywhere (bar dot when there are no mentions).
  readonly property bool anyUnread: {
    for (var g = 0; g < guilds.length; g++)
      if (String(guilds[g].unread || "read") !== "read") return true
    for (var d = 0; d < dms.length; d++)
      if (String(dms[d].unread || "read") !== "read") return true
    return false
  }
  // id -> name over every channel in the structure mirror (mention resolver).
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
  // Markdown.js render context: theme tokens + resolvers (harness Fixtures.ctx).
  // Also carries the media resolvers MessageRow uses (avatars, attachment
  // previews, emoji), so rows never touch the service directly. Reading
  // `mediaPaths` here makes the ctx (and every row's html) refresh when a
  // media_ready lands.
  readonly property var markdownCtx: ({
    users: knownUsers,
    channels: channelNames,
    roles: ({}),
    selfId: selfId,
    mentionColor: Color.accent,
    mentionBg: Util.alpha(Color.accent, 0.18),
    linkColor: Color.accent,
    codeBg: Util.alpha(Color.foreground, 0.08),
    spoilerColor: Color.muted,
    mutedColor: Color.muted,
    monoFamily: Style.font.family,
    fontSize: Style.font.body,
    emojiSize: Math.round(Style.font.body * 1.4),
    imagePreviews: imagePreviews,
    mediaPaths: mediaPaths,
    mediaPath: function(url, size) { return root.mediaPath(url, size) },
    mediaError: function(path) { return root.mediaError(path) },
    emojiPath: function(id, animated) { return root.emojiPath(id, animated) }
  })

  // --- reactions / emoji picker ---
  // Frequently used: [{ e, n }] persisted as a JSON string on the shell.json
  // entry (frequentEmojiKey), capped by Emoji.FREQUENT_CAP.
  property var frequentEmoji: []
  readonly property string frequentEmojiKey: "frequentEmoji"
  // Unicode catalogue [{ e, k }], loaded lazily from the shell's data file.
  property var emojiCatalog: []
  property bool emojiCatalogRequested: false
  // Custom emoji of the current guild [{ name, id, animated }] — filled by
  // list_emoji in a later wave; the picker shows a section when non-empty.
  property var serverEmoji: []
  property int quickSwitchSeq: 0

  // --- UI visibility refcount ---
  property var visibleSurfaces: ({})
  readonly property bool uiVisible: Object.keys(visibleSurfaces).length > 0
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
    // Opening and closing both count: the idle-disconnect window
    // (idleDisconnectMinutes) runs from the last surface closing.
    noteActivity()
    if (value) ensureBackend()
  }

  // --- settings plumbing ---
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
    next.imagePreviews = Api.onOff(next.imagePreviews, "On")
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
    // updateEntryInline replaces the entry wholesale, so carry unknown
    // (shell-managed) keys forward instead of dropping them.
    if (shell && typeof shell.updateEntryInline === "function")
      shell.updateEntryInline(pluginId, Api.assign(entry, next))
  }

  function configuredEntry() {
    var config = shell && shell.shellConfig ? shell.shellConfig : null
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
  }

  // Small opaque state on the same shell.json entry (spotify pattern):
  // merged over the current entry so settings are carried forward.
  function persistOpaque(key, value) {
    var entry = Api.shallowCopy(configuredEntry() || {})
    entry[key] = value
    if (shell && typeof shell.updateEntryInline === "function")
      shell.updateEntryInline(pluginId, entry)
  }

  // --- backend lifecycle ---
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

  // --- commands ---
  function send(name, fields, callback) {
    return backendClient.sendCommand(name, fields, function(ok, result, error) {
      if (!ok) fail(error)
      if (typeof callback === "function") callback(ok, result, error)
    })
  }

  // The token is forwarded straight to the socket and never stored on this
  // object. Callers must clear their own copy after invoking this.
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
      // Non-fatal: the session works, it just will not survive a restart.
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

  function loadChannels(guildId, force) {
    var id = String(guildId || "")
    if (!id || !ready) return
    if (!force && (channelsByGuild[id] !== undefined || channelsLoading[id])) return
    var loading = Api.shallowCopy(channelsLoading)
    loading[id] = true
    channelsLoading = loading
    send("list_channels", { guild_id: id }, function(ok, result) {
      var nextLoading = Api.shallowCopy(root.channelsLoading)
      delete nextLoading[id]
      root.channelsLoading = nextLoading
      if (!ok || !result || !Array.isArray(result.channels)) return
      var next = Api.shallowCopy(root.channelsByGuild)
      next[id] = result.channels
      root.channelsByGuild = next
    })
  }

  // --- message store ---
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

  // Select the view and open the channel (panel Enter, bar middle-click,
  // quick switcher). `guildId` is optional; it is resolved from the
  // open_channel response otherwise.
  function showChannel(channelId, guildId) {
    var id = String(channelId || "")
    if (!id) return
    // One channel open at a time this phase: the sidebar only needs
    // read_state_changed (global), so the previous one is closed to keep the
    // backend's per-connection stream small.
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
        // Not open on the backend: forget it here too, or every later write
        // would fail with channel_not_open instead of this error.
        root.forgetChannel(id)
        return
      }
      var rows = result && Array.isArray(result.messages) ? result.messages : []
      var channel = result && result.channel ? result.channel : null
      root.noteAuthors(rows)
      var entry = root.channelEntry(id) || ({})
      var loaded = entry.messages || []
      // A re-open (reconnect) re-sends the tail. When it overlaps the loaded
      // window, merge it in so paged history survives and live rows are not
      // duplicated, keeping the paging state already established. When the
      // whole tail is newer than anything loaded (long disconnect) the gap
      // between them could never be filled, so the old window is dropped and
      // the tail adopted with the response's own paging state.
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
        if (root.selectedGuildId !== guildId) root.selectedGuildId = guildId
        if (guildId !== "dms") root.loadChannels(guildId)
      }
    })
  }

  // Unread marker for a channel being opened: the last read_state_changed
  // seen for it, else the read marker the backend put on the channel object.
  function readMarker(channelId, channel) {
    var state = readState[channelId]
    if (state && state.last_read_message_id) return String(state.last_read_message_id)
    return channel && channel.last_read_message_id ? String(channel.last_read_message_id) : ""
  }

  // Merge a fresh newest-page over the loaded window: rows already loaded
  // keep their object identity (Timeline diffing) unless the wire content
  // changed (an edit or reaction made while disconnected), unknown rows are
  // added, result stays ascending by id.
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

  // Drop a channel from the open set and the store (no backend call).
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

  // Ack the newest loaded message of the channel (Discord acks own messages too).
  function markChannelRead(channelId) {
    var rows = messagesFor(channelId)
    if (!rows.length) return false
    return ack(channelId, rows[rows.length - 1].id)
  }

  // --- composer: drafts, optimistic send, edit/delete/react, typing, upload ---
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

  // Newest own, non-pending message in the loaded window ("" when none).
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

  // Optimistic send: the row shows immediately as pending; the gateway echo
  // (same nonce) replaces it. Failure removes the row and hands the text
  // back to the composer through the draft.
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
      // The echo may have beaten the response: then the real row is already
      // in the window and the pending one just goes away.
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

  // True when the loaded message carries our reaction with this emoji.
  function hasOwnReaction(channelId, messageId, emoji) {
    var message = findMessage(channelId, messageId)
    var list = message && Array.isArray(message.reactions) ? message.reactions : []
    var value = String(emoji || "")
    for (var i = 0; i < list.length; i++)
      if (list[i] && String(list[i].emoji || "") === value) return !!list[i].me
    return false
  }

  // Picker / chip click: add the reaction, or remove ours when it is
  // already there. Adding also bumps the frequently-used list.
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

  // The unicode catalogue is the shell's own (omarchy.emojis data file),
  // read once on first use; Emoji.FALLBACK covers a missing file.
  function ensureEmojiCatalog() {
    if (emojiCatalogRequested) return
    emojiCatalogRequested = true
    var base = String(Quickshell.env("OMARCHY_PATH") || "")
    if (!base) { emojiCatalog = Emoji.FALLBACK; return }
    emojiFile.path = base + "/shell/plugins/emojis/emojis.json"
  }

  // quick_switch: the backend ranks (unread first, then recents for an
  // empty query). Only the newest request's result reaches the callback;
  // errors go to the switcher, never the panel footer.
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

  // The switcher overlay is created on first use (Loader), then kept.
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

  // Own typing indicator, throttled per channel; silent on failure (a
  // typing error is never worth a footer line).
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

  // Remove a chip and its file on disk.
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

  // Send text + the staged files. The composer clears its input on submit
  // (like sendMessage); the chips show upload_progress (routed by this
  // request's id). Success clears them and removes the staged files, failure
  // leaves them in place, puts the text back into the draft (draftRestored,
  // as a failed send does) and the error in the footer. The draft is only
  // cleared when it still holds the text that was sent, so anything typed
  // during the upload survives.
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
        root.setStaged(id, [])
        if (text && root.draftFor(id) === text) root.setDraft(id, "")
        Quickshell.execDetached(["rm", "-f"].concat(paths))
      } else {
        root.patchStaged(id, { uploading: false })
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

  // Ctrl+V pipeline. Lists clipboard types; an image/* type is written to
  // the staging dir and becomes a chip, anything else leaves the paste to
  // the text input. `callback(staged)` runs once the decision is made.
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
    // notify:true also arrives for channels we do not have open; the store
    // only holds open channels, the notification decision runs for every one.
    maybeNotify(message)
    var entry = channelEntry(channelId)
    if (!entry || !isOpen(channelId)) return
    if (row.author && row.author.id) removeTyper(channelId, String(row.author.id))
    var rows = entry.messages || []
    var id = String(row.id)
    for (var i = rows.length - 1; i >= 0; i--)
      if (String(rows[i].id || "") === id) return
    noteAuthors([row])
    // Own echo of an optimistic send: re-key the pending row in place.
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

  // Read state: patch the channel mirror in place and reduce the guild row
  // from its loaded channel list. When the guild's channels are not loaded the
  // guild row only moves by this event's delta against the last read state
  // seen for that channel (unknown previous => treated as 0). Either way the
  // next guilds_synced / list_* replaces it with the backend's truth; the bar
  // badge itself is total_mention_count, never this reduction.
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

  // --- media cache ---
  function mediaKey(url, size) {
    return String(url) + "|" + (Math.max(0, Number(size) || 0))
  }

  // Local path for a CDN URL at `size` (0 = original), or "" while it is
  // being fetched. A miss only records the want (requestMedia); the
  // fetch_media goes out on the next event-loop turn. Safe to call from
  // bindings: the only reactive read is mediaPaths and nothing is written
  // synchronously.
  function mediaPath(url, size) {
    var u = String(url || "")
    if (!u) return ""
    var key = mediaKey(u, size)
    var known = mediaPaths[key]
    if (known) return String(known)
    requestMedia(u, size)
    return ""
  }

  // Ask for a url+size without reading anything: records the want and
  // defers the fetch_media to the next event-loop turn (Qt.callLater
  // coalesces), so callers inside bindings never write to the socket.
  function requestMedia(url, size) {
    var u = String(url || "")
    if (!u) return
    var key = mediaKey(u, size)
    if (mediaPaths[key] || mediaWanted[key]) return
    mediaWanted[key] = { url: u, size: Math.max(0, Number(size) || 0) }
    Qt.callLater(flushMediaRequests)
  }

  // Issue fetch_media for every wanted key that is neither in flight nor
  // known to fail. Runs deferred after requestMedia() and on every connect
  // (in-flight fetches die with the socket).
  function flushMediaRequests() {
    if (!connected) return
    for (var key in mediaWanted) {
      if (mediaPaths[key]) { delete mediaWanted[key]; continue }
      if (mediaPending[key] || mediaFailed[key]) continue
      mediaPending[key] = true
      fetchMedia(key, mediaWanted[key].url, mediaWanted[key].size)
    }
  }

  function fetchMedia(key, url, size) {
    var fields = { url: url }
    if (size > 0) fields.size = size
    backendClient.sendCommand("fetch_media", fields, function(ok, result, error) {
      if (!ok) { root.mediaFailed[key] = true; delete root.mediaPending[key]; return }
      if (result && result.cached && result.path) root.resolveMedia(key, String(result.path))
      // else: media_ready will follow
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

  // An Image failed to load a cached path (the backend's LRU evicted it):
  // forget the path so the bindings ask again, once per key per connection.
  // Returns true when a re-fetch was triggered.
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

  // media_ready is keyed by url only; two sizes of one url share the event.
  // With a single size outstanding the path is adopted directly, otherwise
  // each size is re-requested (a completed one is now a cache hit).
  function applyMediaReady(message) {
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

  // Custom emoji path for Markdown.js / reaction chips. Animated emoji are
  // fetched as PNG too: Qt rich text cannot animate an <img>, and one cache
  // entry per emoji is cheaper than two.
  function emojiPath(id, animated) {
    var value = String(id || "")
    if (!/^\d+$/.test(value)) return ""
    return mediaPath(emojiUrl(value), emojiSize)
  }

  function sendConfig() {
    if (!connected) return
    backendClient.sendCommand("set_config", { media_cache_mb: mediaCacheMB }, null)
  }

  // --- notifications ---
  // Why a message_create event does NOT raise a notification ("" = notify).
  // Exposed for the harness; maybeNotify() is the caller.
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
      // Inside the per-channel window: hold it, one flush per window. The
      // flush is due when the window ends, never later: a running timer is
      // left alone so continuous traffic cannot postpone it.
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

  // Fire every held notification whose window has ended; re-arm for the
  // earliest one still inside its window.
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

  // argv after "notify-send". Nothing but the preview text, the channel /
  // author names, and a cached avatar path ever goes here.
  function notificationArgs(row, channelName, isDm, count) {
    var author = row.author || {}
    var name = String(author.display_name || author.username || "Someone")
    var channel = String(channelName || "")
    var summary = isDm && (!channel || channel === name) ? name
      : name + " in " + (isDm ? channel : "#" + channel)
    // Preview text is user content by design; redact anyway so a pasted
    // token never lands in a notification daemon's history.
    var body = Api.redact(Markdown.plainText(row.content, markdownCtx))
    if (body.length > 200) body = body.slice(0, 199) + "…"
    if (Array.isArray(row.attachments) && row.attachments.length) body += (body ? " " : "") + "📎"
    if (count > 1) body += (body ? " " : "") + "(+" + (count - 1) + " more)"
    var args = ["--app-name=Omarchy Discord", "--urgency=normal"]
    var avatar = author.avatar_url ? mediaPath(String(author.avatar_url), avatarSize) : ""
    if (avatar) args.push("--icon=" + avatar)
    // The shell renders the body as styled text; the summary is plain.
    args.push("--", summary.replace(/[<>]/g, ""), Markdown.escapeHtml(body))
    return args
  }

  function fireNotification(args) {
    var argv = notifyCommand(args)
    if (Array.isArray(argv) && argv.length) Quickshell.execDetached(argv)
  }

  // --- QR login ---
  function startQrLogin() {
    if (!connected) { fail("The Discord backend is not connected yet"); return false }
    if (qrBusy || lifecycle === "qr_pending") return false
    qrBusy = true
    lastError = ""
    qr = null
    backendClient.sendCommand("start_qr_login", null, function(ok, result, error) {
      root.qrBusy = false
      if (!ok) root.fail(error || "QR login is unavailable right now")
      // Defensive: should the response beat the events, wait for them.
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

  // Cancel the flow the backend still reports and start a fresh one (the
  // panel's Try again when the code never came back after a reconnect).
  function restartQrLogin() {
    if (lifecycle !== "qr_pending") return startQrLogin()
    qrMissing = false
    return cancelQrLogin(function() { root.startQrLogin() })
  }

  // In qr_pending with no code in hand (reconnected mid-flow, or a response
  // that beat its events): the backend replays qr_code on connect, so wait
  // qrReplayMs for it before flagging the code as missing.
  function watchQrReplay() {
    if (lifecycle === "qr_pending" && qr === null && !qrBusy) {
      if (!qrReplayTimer.running) qrReplayTimer.restart()
    } else {
      qrReplayTimer.stop()
      qrMissing = false
    }
  }

  // Forget a finished QR attempt (back to the login choices).
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

  // --- event handling ---
  // Returns false (and ignores the payload) when `generation` is older than
  // the newest one seen. Messages without a generation always pass.
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
    // Arm the grace before the lifecycle flips so showStructure never blips.
    if (now === "connecting" && was === "ready") {
      reconnectGraceActive = true
      reconnectGraceTimer.restart()
    } else if (now !== "connecting") {
      reconnectGraceTimer.stop()
      reconnectGraceActive = false
    }
    backendState = next
    if (next.error) lastError = Api.redact(String(next.error))
    // A session coming up (QR approved, token login) ends the QR view.
    if (qr && (now === "connecting" || now === "ready")) qr = null
    watchQrReplay()
    if (now === "ready" && was !== "ready") {
      refreshStructure()
      // Open channels are per socket connection: re-open after every
      // (re)connect. A gateway resume on the same connection re-sends the tail,
      // which mergeTail() absorbs.
      reopenChannels()
    }
    if (now !== "ready" && was === "ready" && now !== "connecting") {
      guilds = []
      dms = []
      channelsByGuild = ({})
      clearMessages()
    }
  }

  function handleEvent(name, message) {
    switch (name) {
      case "guilds_synced": {
        if (!acceptGeneration(message.generation)) break
        if (Array.isArray(message.guilds)) guilds = message.guilds
        if (Array.isArray(message.dms)) dms = message.dms
        // Drop channel lists for guilds that went away and reload the rest in
        // place, so an open channel list survives the resync.
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
        break
      }
      case "channel_update": {
        var channel = message.channel || {}
        var guildId = String(channel.guild_id || "")
        if (guildId && channelsByGuild[guildId] !== undefined) loadChannels(guildId, true)
        else if (!guildId) send("list_dms", null, function(ok, result) {
          if (ok && result && Array.isArray(result.channels)) root.dms = result.channels
        })
        break
      }
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

  // Forget loaded messages (session gone). The open set is kept so the
  // channels are re-opened when a session comes back.
  function clearMessages() {
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
    if (typeof shell.isPluginOpen === "function" && shell.isPluginOpen(pluginId)) {
      shell.hide(pluginId)
      return "closed"
    }
    shell.summon(pluginId, "{}")
    return "opened"
  }

  function openPanel(payload) {
    if (!shell || typeof shell.summon !== "function") return "unavailable"
    var encoded = JSON.stringify(payload || ({}))
    if (typeof shell.isPluginOpen === "function" && shell.isPluginOpen(pluginId)
        && typeof shell.hide === "function") {
      // Remap onto the current workspace: split hide and summon across
      // event-loop turns so Wayland finishes unmapping first.
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

  Component.onCompleted: {
    // Deferred so shell/manifest injection lands before any startup work.
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

  // Keep the backend up while the plugin is enabled (default), re-checking
  // the unit whenever the socket is down.
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
    function onShellConfigChanged() { root.syncSettings() }
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
      // In-flight fetches died with the socket; flushMediaRequests re-issues
      // every wanted key on connect.
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
        root.backendState = null
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
  }

  // omarchy-shell quickshell.discord.switcher toggle — works from any app;
  // the overlay is created on first use and lives here (always loaded).
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

  // Clipboard pipeline processes (stageClipboardImage). Output is tiny
  // (mime list / byte count), so collecting it is fine.
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
      // Size read back separately so the write and the measurement stay
      // two plain commands.
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
