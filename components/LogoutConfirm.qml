import QtQuick
import "../ui"

FocusScope {
  id: root
  property bool shown: false
  signal confirmed()
  signal dismissed()
  visible: shown
  enabled: shown
  function show() { shown = true; Qt.callLater(function() { cancel.forceActiveFocus() }) }
  function hide() { shown = false; dismissed() }
  Keys.onEscapePressed: hide()
  Rectangle {
    anchors.fill: parent
    color: Color.menu.scrim
    MouseArea { anchors.fill: parent; onClicked: root.hide() }
  }
  BorderSurface {
    anchors.centerIn: parent
    width: Math.min(Style.space(430), parent.width - Style.spacing.panelPadding * 2)
    height: body.implicitHeight + Style.spacing.lg * 2
    color: Color.popups.background
    radius: Style.cornerRadius
    borderSpec: Border.flat(Color.popups.border, Math.max(1, Style.normalBorderWidth))
    MouseArea { anchors.fill: parent }
    Column {
      id: body
      anchors.left: parent.left
      anchors.right: parent.right
      anchors.top: parent.top
      anchors.margins: Style.spacing.lg
      spacing: Style.spacing.md
      Text {
        width: parent.width
        text: "Log out of Discord?"
        color: Color.popups.text
        font.family: Style.font.family
        font.pixelSize: Style.font.title
        font.bold: true
      }
      Text {
        width: parent.width
        wrapMode: Text.WordWrap
        text: "This ends your session and attempts to remove the saved sign-in token. You will need to sign in again."
        color: Color.popups.text
        font.family: Style.font.family
        font.pixelSize: Style.font.body
      }
      Row {
        spacing: Style.spacing.md
        Button {
          id: cancel
          objectName: "logout-cancel"
          text: "Cancel"
          focusable: true
          onClicked: root.hide()
          Keys.onTabPressed: confirm.forceActiveFocus()
          Keys.onBacktabPressed: confirm.forceActiveFocus()
        }
        Button {
          id: confirm
          objectName: "logout-confirm"
          text: "Log out"
          foreground: Color.urgent
          focusable: true
          onClicked: { root.hide(); root.confirmed() }
          Keys.onTabPressed: cancel.forceActiveFocus()
          Keys.onBacktabPressed: cancel.forceActiveFocus()
        }
      }
    }
  }
}
