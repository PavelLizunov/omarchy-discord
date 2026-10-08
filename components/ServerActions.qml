import QtQuick
import "../Api.js" as Api
import "../ServerList.js" as ServerList

QtObject {
  id: root
  property var service: null
  readonly property bool ready: !!(service && service.ready)
  readonly property var guilds: service && service.guilds ? service.guilds : []
  property var guildStats: ({})
  property var guildMute: ({})
  property var archivedGuilds: ({})
  readonly property int archivedCount: ServerList.archivedCount(guilds, archivedGuilds)
  property bool guildStatsBusy: false
  property bool guildActionBusy: false
  property string settingsError: ""
  property string actionError: ""

  onReadyChanged: {
    if (!ready) {
      guildActionBusy = false
      guildStatsBusy = false
    } else {
      loadArchived()
    }
  }
  onServiceChanged: if (ready) loadArchived()

  function isArchived(id) {
    return !!archivedGuilds[String(id)]
  }

  function loadArchived() {
    if (!service) return
    var entry = typeof service.configuredEntry === "function" ? service.configuredEntry() : null
    var raw = entry && entry.archivedGuilds !== undefined ? entry.archivedGuilds : null
    var list = []
    if (typeof raw === "string") {
      try { list = JSON.parse(raw) } catch (e) { list = [] }
    } else if (Array.isArray(raw)) {
      list = raw
    }
    var map = {}
    if (Array.isArray(list)) {
      for (var i = 0; i < list.length; i++) {
        var gid = String(list[i] || "")
        if (gid) map[gid] = true
      }
    }
    archivedGuilds = map
  }

  function toggleArchive(id, archived) {
    var next = Api.shallowCopy(archivedGuilds)
    var gid = String(id || "")
    if (!gid) return
    if (archived) next[gid] = true
    else delete next[gid]
    archivedGuilds = next
    if (service && typeof service.persistOpaque === "function") {
      service.persistOpaque("archivedGuilds", JSON.stringify(Object.keys(next)))
    }
  }

  function fetchGuildStats(callback) {
    if (!ready || guildStatsBusy) return false
    guildStatsBusy = true
    var ids = guilds.map(function(g) { return String(g.id) })
    var next = ({})
    var index = 0
    var deadline = Date.now() + 120000
    // One finite serial sweep on explicit request; no timer or retry loop.
    function step() {
      if (index >= ids.length || index >= 200 || Date.now() >= deadline || !root.ready) {
        root.guildStats = next
        root.guildStatsBusy = false
        if (callback) callback()
        return
      }
      var id = ids[index++]
      service.backend.sendCommand("guild_stats", {guild_id:id}, function(ok, result) {
        if (ok && result && typeof result.online_count === "number") {
          result.fetched_at = Date.now()
          next[id] = result
        }
        Qt.callLater(step)
      })
    }
    step()
    return true
  }

  function loadGuildSettings(id) {
    settingsError = ""
    if (!ready) { settingsError = "Not connected. Reconnect and try again."; return false }
    var unknown = Api.shallowCopy(guildMute)
    delete unknown[String(id)]
    guildMute = unknown
    service.send("guild_settings", {guild_id:String(id)}, function(ok, result, error) {
      if (!ok) { root.settingsError = String(error || "Server settings unavailable"); return }
      var next = Api.shallowCopy(root.guildMute)
      next[String(id)] = !!result.muted
      root.guildMute = next
    })
  }

  function serverAction(name, id, fields, callback) {
    if (!ready) { actionError = "Not connected. Reconnect and try again."; return false }
    if (guildActionBusy) return false
    guildActionBusy = true
    actionError = ""
    return service.send(name, Api.assign({guild_id:String(id)}, fields || {}), function(ok, result, error) {
      if (!ok) root.actionError = String(error || "Server action failed")
      root.guildActionBusy = false
      if (ok && name === "set_guild_mute") {
        var next = Api.shallowCopy(root.guildMute)
        next[String(id)] = !!fields.muted
        root.guildMute = next
      }
      if (ok && name === "leave_guild") {
        var entry = service.channelEntry(service.currentChannelId)
        if (entry && entry.channel && String(entry.channel.guild_id) === String(id)) {
          service.closeChannel(service.currentChannelId)
          service.currentChannelId = ""
        }
        if (service.voice && String(service.voice.guildId || "") === String(id)) {
          service.voiceLeave()
        }
        if (service.selectedGuildId === String(id)) service.selectedGuildId = "dms"
        service.refreshStructure()
      }
      if (callback) callback(ok, result)
    })
  }

}
