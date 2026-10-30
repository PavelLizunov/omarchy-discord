pragma ComponentBehavior: Bound
import QtQuick
import "../ui" as PluginTheme
import "../ui"

Item {
  id: root
  property var service: null
  property var sharing: null
  property string channelName: ""
  property bool expanded: false
  property bool compactRosterHeader: true
  property bool showDiagnostics: true
  property bool rejoining: false
  property color foreground: PluginTheme.Color.foreground
  property color secondary: PluginTheme.Color.foreground
  property color accent: PluginTheme.Color.accent
  property string fontFamily: Style.font.family
  readonly property var voice: service && service.voice ? service.voice : ({})
  readonly property string status: String(voice.status || "idle")
  readonly property bool inCall: status !== "idle"
  readonly property bool connected: status === "connected"
  readonly property bool failed: status === "error"
  readonly property bool isMuted: !!voice.muted
  readonly property bool isDeafened: !!voice.deafened
  readonly property var roomUsers: service && voice.channelId
    ? service.voiceUsers(String(voice.guildId || ""), String(voice.channelId)) : []
  readonly property alias roster: voiceRoster
  readonly property string statusText: failed ? String(voice.error || "Voice disconnected")
    : rejoining ? "Reconnecting…" : status === "connecting" ? "Connecting…" : ""
  readonly property bool controlFocused: voiceRoster.activeFocus || voiceRoster.audioControls.controlFocused
    || diagnostics.testFocused || focusControls().some(function(button) { return button.activeFocus })
  signal chatRequested()
  signal videoRequested()

  function openChat() {
    if (!service || !voice.channelId) return
    service.showChannel(String(voice.channelId), String(voice.guildId || ""))
    chatRequested()
  }
  function rejoin() {
    if (rejoining || !service || !service.showStructure || !voice.channelId) return
    rejoining = !!service.voiceJoin(String(voice.guildId || ""), String(voice.channelId))
    if (rejoining) rejoinDeadline.restart()
  }
  function focusControls() {
    return [chatButton,peopleButton,diagnosticToggle,muteButton,deafenButton,shareButton,watchButton,reconnect,leaveButton]
      .filter(function(button) { return button.visible && button.enabled })
  }
  function focusAudio(delta) {
    var buttons = focusControls()
    if (voiceRoster.visible) buttons = buttons.concat(voiceRoster.audioUserId ? voiceRoster.audioControls.focusControls() : [voiceRoster])
    if (diagnostics.details) buttons = buttons.concat(diagnostics.focusControls())
    var current = buttons.findIndex(function(button) { return button.activeFocus })
    var next = current < 0 ? (delta < 0 ? buttons.length-1 : 0) : current+delta
    if (next < 0 || next >= buttons.length) return false
    buttons[next].forceActiveFocus()
    return true
  }
  function dismissAudio() {
    if (voiceRoster.audioUserId) { voiceRoster.closeAudio(); return true }
    if (diagnostics.details) { diagnostics.details=false; diagnosticToggle.forceActiveFocus(); return true }
    return false
  }
  onStatusChanged: if (status !== "connected") { rejoining=false; rejoinDeadline.stop() }
  Timer { id:rejoinDeadline; interval:10000; onTriggered:root.rejoining=false }
  visible: inCall
  implicitHeight: header.height + controls.height + (feedback.visible ? feedback.height + content.spacing : 0)
    + (expanded || diagnostics.details ? Style.space(120) + content.spacing : 0) + Style.spacing.sm*2 + content.spacing
  height: implicitHeight

  BorderSurface {
    anchors.fill:parent
    radius:Style.cornerRadius
    color:PluginTheme.Color.popups.background
    borderSpec:Border.flat(PluginTheme.Color.popups.border, Math.max(1,Style.normalBorderWidth))
    Column {
      id:content
      anchors.fill:parent; anchors.margins:Style.spacing.sm; spacing:Style.spacing.xs
      Item {
        id:header; width:parent.width; height:Style.spacing.controlHeight
        Button {
          id:chatButton; objectName:"voice-chat-button"
          anchors.left:parent.left; anchors.right:peopleButton.left
          anchors.rightMargin:Style.spacing.xs
          text:root.channelName || "Voice"; iconName:"headphones"; leftAlign:true
          focusable:true; tooltipText:"Open this room's chat"; onClicked:root.openChat()
        }
        Button {
          id:peopleButton; objectName:"voice-people-toggle"
          anchors.right:diagnosticToggle.visible ? diagnosticToggle.left : parent.right
          text:"People · " + root.roomUsers.length; iconName:"members"; selected:root.expanded && !diagnostics.details
          focusable:true; onClicked:{diagnostics.details=false;root.expanded=!root.expanded}
        }
        Button {
          id:diagnosticToggle; objectName:"voice-audio-details"; anchors.right:parent.right
          visible:root.showDiagnostics; iconOnly:true; iconName:"sliders"; text:"Audio settings"
          tooltipText:"Audio settings and diagnostics"; selected:diagnostics.details; focusable:true
          onClicked:diagnostics.details=!diagnostics.details
        }
      }
      Flow {
        id:controls; objectName:"voice-actions"; width:parent.width; spacing:Style.spacing.controlGap
        Button {
          id:muteButton; objectName:"voice-mute"; text:root.isMuted ? "Unmute" : "Mute"
          iconName:root.isMuted ? "microphoneOff" : "microphone"; active:root.isMuted
          focusable:true; enabled:root.connected; tooltipText:"Toggle microphone (Ctrl+Shift+M)"
          onClicked:if(root.service)root.service.toggleMute()
        }
        Button {
          id:deafenButton; objectName:"voice-deafen"; text:root.isDeafened ? "Hear" : "Deafen"
          iconName:root.isDeafened ? "headphonesOff" : "headphones"; active:root.isDeafened
          focusable:true; enabled:root.connected; tooltipText:"Toggle incoming audio (Ctrl+Shift+D)"
          onClicked:if(root.service)root.service.toggleDeafen()
        }
        Button {
          id:shareButton; objectName:"screen-share"; visible:!!root.sharing
          text:root.sharing && root.sharing.active ? "Stop sharing" : "Share screen"
          iconName:root.sharing && root.sharing.active ? "close" : "screen"
          selected:!!(root.sharing && root.sharing.active); focusable:true
          enabled:root.connected; tooltipText:"Choose a screen or window. System audio is not shared."
          onClicked:root.sharing.toggle()
        }
        Button {
          id:watchButton; objectName:"voice-watch"; visible:!!root.sharing
          text:"Watch"; iconName:"camera"; enabled:root.connected; focusable:true
          tooltipText:"Watch a participant's camera or screen"; onClicked:root.videoRequested()
        }
        Button {
          id:reconnect; objectName:"voice-reconnect"; visible:root.failed
          text:"Reconnect"; iconName:"reconnect"; enabled:!root.rejoining; focusable:true
          onClicked:root.rejoin()
        }
        Button {
          id:leaveButton; objectName:"voice-leave"; text:"Leave"; iconName:"leave"
          foreground:PluginTheme.Color.urgent; bordered:true; focusable:true
          tooltipText:"Leave the call and stop sharing (Ctrl+Shift+H)"
          onClicked:if(root.service)root.service.voiceLeave()
        }
      }
      Text {
        id:feedback; objectName:"voice-status"; width:parent.width
        visible:text!==""; text:root.sharing && root.sharing.message ? root.sharing.message : root.statusText
        textFormat:Text.PlainText; wrapMode:Text.WordWrap
        color:root.failed || (root.sharing && root.sharing.error) ? PluginTheme.Color.urgent : root.foreground
        font.family:root.fontFamily; font.pixelSize:Style.font.bodySmall
      }
      MemberList {
        id:voiceRoster; objectName:"voice-roster"; width:parent.width
        visible:root.expanded && !diagnostics.details
        height:visible ? Math.max(0,content.height-y) : 0
        service:root.service; voiceMode:true; compactHeader:root.compactRosterHeader
        voiceUsers:root.roomUsers; channelName:root.channelName
        headingText:root.failed ? "People in room" : "In voice"
        active:activeFocus || audioControls.controlFocused
      }
      VoiceDiagnostics {
        id:diagnostics; objectName:"voice-audio-diagnostics"; width:parent.width
        visible:root.showDiagnostics; height:details ? Math.max(0,content.height-y) : 0
        service:root.service; connected:root.connected
        callIdentity:String(root.voice.guildId || "")+":"+String(root.voice.channelId || "")
        muted:root.isMuted; deafened:root.isDeafened; rejoining:root.rejoining
        onReconnectRequested:root.rejoin()
      }
    }
  }
}
