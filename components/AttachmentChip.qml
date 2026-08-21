import QtQuick
import qs.Commons
import qs.Ui

import "../Api.js" as Api

import "../Markdown.js" as Markdown

// One staged attachment above the composer: thumbnail, filename + size, an
// upload progress bar (driven by upload_progress through Service.staged) and
// a remove button. Keyboard reach is the composer's chip cursor (`cursor`);
// the mouse gets the same affordance through the ✕ button.
Item {
  id: root

  // { path, filename, size, sent, total, uploading } from Service.stagedFor().
  property var item: ({})
  property bool cursor: false

  signal remove()
  signal clicked()

  readonly property color foreground: Color.foreground
  readonly property color muted: Api.secondaryColor(Color.muted, Color.foreground, Color.background)
  readonly property color accent: Color.accent
  readonly property string fontFamily: Style.font.family
  readonly property string path: String(item && item.path || "")
  readonly property string filename: String(item && item.filename || "")
  readonly property bool uploading: !!(item && item.uploading)
  readonly property real progress: {
    var total = Number(item && item.total) || 0
    var sent = Number(item && item.sent) || 0
    return total > 0 ? Math.max(0, Math.min(1, sent / total)) : 0
  }
  readonly property int thumbSize: Style.space(96)

  implicitWidth: thumbSize + Style.spacing.sm * 2
  implicitHeight: thumbSize + caption.implicitHeight + Style.spacing.xs + Style.spacing.sm * 2

  BorderSurface {
    anchors.fill: parent
    radius: Style.cornerRadius
    color: root.cursor || mouse.containsMouse
      ? Style.hoverFillFor(root.foreground, root.accent)
      : Style.normalFillFor(root.foreground, root.accent)
    borderSpec: root.cursor
      ? Border.controlSpec("hover-cursor", root.foreground, root.accent)
      : Border.flat(Color.popups.border, Math.max(1, Style.normalBorderWidth))
  }

  MouseArea {
    id: mouse
    anchors.fill: parent
    hoverEnabled: true
    onClicked: root.clicked()
  }

  Image {
    id: thumb
    anchors.top: parent.top
    anchors.horizontalCenter: parent.horizontalCenter
    anchors.topMargin: Style.spacing.sm
    width: root.thumbSize
    height: root.thumbSize
    source: root.path ? "file://" + root.path : ""
    fillMode: Image.PreserveAspectFit
    asynchronous: true
    sourceSize.width: root.thumbSize * 2
    sourceSize.height: root.thumbSize * 2
    opacity: root.uploading ? 0.6 : 1
  }

  Text {
    anchors.centerIn: thumb
    visible: thumb.status === Image.Error
    text: "image"
    color: root.muted
    font.family: root.fontFamily
    font.pixelSize: Style.font.caption
  }

  // Progress bar along the bottom edge of the thumbnail.
  Rectangle {
    anchors.left: thumb.left
    anchors.right: thumb.right
    anchors.bottom: thumb.bottom
    height: Style.spacing.xs
    radius: height / 2
    visible: root.uploading
    color: Util.alpha(root.foreground, 0.15)

    Rectangle {
      anchors.left: parent.left
      anchors.top: parent.top
      anchors.bottom: parent.bottom
      width: parent.width * root.progress
      radius: parent.radius
      color: root.accent
    }
  }

  Text {
    id: caption
    anchors.top: thumb.bottom
    anchors.left: parent.left
    anchors.right: parent.right
    anchors.topMargin: Style.spacing.xs
    anchors.leftMargin: Style.spacing.sm
    anchors.rightMargin: Style.spacing.sm
    horizontalAlignment: Text.AlignHCenter
    elide: Text.ElideMiddle
    text: root.filename + " · " + (root.uploading
      ? Math.round(root.progress * 100) + "%"
      : Markdown.formatSize(root.item ? root.item.size : 0))
    color: root.muted
    font.family: root.fontFamily
    font.pixelSize: Style.font.caption
  }

  // Remove button (mouse); keyboard uses x / Delete on the focused chip.
  Rectangle {
    anchors.top: parent.top
    anchors.right: parent.right
    anchors.margins: Style.spacing.xs
    width: Style.space(18)
    height: width
    radius: width / 2
    visible: !root.uploading && (root.cursor || mouse.containsMouse || removeMouse.containsMouse)
    color: removeMouse.containsMouse ? Color.urgent : Util.alpha(Color.popups.background, 0.85)

    Text {
      anchors.centerIn: parent
      text: "✕"
      color: removeMouse.containsMouse ? Color.popups.background : root.foreground
      font.family: root.fontFamily
      font.pixelSize: Style.font.caption
      font.bold: true
    }
    MouseArea {
      id: removeMouse
      anchors.fill: parent
      hoverEnabled: true
      onClicked: root.remove()
    }
  }
}
