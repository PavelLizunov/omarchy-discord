import QtQuick
import Quickshell
import Quickshell.Io

Item {
  id: root
  property string pluginDir: ""
  readonly property bool running: helper.running
  signal failed(string reason)

  function start() {
    if (helper.running || !pluginDir) return false
    helper.running = true
    return true
  }

  Process {
    id: helper
    command: ["/usr/bin/bash", root.pluginDir + "/scripts/login.sh"]
    onExited: function(code) {
      if (code === 3) root.failed(qsTr("A Discord sign-in window is already open."))
      else if (code > 1) root.failed(qsTr("Could not open Discord sign-in. Run scripts/build-login.sh from the plugin checkout."))
    }
  }
}
