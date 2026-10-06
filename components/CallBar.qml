pragma ComponentBehavior: Bound
import QtQuick
import "../ui"

import "../Api.js" as Api

Item {
  id: root

  property var service: null
  property string channelName: ""
  property bool expanded: false
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
    if (status === "connecting") return "Connecting…"
    if (failed) return "Disconnected · reconnect to rejoin"
    if (!connected) return ""
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

      Row {
        id: voiceTitle
        width: parent.width
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

      Text {
        id: callStatus
        objectName: "voice-status"
        width: parent.width
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
        height: visible ? Math.max(0, root.height - voiceTitle.height - callStatus.height - controls.height
          - (reconnect.visible ? reconnect.height + content.spacing : 0) - Style.spacing.sm * 2 - content.spacing * 3) : 0
        service: root.service
        voiceMode: true
        voiceUsers: root.roomUsers
        headingText: root.failed ? "People in room" : "In voice"
      }
      Button {
        id: reconnect
        objectName: "voice-reconnect"
        visible: root.failed
        width: parent.width
        text: "Reconnect"
        iconName: "reconnect"
        focusable: true
        enabled: !!(root.service && root.service.showStructure && root.voice.guildId && root.voice.channelId)
        tooltipText: String(root.voice.error || "Voice disconnected") + " · Rejoin this voice channel"
        onClicked: if (root.service) root.service.voiceJoin(String(root.voice.guildId), String(root.voice.channelId))
      }
      Row {
        id: controls
        width: parent.width
        topPadding: Style.spacing.xxs
        spacing: Style.spacing.controlGap
        readonly property real cell: Math.max(0, (width - spacing * 2) / 3)

        Button {
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
