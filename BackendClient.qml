import QtQuick
import Quickshell
import Quickshell.Io

import "Api.js" as Api

Item {
  id: root

  visible: false
  width: 0
  height: 0

  property bool wanted: false
  readonly property var activeSocket: socketLoader.item
  readonly property bool connected: !!(activeSocket && activeSocket.connected)
  property string lifecycle: ""
  property var lastState: null
  property int nextId: 1
  property var pending: ({})
  property int reconnectAttempt: 0

  readonly property string socketPath: {
    var runtime = String(Quickshell.env("XDG_RUNTIME_DIR") || "")
    return runtime ? runtime + "/omarchy-discord/backend.sock" : ""
  }
  readonly property bool socketPathAvailable: socketPath !== ""
  readonly property string configurationError: socketPathAvailable ? ""
    : "XDG_RUNTIME_DIR is not set, so the Discord backend socket cannot be located"

  signal stateReceived(var state)
  signal eventReceived(string name, var message)
  signal configurationFailed(string reason)

  function resetPending(reason) {
    var waiters = pending
    pending = ({})
    var message = String(reason || "The Discord backend is unavailable")
    for (var id in waiters) {
      var callback = waiters[id]
      if (typeof callback === "function") callback(false, null, message)
    }
  }

  function sendCommand(name, fields, callback) {
    var socket = activeSocket
    if (!socket || !socket.connected) {
      if (typeof callback === "function")
        callback(false, null, "The Discord backend is not connected")
      return 0
    }
    var id = nextId++
    var payload = { v: 1, id: id, command: String(name || "") }
    Api.assign(payload, fields || {})
    var nextPending = ({})
    for (var existing in pending) nextPending[existing] = pending[existing]
    nextPending[String(id)] = typeof callback === "function" ? callback : null
    pending = nextPending
    socket.write(JSON.stringify(payload) + "\n")
    socket.flush()
    return id
  }

  function handleLine(line) {
    var message = Api.parseJson(line, null)
    if (!message || typeof message !== "object") return
    if (message.type === "event") {
      var name = String(message.event || "")
      if (name === "state_changed" && message.state) {
        lastState = message.state
        lifecycle = String(message.state.lifecycle || "")
        stateReceived(message.state)
      }
      eventReceived(name, message)
      return
    }
    if (message.type !== "response") return
    var id = String(message.id || "")
    var callback = pending[id]
    if (callback === undefined) return
    var nextPending = ({})
    for (var key in pending) if (key !== id) nextPending[key] = pending[key]
    pending = nextPending
    if (typeof callback !== "function") return
    if (message.ok === true) callback(true, message.result || ({}), "")
    else {
      var error = message.error || {}
      callback(false, null, Api.redact(String(error.message
        || error.code || "Discord command failed")))
    }
  }

  onWantedChanged: {
    if (wanted) {
      if (!socketPathAvailable) configurationFailed(configurationError)
      return
    }
    reconnectTimer.stop()
    resetPending("The Discord backend stopped")
    socketLoader.active = false
    lifecycle = ""
    lastState = null
    reconnectAttempt = 0
  }

  onConnectedChanged: {
    if (connected) reconnectAttempt = 0
    else resetPending("The Discord backend disconnected")
  }

  Component {
    id: socketComponent
    Socket {
      path: root.socketPath
      connected: true
      parser: SplitParser {
        splitMarker: "\n"
        onRead: function(line) { root.handleLine(line) }
      }
      onConnectionStateChanged: {
        if (connected) {
          var client = root
          var sock = this
          Qt.callLater(function() {
            try {
              if (client && sock && client.activeSocket === sock && sock.connected)
                client.sendCommand("hello", null, null)
            } catch (e) {}
          })
        } else root.lifecycle = ""
      }
    }
  }

  Loader {
    id: socketLoader
    active: false
    sourceComponent: socketComponent
  }

  Timer {
    id: reconnectTimer
    interval: Math.min(1500, 180 + root.reconnectAttempt * 120)
    repeat: true
    triggeredOnStart: true
    running: root.wanted && root.socketPathAvailable && !root.connected
    onTriggered: {
      root.reconnectAttempt = Math.min(12, root.reconnectAttempt + 1)
      socketLoader.active = false
      socketLoader.active = true
    }
  }
}
