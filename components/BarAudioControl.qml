import QtQuick

Item {
  id: root
  visible: false
  property var service: null
  readonly property var voice: service && service.voice ? service.voice : ({})
  readonly property bool available: !!(service && service.ready && voice.status === "connected")
  readonly property string room: available ? String(voice.channelId || "") : ""
  property bool busy: false
  property bool savedMuted: false
  property bool hasSavedMute: false
  property int revision: 0
  property bool previousDeafened: false
  property var pendingFields: null
  property bool replyReceived: false

  function reset() {
    revision++
    timeout.stop()
    busy = false
    pendingFields = null
    replyReceived = false
    hasSavedMute = false
    previousDeafened = !!voice.deafened
  }
  onRoomChanged: reset()
  onServiceChanged: reset()

  function sync() {
    if (busy && replyReceived && pendingFields
        && (pendingFields.muted === undefined || voice.muted === pendingFields.muted)
        && (pendingFields.deafened === undefined || voice.deafened === pendingFields.deafened)) {
      timeout.stop()
      busy = false
      pendingFields = null
      if (!voice.deafened) hasSavedMute = false
    }
    if (previousDeafened && !voice.deafened && !busy) hasSavedMute = false
    previousDeafened = !!voice.deafened
  }
  onVoiceChanged: sync()

  function request(fields) {
    if (!available || busy || !service.backend) return false
    var token = ++revision, owner = room
    pendingFields = fields
    replyReceived = false
    busy = true
    timeout.start()
    service.backend.sendCommand("voice_set", fields, function(ok, result, error) {
      if (token !== root.revision || owner !== root.room) return
      if (ok) {
        root.replyReceived = true
        root.sync()
      } else {
        root.timeoutStop()
        root.busy = false
        root.pendingFields = null
        if (!root.voice.deafened) root.hasSavedMute = false
        if (root.service && typeof root.service.fail === "function") root.service.fail(error)
      }
    })
    return true
  }
  function timeoutStop() { timeout.stop() }
  function toggleMicrophone() {
    if (voice.deafened) return false
    return request({muted: !voice.muted})
  }
  function toggleSilence() {
    if (!available || busy || !service.backend) return false
    if (voice.deafened)
      return request({muted: hasSavedMute ? savedMuted : !!voice.muted, deafened: false})
    savedMuted = !!voice.muted
    hasSavedMute = true
    return request({muted: true, deafened: true})
  }
  Timer {
    id: timeout
    interval: 5000
    onTriggered: {
      root.revision++
      root.busy = false
      root.pendingFields = null
      if (!root.voice.deafened) root.hasSavedMute = false
      if (root.service && typeof root.service.fail === "function")
        root.service.fail("Discord audio control timed out; check the current call state")
    }
  }
}
