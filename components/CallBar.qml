pragma ComponentBehavior: Bound
import QtQuick
import qs.Commons
import qs.Ui

import "../Api.js" as Api

// The call bar: one strip above the composer while a voice call is anything
// but idle (Service.voice, mirrored from protocol.State.voice, so it is back
// the moment the panel is re-summoned mid-call). Channel name, what the call
// is doing, and the three controls. It is a panel focus stop rather than a
// zone: everything it does has a chord (Ctrl+Shift+M / D / H) that works from
// anywhere, so there is no cursor to rove.
Item {
  id: root

  property var service: null
  property string channelName: ""
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
    if (failed) return String(voice.error || "") || "Voice failed"
    if (!connected) return ""
    var parts = []
    if (isMuted) parts.push("muted")
    if (isDeafened) parts.push("deafened")
    return parts.length ? "Connected · " + parts.join(" · ") : "Connected"
  }

  visible: inCall
  implicitHeight: Style.spacing.controlHeight + Style.spacing.sm * 2
  height: implicitHeight

  BorderSurface {
    anchors.fill: parent
    radius: Style.cornerRadius
    color: root.activeFocus ? Style.focusFillFor(root.foreground, root.accent)
      : Style.normalFillFor(root.foreground, root.accent)
    borderSpec: root.activeFocus
      ? Border.controlSpec("focus", root.foreground, root.accent)
      : Border.none()

    Text {
      id: glyph
      anchors.left: parent.left
      anchors.leftMargin: Style.spacing.rowPaddingX
      anchors.verticalCenter: parent.verticalCenter
      text: Api.channelGlyph("voice")
      color: root.failed ? Color.urgent : (root.connected ? root.accent : root.secondary)
      font.family: root.fontFamily
      font.pixelSize: Style.font.body
    }

    Text {
      id: name
      anchors.left: glyph.right
      anchors.leftMargin: Style.spacing.sm
      anchors.verticalCenter: parent.verticalCenter
      width: Math.min(implicitWidth, Math.max(0, controls.x - x - Style.spacing.sm))
      elide: Text.ElideRight
      text: root.channelName || "Voice"
      color: root.foreground
      font.family: root.fontFamily
      font.pixelSize: Style.font.body
      font.bold: true
    }

    Text {
      anchors.left: name.right
      anchors.leftMargin: Style.spacing.sm
      anchors.right: controls.left
      anchors.rightMargin: Style.spacing.sm
      anchors.verticalCenter: parent.verticalCenter
      elide: Text.ElideRight
      text: root.statusText
      color: root.failed ? Color.urgent : root.secondary
      font.family: root.fontFamily
      font.pixelSize: Style.font.bodySmall
    }

    // Mouse-only: the keyboard route is the three chords, which reach these
    // actions from every zone (and from any app over the voice IpcHandler),
    // so a fourth roving cursor would buy nothing.
    Row {
      id: controls
      anchors.right: parent.right
      anchors.rightMargin: Style.spacing.sm
      anchors.verticalCenter: parent.verticalCenter
      spacing: Style.spacing.controlGap

      Button {
        text: root.isMuted ? "Unmute" : "Mute"
        active: root.isMuted
        enabled: root.connected
        foreground: root.foreground
        fontFamily: root.fontFamily
        tooltipText: "Mute / unmute the microphone (Ctrl+Shift+M)"
        onClicked: if (root.service) root.service.toggleMute()
      }
      Button {
        text: root.isDeafened ? "Undeafen" : "Deafen"
        active: root.isDeafened
        enabled: root.connected
        foreground: root.foreground
        fontFamily: root.fontFamily
        tooltipText: "Stop / resume hearing the others (Ctrl+Shift+D)"
        onClicked: if (root.service) root.service.toggleDeafen()
      }
      Button {
        text: "Leave"
        foreground: root.foreground
        fontFamily: root.fontFamily
        tooltipText: "Leave the voice channel (Ctrl+Shift+H)"
        onClicked: if (root.service) root.service.voiceLeave()
      }
    }
  }
}
