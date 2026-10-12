import QtQuick
import "." as PluginTheme
import QtQuick.Controls

ToolTip {
  id: root

  property color panelForeground: PluginTheme.Color.tooltip.text
  property color panelBackground: PluginTheme.Color.tooltip.background
  property color panelBorder: PluginTheme.Color.tooltip.border
  property string fontFamily: Style.font.family
  property real fontSize: Style.font.bodySmall

  readonly property var panelBorderSpec: Border.localOrSurfaceSpec("tooltip", "border", panelBorder, PluginTheme.Color.tooltip.border, Style.normalBorderWidth)

  delay: 400
  padding: 0

  background: BorderSurface {
    color: root.panelBackground
    borderSpec: root.panelBorderSpec
    radius: Style.cornerRadius
  }

  contentItem: Text {
    textFormat: Text.PlainText
    text: root.text
    color: root.panelForeground
    font.family: root.fontFamily
    font.pixelSize: root.fontSize
    leftPadding: Border.left(root.panelBorderSpec) + Style.spacing.controlPaddingX
    rightPadding: Border.right(root.panelBorderSpec) + Style.spacing.controlPaddingX
    topPadding: Border.top(root.panelBorderSpec) + Style.spacing.controlPaddingY
    bottomPadding: Border.bottom(root.panelBorderSpec) + Style.spacing.controlPaddingY
  }
}
