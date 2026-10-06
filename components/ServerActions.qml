import QtQuick
import "../Api.js" as Api

QtObject {
  id: root
  property var service: null
  readonly property bool ready: !!(service && service.ready)
  readonly property var guilds: service && service.guilds ? service.guilds : []
  property var guildStats: ({})
  property var guildMute: ({})
  property bool guildStatsBusy: false
  property bool guildActionBusy: false
  property string settingsError: ""
  property string actionError: ""

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
        if (service.selectedGuildId === String(id)) service.selectedGuildId = "dms"
        service.refreshStructure()
      }
      if (callback) callback(ok, result)
    })
  }

}
