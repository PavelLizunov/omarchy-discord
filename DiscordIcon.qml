import QtQuick
import "ui" as PluginTheme
import "ui"
import "ui/Icons.js" as Icons

// Selected solid stepped mark, preserving the native optical canvas.
Item {
  id: root
  property real iconSize: Style.font.icon
  property color color: PluginTheme.Color.foreground
  readonly property int iconStatus: drawing.status
  implicitWidth: iconSize
  implicitHeight: iconSize
  width: implicitWidth
  height: implicitHeight
  Image {
    id: drawing
    anchors.fill: parent
    source: Icons.source("omacord", root.color, sourceSize.width)
    sourceSize.width: Math.max(1, Math.ceil(root.width * Screen.devicePixelRatio))
    sourceSize.height: Math.max(1, Math.ceil(root.height * Screen.devicePixelRatio))
    fillMode: Image.PreserveAspectFit
  }
}
