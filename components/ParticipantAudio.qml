import QtQuick
import QtQuick.Controls as Controls
import "../ui"

Column {
  id: root
  property var service: null
  property string userId: ""
  property string userName: ""
  property string callIdentity: ""
  property bool connected: false
  property bool busy: false
  property int generation: 0
  property int volume: 100
  property bool muted: false
  property string errorText: ""
  readonly property bool controlFocused: mute.activeFocus || level.activeFocus || close.activeFocus
  signal closed()
  signal focusRequested(var item)
  spacing: Style.spacing.xxs
  visible: userId !== ""
  function request(command, fields) {
    if (busy || !connected || !service || !service.backend) return
    var gen = ++generation, id = userId
    busy = true; errorText = ""; deadline.restart()
    service.backend.sendCommand(command, fields, function(ok, result, error) {
      if (gen !== root.generation || id !== root.userId) return
      root.busy = false; deadline.stop()
      if (!ok) { root.errorText = String(error || "Participant audio unavailable"); return }
      var row = result && result[id] ? result[id] : {volume:100,muted:false}
      root.volume = row.volume; root.muted = !!row.muted
    })
  }
  function reload() {
    generation++; busy = false; deadline.stop(); errorText = ""; volume = 100; muted = false
    if (userId && connected) request("voice_user_audio", {})
  }
  function setAudio(fields) {
    fields.user_id = userId
    request("voice_user_set", fields)
  }
  function focusControls() { return [mute, level, close] }
  onUserIdChanged: reload()
  onCallIdentityChanged: { if (userId) closed() }
  onConnectedChanged: { if (!connected && userId) closed() }
  Component.onDestruction: generation++
  Timer {
    id: deadline; interval: 5000
    onTriggered: {root.generation++;root.busy=false;root.errorText="Audio settings timed out"}
  }
  Text {
    width: parent.width; text: root.userName; textFormat: Text.PlainText
    elide: Text.ElideRight; color: Color.foreground
    font.family: Style.font.family; font.pixelSize: Style.font.bodySmall
  }
  Button {
    id: mute; objectName: "participant-mute"; width: parent.width
    text: root.muted ? "Unmute locally" : "Mute locally"
    tooltipText: "Change only how you hear this participant"
    focusable: true; active: root.muted; enabled: root.connected && !root.busy
    onActiveFocusChanged: if(activeFocus)root.focusRequested(mute)
    onClicked: root.setAudio({muted:!root.muted})
  }
  Text {
    width: parent.width; text: "Volume · " + Math.round(level.value) + "%"
    color: Color.foreground; font.family: Style.font.family; font.pixelSize: Style.font.bodySmall
  }
  Controls.Slider {
    id: level; objectName: "participant-volume"; width: parent.width
    implicitHeight: Style.space(32)
    from: 0; to: 200; stepSize: 5; value: root.volume
    enabled: root.connected && !root.busy
    Accessible.name: "Participant playback volume"
    onActiveFocusChanged: if(activeFocus)root.focusRequested(level)
    onMoved: if (!pressed) root.setAudio({volume:Math.round(value)})
    onPressedChanged: if (!pressed && enabled) root.setAudio({volume:Math.round(value)})
    background: Rectangle {
      x: level.leftPadding; y: level.topPadding + (level.availableHeight-height)/2
      width: level.availableWidth; height: Style.space(4); color: Color.muted
      Rectangle {width: level.visualPosition*parent.width;height: parent.height;color: Color.accent}
    }
    handle: BorderSurface {
      x: level.leftPadding+level.visualPosition*(level.availableWidth-width)
      y: level.topPadding+(level.availableHeight-height)/2
      width: Style.space(16); height: width; radius: width/2; color: Color.foreground
      borderSpec: level.activeFocus ? Border.controlSpec("focus",Color.foreground,Color.accent) : Border.none()
    }
  }
  Text {
    width: parent.width; visible: root.errorText !== ""; text: root.errorText
    textFormat: Text.PlainText; wrapMode: Text.WordWrap; maximumLineCount: 2; elide: Text.ElideRight
    color: Color.urgent; font.family: Style.font.family; font.pixelSize: Style.font.bodySmall
  }
  Button {id:close;objectName:"participant-audio-close";width:parent.width;text:"Back to people";focusable:true;onActiveFocusChanged:if(activeFocus)root.focusRequested(close);onClicked:root.closed()}
  Keys.onEscapePressed: function(event) {root.closed();event.accepted=true}
}
