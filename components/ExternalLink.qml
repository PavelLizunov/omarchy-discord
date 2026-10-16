import QtQuick
import Quickshell.Io

Item {
  id: root
  visible: false
  property var service: null
  readonly property bool busy: opener.running
  property string status: "idle"
  property int exitCode: -1
  signal finished(bool ok)

  function open(url) {
    var link=String(url || "")
    if (!/^https?:\/\/[^\s]+$/i.test(link)) {
      if(service)service.fail("Only HTTP and HTTPS links can be opened")
      return false
    }
    if(busy) {if(service)service.fail("A link is already opening");return false}
    status="opening";exitCode=-1
    opener.command=["/usr/bin/xdg-open",link]
    opener.running=true
    return true
  }
  Process {
    id: opener
    onStarted: deadline.start()
    onExited: function(code) {
      deadline.stop()
      root.exitCode=code
      var ok=code===0 && root.status!=="timeout"
      root.status=ok?"handed-off":"failed"
      if(!ok && root.service)root.service.fail("Could not open the link in your browser (exit " + code + ")")
      root.finished(ok)
    }
  }
  Timer {
    id: deadline
    interval: 10000
    onTriggered: {root.status="timeout";opener.signal(15)}
  }
}
