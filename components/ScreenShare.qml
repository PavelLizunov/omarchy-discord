pragma ComponentBehavior: Bound
import QtQuick

Item {
  id: root
  visible: false
  property var service: null
  property var sample: ({status:"idle"})
  property bool busy: false
  property bool polling: false
  property double requestRevision: 0
  property int generation: 0
  property string error: ""
  readonly property string identity: service && service.voice
    ? String(service.voice.guildId || "") + ":" + String(service.voice.channelId || "") + ":" + String(service.voice.status || "idle") : ""
  readonly property bool connected: !!(service && service.voice && service.voice.status === "connected")
  readonly property string status: String(sample.status || "idle")
  readonly property bool active: ["selecting","connecting","waiting","sharing","stopping"].indexOf(status) >= 0
  readonly property string message: error || String(sample.error || "") || ({
    selecting:"Choose a screen or window in the system dialog",
    connecting:"Connecting screen share…", waiting:"Waiting for a viewer…",
    sharing:"Sharing screen · no system audio", stopping:"Stopping screen share…"
  })[status] || ""
  function revision() { requestRevision = Math.max(Date.now()*1000, requestRevision+1); return requestRevision }
  function toggle() { if (active || busy) stop(); else start() }
  function start() {
    if (!connected || busy || active || !service.backend) return
    error = ""; busy = true; sample = {status:"selecting"}
    var token = ++generation
    deadline.restart()
    service.backend.sendCommand("screen_share", {action:"start",height:720,fps:30,revision:revision()}, function(ok,result,reason) {
      if (token !== root.generation) return
      root.busy = false; deadline.stop()
      root.sample = ok ? result : {status:"error"}
      root.error = ok ? "" : String(reason || "Screen sharing failed")
    })
  }
  function stop() {
    if (!service || !service.backend) return
    error = ""; busy = false; sample = {status:"stopping"}
    var token = ++generation
    deadline.restart()
    service.backend.sendCommand("screen_share", {action:"stop",revision:revision()}, function(ok,result,reason) {
      if (token !== root.generation) return
      deadline.stop()
      if (ok) root.sample = result
      else root.error = String(reason || "Could not stop sharing. Leave the call to stop capture.")
    })
  }
  function poll() {
    if (!connected || busy || polling || !service.backend) return
    polling = true
    var token = generation
    service.backend.sendCommand("screen_share", {action:"status"}, function(ok,result) {
      root.polling = false
      if (token === root.generation && ok) root.sample = result
    })
  }
  onIdentityChanged: {
    generation++; busy = false; polling = false; deadline.stop()
    sample = {status:"idle"}; error = ""
    if (connected) poll()
  }
  Component.onCompleted: poll()
  Component.onDestruction: if (active || busy) stop()
  Timer { interval:1000; repeat:true; running:root.connected; onTriggered:root.poll() }
  Timer {
    id:deadline; interval:7000
    onTriggered: {
      root.generation++; root.busy = false
      root.error = "Screen request timed out. Stop sharing or leave the call."
    }
  }
}
