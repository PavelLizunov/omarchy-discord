import QtQuick
import Quickshell.Io

import "Api.js" as Api

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
    if (value) {
      noteActivity()
      ensureBackend()
    }
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
    applySettings(configuredEntry() || {})
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
    if (now === "ready" && was !== "ready") refreshStructure()
    if (now !== "ready" && was === "ready" && now !== "connecting") {
      guilds = []
      dms = []
      channelsByGuild = ({})
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
        if (backendState && message.total_mention_count !== undefined) {
          var next = Api.shallowCopy(backendState)
          next.total_mention_count = message.total_mention_count
          backendState = next
        }
        break
      default:
        break
    }
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
    id: reconnectGraceTimer
    interval: 3000
    onTriggered: root.reconnectGraceActive = false
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

  DaemonManager {
    id: daemonManager
  }

  BackendClient {
    id: backendClient
    wanted: daemonManager.running
  }
}
