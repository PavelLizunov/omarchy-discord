import QtQuick

Item {
  id: root
  objectName: "voice-roster-sync"
  visible: false
  property var service: null
  property bool active: false
  readonly property var backend: service && service.backend ? service.backend : null
  readonly property bool ready: !!(service && service.connected && service.lifecycle === "ready")
  readonly property var guilds: service && service.guilds ? service.guilds : []
  property bool busy: false
  property int revision: 0
  property int refreshed: 0
  property var eventRevisions: ({})

  function cancel() {
    revision++
    busy = false
    deadline.stop()
  }
  function refresh() {
    if (!ready || !backend || busy) return false
    var ids = guilds.map(function(g) { return String(g.id || "") }).filter(function(id) { return !!id })
    var generation = ++revision, owner = service, client = backend, index = 0
    busy = true
    refreshed = 0
    deadline.restart()
    function step() {
      if (generation !== root.revision || owner !== root.service || !root.ready) return
      if (index >= ids.length) { root.busy = false; deadline.stop(); return }
      var id = ids[index++], eventRevision = Number(root.eventRevisions[id]) || 0
      client.sendCommand("voice_members", {guild_id: id}, function(ok, result) {
        if (generation !== root.revision || owner !== root.service || !root.ready) return
        if (ok && result && String(result.guild_id) === id
            && eventRevision === (Number(root.eventRevisions[id]) || 0)) {
          owner.applyVoiceMembers(result)
          root.refreshed++
        }
        Qt.callLater(step)
      })
    }
    step()
    return true
  }
  onServiceChanged: { cancel(); eventRevisions = ({}); Qt.callLater(refresh) }
  onReadyChanged: { cancel(); eventRevisions = ({}); if (ready) Qt.callLater(refresh) }
  onGuildsChanged: { cancel(); Qt.callLater(refresh) }
  onActiveChanged: if (active) Qt.callLater(refresh)
  Component.onCompleted: Qt.callLater(refresh)
  Component.onDestruction: cancel()
  Connections {
    target: root.backend
    ignoreUnknownSignals: true
    function onEventReceived(name, message) {
      if (name !== "voice_members") return
      var id = String(message.guild_id || "")
      if (id) root.eventRevisions[id] = (Number(root.eventRevisions[id]) || 0) + 1
    }
    function onConnectedChanged() { root.cancel(); if (root.ready) Qt.callLater(root.refresh) }
  }
  Timer { id: deadline; objectName: "voice-roster-deadline"; interval: 15000; onTriggered: root.cancel() }
  Timer { objectName: "voice-roster-timer"; interval: 15000; repeat: true; running: root.active && root.ready; onTriggered: root.refresh() }
}
