import QtQuick
import "ui"
import "ui/Icons.js" as Icons

// Original outline mark, using the same stroke and palette as action icons.
Item {
  id: root
  property real iconSize: Style.font.icon
  property color color: Color.foreground
  readonly property int iconStatus: drawing.status
  implicitWidth: iconSize
  implicitHeight: iconSize
  width: implicitWidth
  height: implicitHeight
  Image {
    id: drawing
    anchors.fill: parent
    source: Icons.source("omacord", root.color)
    sourceSize.width: Math.ceil(root.width * 2)
    sourceSize.height: Math.ceil(root.height * 2)
    fillMode: Image.PreserveAspectFit
  }
}
