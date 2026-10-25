import QtQuick
import "." as Theme

Text {
  id: root

  property color foreground: Theme.Color.foreground
  property string fontFamily: Style.font.family
  property real fontSize: Style.font.caption

  textFormat: Text.PlainText
  color: Qt.darker(foreground, 1.4)
  font.family: fontFamily
  font.pixelSize: fontSize
  font.bold: true

  topPadding: Math.ceil(fontSize * 0.15)
}
