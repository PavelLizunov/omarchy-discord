import QtQuick
import QtQuick.Controls as Controls
import "Icons.js" as Icons
import "../Api.js" as Api

Controls.AbstractButton {
  id: root
  property string iconText: ""
  property string iconName: ""
  property bool iconOnly: false
  property string tooltipText: ""
  property bool selected: false
  property bool active: false
  property bool hasCursor: false
  property bool focusable: false
  property bool bordered: false
  property bool leftAlign: false
  property color foreground: Color.foreground
  property color backgroundColor: "transparent"
  property color accent: Color.accent
  property string fontFamily: Style.font.family
  property real fontSize: Style.font.body
  property real iconSize: Style.space(16)
  property real iconRotation: 0
  property bool iconSpinning: false
  horizontalPadding: Style.spacing.controlPaddingX
  verticalPadding: Style.spacing.controlPaddingY
  property color tooltipBackground: Color.tooltip.background
  property color tooltipForeground: Color.tooltip.text
  property color tooltipBorder: Color.tooltip.border
  readonly property alias iconStatus: svgIcon.status
  readonly property alias tooltipItem: buttonToolTip
  readonly property bool hot: hovered || hasCursor
  readonly property bool _showFocusRing: focusable && activeFocus
  readonly property color _selectedColor: Style.selectedStateColor(foreground, accent)
  readonly property color paintedBackground: Api.blend(root.down ? Style.pressedFillFor(foreground, accent)
    : _showFocusRing ? Style.focusFillFor(foreground, accent)
    : selected || active ? Style.selectedFillFor(foreground, accent)
    : hot ? Style.hoverFillFor(foreground, accent) : backgroundColor, Color.popups.background,
    root.down ? Style.pressedFillAlpha : _showFocusRing ? Style.focusFillAlpha
      : selected || active ? Style.selectedFillAlpha : hot ? Style.hoverFillAlpha : backgroundColor.a)
  readonly property color textForeground: Api.textColor(selected || active ? _selectedColor : foreground, Color.foreground, paintedBackground)
  readonly property var _borderSpec: Border.controlSpec(_showFocusRing ? "focus" : hot ? "hover-cursor" : selected || active ? "selected" : "normal", foreground, accent)
  signal rightClicked()

  focusPolicy: focusable ? Qt.StrongFocus : Qt.NoFocus
  activeFocusOnTab: focusable
  hoverEnabled: true
  Accessible.role: Accessible.Button
  Accessible.name: tooltipText || text
  leftPadding: horizontalPadding + Border.left(_borderSpec)
  rightPadding: horizontalPadding + Border.right(_borderSpec)
  topPadding: verticalPadding + Border.top(_borderSpec)
  bottomPadding: verticalPadding + Border.bottom(_borderSpec)
  implicitWidth: Math.max(Style.space(32), (iconName !== "" || iconText !== "" ? iconSize : 0)
    + (!iconOnly && text !== "" ? labelMetrics.advanceWidth : 0)
    + (!iconOnly && text !== "" && (iconName !== "" || iconText !== "") ? Style.spacing.controlGap : 0)
    + leftPadding + rightPadding)
  implicitHeight: Math.max(Style.space(32), content.implicitHeight + topPadding + bottomPadding)
  opacity: enabled ? 1 : 0.45
  background: BorderSurface {
    radius: Style.cornerRadius
    color: root.down ? Style.pressedFillFor(root.foreground, root.accent)
      : root._showFocusRing ? Style.focusFillFor(root.foreground, root.accent)
      : root.selected || root.active ? Style.selectedFillFor(root.foreground, root.accent)
      : root.hot ? Style.hoverFillFor(root.foreground, root.accent) : root.backgroundColor
    borderSpec: root._showFocusRing || root.hot || root.selected || root.active || root.bordered ? root._borderSpec : Border.none()
    Behavior on color { ColorAnimation { duration: 120 } }
  }
  TextMetrics { id: labelMetrics; text: root.text; font.family: root.fontFamily; font.pixelSize: root.fontSize }
  contentItem: Item {
    id: content
    clip: false
    readonly property bool hasIcon: root.iconName !== "" || root.iconText !== ""
    readonly property bool hasLabel: root.text !== "" && !root.iconOnly
    implicitWidth: (hasIcon ? root.iconSize : 0) + (hasLabel ? labelMetrics.advanceWidth : 0)
      + (hasIcon && hasLabel ? Style.spacing.controlGap : 0)
    implicitHeight: Math.max(hasIcon ? root.iconSize : 0, hasLabel ? label.implicitHeight : 0)
    Item {
      id: drawing
      width: content.hasIcon ? root.iconSize : 0
      height: root.iconSize
      anchors.verticalCenter: parent.verticalCenter
      x: root.leftAlign || content.hasLabel ? 0 : (parent.width - width) / 2
      Image {
        id: svgIcon
        objectName: root.iconName ? "action-icon-" + root.iconName : ""
        anchors.fill: parent
        visible: root.iconName !== ""
        source: Icons.source(root.iconName, root.selected || root.active ? root._selectedColor : root.foreground)
        sourceSize: Qt.size(width * Screen.devicePixelRatio, height * Screen.devicePixelRatio)
        fillMode: Image.PreserveAspectFit
      }
      Text {
        anchors.centerIn: parent
        visible: root.iconName === "" && root.iconText !== ""
        text: root.iconText; textFormat: Text.PlainText
        color: root.foreground
        font.family: root.fontFamily; font.pixelSize: root.iconSize
        rotation: root.iconRotation
        RotationAnimation on rotation { from: 0; to: 360; duration: 900; loops: Animation.Infinite; running: root.iconSpinning }
      }
    }
    Text {
      id: label
      visible: content.hasLabel
      anchors.left: drawing.right
      anchors.leftMargin: content.hasIcon ? Style.spacing.controlGap : 0
      anchors.right: parent.right
      anchors.verticalCenter: parent.verticalCenter
      elide: Text.ElideRight
      horizontalAlignment: root.leftAlign ? Text.AlignLeft : Text.AlignHCenter
      text: root.text; textFormat: Text.PlainText
      color: root.textForeground
      font.family: root.fontFamily; font.pixelSize: root.fontSize
    }
  }
  TapHandler { acceptedButtons: Qt.RightButton; onTapped: root.rightClicked() }
  PanelToolTip {
    id: buttonToolTip
    visible: root.tooltipText !== "" && root.hovered
    delay: 1000
    text: root.tooltipText
  }
}
