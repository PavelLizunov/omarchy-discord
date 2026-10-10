pragma ComponentBehavior: Bound
import QtQuick
import "../ui"

import "../Api.js" as Api

Item {
  id: root

  property bool rejoining: false
  function rejoin() {
    if (rejoining || !service || !service.showStructure || !voice.channelId) return
    rejoining = true
    if (!service.voiceJoin(String(voice.guildId || ""), String(voice.channelId))) rejoining = false
    else rejoinDeadline.restart()
  }
  onStatusChanged: if (status === "connecting" || status === "error" || status === "idle") { rejoining = false; rejoinDeadline.stop() }
  Timer { id: rejoinDeadline; interval: 10000; onTriggered: root.rejoining = false }
  readonly property bool controlFocused: diagnosticToggle.activeFocus || diagnostics.testFocused || voiceRoster.activeFocus || voiceRoster.audioControls.controlFocused || muteButton.activeFocus || deafenButton.activeFocus || leaveButton.activeFocus || reconnect.activeFocus
  function focusAudio(delta) {
    var buttons = voiceRoster.audioUserId ? voiceRoster.audioControls.focusControls() : [voiceRoster]
    buttons.push(diagnosticToggle)
    if (diagnostics.details) buttons = buttons.concat(diagnostics.focusControls())
    if (reconnect.visible) buttons.push(reconnect)
    buttons = buttons.concat([muteButton,deafenButton,leaveButton])
    var current = buttons.findIndex(function(button) { return button.activeFocus })
    var next = current < 0 ? (delta < 0 ? buttons.length - 1 : buttons.indexOf(diagnosticToggle)) : current + delta
    if (next < 0 || next >= buttons.length) return false
    if (diagnostics.focusControls().indexOf(buttons[next]) >= 0) diagnostics.focusControl(buttons[next])
    else buttons[next].forceActiveFocus()
    return true
  }
  function dismissAudio(){if(voiceRoster.audioUserId){voiceRoster.closeAudio();return true}if(!diagnostics.details)return false;diagnostics.details=false;diagnosticToggle.forceActiveFocus();return true}
  signal chatRequested()
  function openChat() {
    if (!service || !voice.channelId) return
    service.showChannel(String(voice.channelId), String(voice.guildId || ""))
    chatRequested()
  }

  property var service: null
  property string channelName: ""
  property bool expanded: false
  property bool compactRosterHeader: false
  readonly property var roomUsers: service && voice.channelId
    ? service.voiceUsers(String(voice.guildId || ""), String(voice.channelId)) : []
  readonly property alias roster: voiceRoster
  property color foreground: Color.foreground
  property color secondary: Color.foreground
  property color accent: Color.accent
  property string fontFamily: Style.font.family

  readonly property var voice: service && service.voice ? service.voice : ({})
  readonly property string status: String(voice.status || "idle")
  readonly property bool inCall: status !== "idle"
  readonly property bool connected: status === "connected"
  readonly property bool failed: status === "error"
  readonly property bool isMuted: !!voice.muted
  readonly property bool isDeafened: !!voice.deafened
  readonly property string statusText: {
    if (rejoining) return "Reconnecting…"
    if (status === "connecting") return "Connecting…"
    if (failed) return "Disconnected · reconnect to rejoin"
    if (!connected) return ""
    if (diagnostics.fresh && !diagnostics.sample.encryption_ready) return "Securing audio…"
    var parts = []
    if (isMuted) parts.push("muted")
    if (isDeafened) parts.push("deafened")
    return parts.length ? "Connected · " + parts.join(" · ") : "Connected"
  }

  visible: inCall
  implicitHeight: expanded ? Style.space(320) : content.implicitHeight + Style.spacing.sm * 2
  height: implicitHeight

  BorderSurface {
    anchors.fill: parent
    radius: Style.cornerRadius
    color: root.activeFocus ? Style.focusFillFor(root.foreground, root.accent)
      : Style.normalFillFor(root.foreground, root.accent)
    borderSpec: root.activeFocus
      ? Border.controlSpec("focus", root.foreground, root.accent)
      : Border.none()

    Column {
      id: content
      anchors.left: parent.left
      anchors.right: parent.right
      anchors.top: parent.top
      anchors.topMargin: Style.spacing.sm
      anchors.leftMargin: Style.spacing.sm
      anchors.rightMargin: Style.spacing.sm
      spacing: Style.spacing.xxs

      Item {
        id: voiceTitleContainer
        width: parent.width
        height: Math.max(voiceTitle.height, chatButton.height)

        Row {
          id: voiceTitle
          anchors.left: parent.left
          anchors.right: chatButton.left
          anchors.rightMargin: Style.spacing.xs
          anchors.verticalCenter: parent.verticalCenter
          spacing: Style.spacing.sm

          Text {
            id: glyph
            anchors.verticalCenter: parent.verticalCenter
            text: Api.channelGlyph("voice")
            color: root.failed ? Color.urgent : (root.connected ? root.accent : root.secondary)
            font.family: root.fontFamily
            font.pixelSize: Style.font.body
          }

          Text {
            anchors.verticalCenter: parent.verticalCenter
            width: Math.max(0, parent.width - glyph.width - parent.spacing)
            elide: Text.ElideRight
            text: root.channelName || "Voice"
            color: root.foreground
            font.family: root.fontFamily
            font.pixelSize: Style.font.body
            font.bold: true
          }
        }

        MouseArea {
          anchors.fill: voiceTitle
          cursorShape: Qt.PointingHandCursor
          onClicked: root.openChat()
        }

        Button {
          id: chatButton
          objectName: "voice-chat-button"
          anchors.right: parent.right
          anchors.verticalCenter: parent.verticalCenter
          iconOnly: true
          iconName: "reply"
          text: "Chat"
          tooltipText: "Open text chat for this voice room"
          focusable: true
          onClicked: root.openChat()
        }
      }

      Text {
        id: callStatus
        objectName: "voice-status"
        width: parent.width
        visible: !root.connected || root.expanded
        wrapMode: Text.WordWrap
        text: root.statusText
        color: root.failed ? Color.urgent : root.secondary
        font.family: root.fontFamily
        font.pixelSize: Style.font.bodySmall
      }

      MemberList {
        id: voiceRoster
        objectName: "voice-roster"
        visible: root.expanded
        width: parent.width
        height: visible && !diagnostics.details ? Math.max(0, root.height - voiceTitleContainer.height - (callStatus.visible ? callStatus.height : 0) - controls.height - (diagnosticToggle.visible ? diagnosticToggle.height : 0)
          - (reconnect.visible ? reconnect.height + content.spacing : 0) - Style.spacing.sm * 2 - content.spacing * 3) : 0
        service: root.service
        voiceMode: true
        active: activeFocus || audioControls.controlFocused
        compactHeader: root.compactRosterHeader
        voiceUsers: root.roomUsers
        channelName: root.channelName
        headingText: root.failed ? "People in room" : "In voice"
      }
      Button {
        id: diagnosticToggle
        objectName: "voice-audio-details"
        width: parent.width
        visible: root.connected
        text: diagnostics.details ? "Show people" : "Audio details"
        leftAlign: true
        foreground: root.foreground
        fontFamily: root.fontFamily
        implicitHeight: Style.space(32)
        focusable: true
        onClicked: diagnostics.details = !diagnostics.details
      }
      VoiceDiagnostics {
        id: diagnostics
        objectName: "voice-audio-diagnostics"
        width: parent.width
        service: root.service
        connected: root.connected
        callIdentity: String(root.voice.guildId || "") + ":" + String(root.voice.channelId || "")
        muted: root.isMuted
        deafened: root.isDeafened
        foreground: root.foreground
        fontFamily: root.fontFamily
        rejoining: root.rejoining
        onReconnectRequested: root.rejoin()
        height: !details ? 0 : !root.expanded ? Math.min(implicitHeight, Style.space(220))
          : Math.max(0, root.height - voiceTitleContainer.height - (callStatus.visible ? callStatus.height + content.spacing : 0) - controls.height - diagnosticToggle.height - Style.spacing.sm * 2 - content.spacing * 4)
      }
      Button {
        id: reconnect
        objectName: "voice-reconnect"
        visible: root.failed
        width: parent.width
        text: "Reconnect"
        iconName: "reconnect"
        focusable: true
        enabled: !root.rejoining && !!(root.service && root.service.showStructure && root.voice.channelId)
        tooltipText: String(root.voice.error || "Voice disconnected") + " · Rejoin this voice channel"
        foreground: root.foreground
        fontFamily: root.fontFamily
        onClicked: root.rejoin()
      }
      Row {
        id: controls
        width: parent.width
        topPadding: Style.spacing.xxs
        spacing: Style.spacing.controlGap
        readonly property real cell: Math.max(0, (width - spacing * 2) / 3)

        Button {
          id: muteButton
          objectName: "voice-mute"
          width: controls.cell
          iconOnly: true
          text: root.isMuted ? "Unmute" : "Mute"
          iconName: "microphone"
          focusable: true
          active: root.isMuted
          enabled: root.connected
          foreground: root.foreground
          fontFamily: root.fontFamily
          tooltipText: (root.isMuted ? "Unmute" : "Mute") + " the microphone (Ctrl+Shift+M)"
          onClicked: if (root.service) root.service.toggleMute()
        }
        Button {
          id: deafenButton
          objectName: "voice-deafen"
          width: controls.cell
          iconOnly: true
          text: root.isDeafened ? "Hear" : "Deafen"
          iconName: "headphones"
          focusable: true
          active: root.isDeafened
          enabled: root.connected
          foreground: root.foreground
          fontFamily: root.fontFamily
          tooltipText: (root.isDeafened ? "Resume" : "Stop") + " hearing the others (Ctrl+Shift+D)"
          onClicked: if (root.service) root.service.toggleDeafen()
        }
        Button {
          id: leaveButton
          objectName: "voice-leave"
          width: controls.cell
          iconOnly: true
          text: "Leave"
          iconName: "leave"
          focusable: true
          foreground: Color.urgent
          fontFamily: root.fontFamily
          tooltipText: "Leave the voice channel (Ctrl+Shift+H)"
          onClicked: if (root.service) root.service.voiceLeave()
        }
      }
    }
  }
}
