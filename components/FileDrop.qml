import QtQuick
import "../ui" as Theme
import "../ui"

DropArea {
  id: root
  property var service: null
  property string channelId: ""
  property bool available: false
  property var stager: null
  readonly property bool busy: !!stager && stager.busy
  property string enteredChannel: ""
  objectName: "chat-file-drop"
  enabled: available && !!stager && !busy

  function localFiles(urls) {
    if (!urls || !urls.length || urls.length > 10) return false
    for (var i=0;i<urls.length;i++)
      if (!/^file:\/\/(?:localhost)?\//.test(String(urls[i]))) return false
    return true
  }
  onEntered: function(drag) {
    drag.accepted = root.localFiles(drag.urls)
    enteredChannel = drag.accepted ? channelId : ""
  }
  onExited: enteredChannel = ""
  onDropped: function(drop) {
    if (!root.service || !root.channelId || root.enteredChannel !== root.channelId
        || !root.localFiles(drop.urls)) { drop.accepted = false; return }
    var existing=root.service.staged[root.channelId] || []
    if (existing.length + drop.urls.length > 10) {
      root.service.fail("At most 10 files per message."); drop.accepted=false; return
    }
    if (!root.stager.stage(root.channelId, Array.from(drop.urls).map(function(url){return String(url)}))) {
      drop.accepted=false; return
    }
    drop.accept(Qt.CopyAction)
    enteredChannel=""
  }
  Rectangle {
    anchors.fill: parent
    visible: root.containsDrag
    color: Theme.Color.popups.background
    opacity: 0.94
    border.color: Theme.Color.accent
    border.width: Math.max(1,Style.normalBorderWidth)
    radius: Style.cornerRadius
    Text {
      anchors.centerIn: parent
      text: "Drop files to attach"
      textFormat: Text.PlainText
      color: Theme.Color.foreground
      font.family: Style.font.family
      font.pixelSize: Style.font.title
    }
  }
}
