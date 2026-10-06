import QtQuick
import "../ui"
import "../Markdown.js" as Markdown

Item {
  id: root
  property var item: ({})
  property bool cursor: false
  signal remove()
  signal clicked()
  readonly property bool uploading: !!item.uploading
  readonly property real progress: item.total > 0 ? Math.min(1, item.sent / item.total) : 0
  implicitWidth: Style.space(180)
  implicitHeight: label.implicitHeight + Style.spacing.sm * 2
  BorderSurface {
    anchors.fill: parent
    color: root.cursor ? Style.hoverFillFor(Color.foreground, Color.accent) : "transparent"
    radius: Style.cornerRadius
    borderSpec: Border.flat(Color.popups.border, 1)
  }
  MouseArea { anchors.fill: parent; onClicked: root.clicked() }
  Text {
    id: label
    anchors.left: parent.left
    anchors.right: removeButton.left
    anchors.verticalCenter: parent.verticalCenter
    anchors.margins: Style.spacing.sm
    textFormat: Text.PlainText
    text: String(root.item.filename || "attachment") + " · "
      + (root.uploading ? Math.round(root.progress * 100) + "%" : Markdown.formatSize(root.item.size))
    elide: Text.ElideMiddle
    color: Color.foreground
    font.family: Style.font.family
    font.pixelSize: Style.font.caption
  }
  Button {
    id: removeButton
    anchors.right: parent.right
    anchors.verticalCenter: parent.verticalCenter
    text: "x"
    focusable: true
    enabled: !root.uploading
    tooltipText: "Remove attachment"
    horizontalPadding: Style.spacing.sm
    verticalPadding: Style.spacing.xxs
    onClicked: root.remove()
  }
}
