pragma ComponentBehavior: Bound
import QtQuick

// Stand-in for Service.qml inside the offscreen Panel harness: enough of the
// surface for Panel.qml and its components to build, plus call recording so
// the harness can assert what the panel asked the service to do. It is NOT a
// second implementation — every function here either records or does the one
// state assignment the real service would do. Service.qml's own logic is
// exercised against the real object in service.qml.
QtObject {
  id: mock

  // --- recording ---
  property var calls: []
  function note(name, arg) {
    var next = calls.slice()
    next.push(arg === undefined ? name : name + ":" + arg)
    calls = next
  }
  function callCount(name) {
    var n = 0
    for (var i = 0; i < calls.length; i++)
      if (calls[i] === name || calls[i].indexOf(name + ":") === 0) n++
    return n
  }
  function lastCall(name) {
    for (var i = calls.length - 1; i >= 0; i--)
      if (calls[i].indexOf(name + ":") === 0) return calls[i].substring(name.length + 1)
    return null
  }
  function reset() { calls = [] }

  // --- lifecycle / status ---
  property bool connected: true
  property string lifecycle: "ready"
  property bool showStructure: true
  property string statusText: "Connected"
  property string statusMessage: ""
  property string lastError: ""
  property string notice: ""
  property var user: ({ username: "zgt", display_name: "zgt" })
  property string selfId: "100"
  property bool loginBusy: false
  property var qr: null
  property bool qrBusy: false
  property bool qrMissing: false
  readonly property var daemon: QtObject {
    property bool runtimeChecked: true
    property bool setupBusy: false
    property bool runtimeAvailable: true
    property bool running: true
  }

  // --- structure ---
  property var guilds: []
  property var dms: []
  property var channelsByGuild: ({})
  property var threadsByParent: ({})
  property var channelsLoading: ({})
  property string selectedGuildId: ""
  property string currentChannelId: ""
  property var channelData: ({})
  property var channelNames: ({})
  property var typers: ({})
  function channelsFor(guildId) {
    var list = channelsByGuild[String(guildId || "")]
    return Array.isArray(list) ? list : []
  }
  function threadsFor(parentId, guildId) {
    var known = threadsByParent[String(parentId || "")]
    return Array.isArray(known) ? known : []
  }
  function isLoadingChannels(guildId) { return channelsLoading[String(guildId || "")] === true }
  function loadChannels(guildId, force) { note("loadChannels", guildId) }
  function listThreads(parentId) { note("listThreads", parentId) }

  // The two calls the guild-entry contract is about.
  signal guildEntered(string guildId, string channelId)
  function enterGuild(guildId) { note("enterGuild", guildId) }
  function showChannel(channelId, guildId) {
    note("showChannel", channelId)
    currentChannelId = String(channelId || "")
    if (guildId) selectedGuildId = String(guildId)
  }
  function openChannel(channelId) { note("openChannel", channelId) }
  function loadHistory(channelId) { note("loadHistory", channelId) }
  function markChannelRead(channelId) { note("markChannelRead", channelId) }
  function findMessage(channelId, messageId) { return null }
  function deleteMessage(channelId, messageId) { note("deleteMessage", messageId) }
  function toggleReaction(channelId, messageId, emoji) { note("toggleReaction", emoji); return true }
  function editMessage(channelId, messageId, content) { note("editMessage", messageId) }
  function sendMessage(channelId, content, replyTo) { note("sendMessage", channelId) }
  function typing(channelId) {}
  function lastOwnMessageId(channelId) { return "" }

  // --- member pane ---
  property bool membersWanted: false
  property string membersChannelId: ""
  property bool membersTimedOut: false
  property var memberList: null
  function setMembersWanted(value) { membersWanted = !!value; note("setMembersWanted", value) }

  // --- voice ---
  property var voice: ({ status: "idle", guildId: "", channelId: "", muted: false, deafened: false, error: "" })
  property var voiceMembers: ({})
  property var speaking: ({})
  function voiceUsers(guildId, channelId) {
    var list = voiceMembers[String(guildId || "")]
    if (!Array.isArray(list)) return []
    for (var i = 0; i < list.length; i++)
      if (list[i] && String(list[i].channel_id || "") === String(channelId || ""))
        return Array.isArray(list[i].users) ? list[i].users : []
    return []
  }
  function voiceJoin(guildId, channelId) { note("voiceJoin", channelId); return true }
  function voiceLeave() { note("voiceLeave"); return true }
  function voiceSet(options) { note("voiceSet", JSON.stringify(options || ({}))); return true }
  function toggleMute() { note("toggleMute"); return true }
  function toggleDeafen() { note("toggleDeafen"); return true }

  // --- panel plumbing ---
  property bool panelActive: false
  property bool panelMapped: false
  property string panelScreenName: ""
  property bool timelinePinned: true
  function setUiVisible(key, value) { note("setUiVisible", key + "=" + value) }
  function refresh() { note("refresh") }
  function succeed(message) {}
  function openSwitcher() { note("openSwitcher") }
  function startBackend() { note("startBackend") }
  function login(token) { note("login") }
  function logout() { note("logout") }
  function startQrLogin() { note("startQrLogin") }
  function cancelQrLogin() { note("cancelQrLogin") }
  function dismissQr() { note("dismissQr") }
  function restartQrLogin() { note("restartQrLogin") }

  // --- media / markdown / emoji ---
  property var frequentEmoji: []
  property var emojiCatalog: []
  property var serverEmoji: []
  property var staged: ({})
  readonly property var markdownCtx: ({ users: ({}), channels: ({}), roles: ({}), selfId: mock.selfId })
  function mediaPath(url, size) { return "" }
  function mediaError(path) {}
  function emojiPath(id, animated) { return "" }
  function ensureEmojiCatalog() {}
  function draftFor(channelId) { return "" }
  function setDraft(channelId, text) {}
  function stageClipboardImage(channelId) {}
  function unstage(channelId, index) {}
  function upload(channelId) {}
}
