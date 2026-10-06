import QtQuick
import "../ui"

import "../Api.js" as Api

Item {
  id: root

  property var service: null
  property string url: ""
  property string name: ""
  property color foreground: Color.foreground
  property string fontFamily: Style.font.family

  readonly property string path: ""

  Rectangle {
    anchors.fill: parent
    radius: width / 2
    color: Util.alpha(root.foreground, 0.12)

    Text {
      anchors.centerIn: parent
      text: Api.initials(root.name).charAt(0)
      color: root.foreground
      font.family: root.fontFamily
      font.pixelSize: Style.font.caption
      font.bold: true
    }
  }
}
