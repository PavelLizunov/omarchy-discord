pragma ComponentBehavior: Bound
import QtQuick
import "../ui"

import "../Api.js" as Api

// The call bar: the bottom strip of the channel column while a voice call is
// anything but idle (Service.voice, mirrored from protocol.State.voice, so it
// is back the moment the panel is re-summoned mid-call). Channel name, what
// the call is doing, and the three controls — stacked, because the column is
// ~230 px wide and "Undeafen" next to two more labels does not fit; the
// controls are icon-only with the chord in their tooltip. It is a panel focus
// stop rather than a zone: everything it does has a chord (Ctrl+Shift+M / D /
// H) that works from anywhere, so there is no cursor to rove.
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
  implicitHeight: content.implicitHeight + Style.spacing.sm * 2
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
      anchors.verticalCenter: parent.verticalCenter
      anchors.leftMargin: Style.spacing.sm
      anchors.rightMargin: Style.spacing.sm
      spacing: Style.spacing.xxs

      Row {
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
        width: parent.width
        elide: Text.ElideRight
        text: root.statusText
        color: root.failed ? Color.urgent : root.secondary
        font.family: root.fontFamily
        font.pixelSize: Style.font.bodySmall
      }

      // Mouse-only: the keyboard route is the three chords, which reach these
      // actions from every zone (and from any app over the voice IpcHandler),
      // so a fourth roving cursor would buy nothing. Icon-only at this width;
      // the tooltip carries the name and the chord.
      Row {
        id: controls
        width: parent.width
        topPadding: Style.spacing.xxs
        spacing: Style.spacing.controlGap
        readonly property real cell: Math.max(0, (width - spacing * 2) / 3)

        Button {
          width: controls.cell
          text: root.isMuted ? "Unmute" : "Mute"
          focusable: true
          active: root.isMuted
          enabled: root.connected
          foreground: root.foreground
          fontFamily: root.fontFamily
          tooltipText: (root.isMuted ? "Unmute" : "Mute") + " the microphone (Ctrl+Shift+M)"
          onClicked: if (root.service) root.service.toggleMute()
        }
        Button {
          width: controls.cell
          text: root.isDeafened ? "Hear" : "Deafen"
          focusable: true
          active: root.isDeafened
          enabled: root.connected
          foreground: root.foreground
          fontFamily: root.fontFamily
          tooltipText: (root.isDeafened ? "Resume" : "Stop") + " hearing the others (Ctrl+Shift+D)"
          onClicked: if (root.service) root.service.toggleDeafen()
        }
        Button {
          width: controls.cell
          text: "Leave"
          focusable: true
          // The one destructive control of the three, in the urgent token.
          foreground: Color.urgent
          fontFamily: root.fontFamily
          tooltipText: "Leave the voice channel (Ctrl+Shift+H)"
          onClicked: if (root.service) root.service.voiceLeave()
        }
      }
    }
  }
}
