pragma ComponentBehavior: Bound
import QtQuick
import QtQuick.Controls
import "../ui"

import "../Api.js" as Api

import "../Keymap.js" as Keymap

FocusScope {
  id: root

  property bool shown: false
  signal closeRequested()

  readonly property var sections: Keymap.sections()
  readonly property color foreground: Color.popups.text
  readonly property color muted: Api.secondaryColor(Color.muted, Color.foreground, Color.background)
  readonly property string fontFamily: Style.font.family
  readonly property int keyColumnWidth: Style.space(190)

  visible: shown
  enabled: shown

  function show() {
    shown = true
    flick.contentY = 0
    Qt.callLater(function() { if (root.shown) root.forceActiveFocus() })
  }

  function hide() {
    shown = false
    closeRequested()
  }

  function scrollBy(delta) {
    var max = Math.max(0, flick.contentHeight - flick.height)
    flick.contentY = Math.max(0, Math.min(max, flick.contentY + delta))
  }

  function handleKey(event) {
    var key = event.key
    var text = event.text
    var ctrl = (event.modifiers & Qt.ControlModifier) !== 0
    if (key === Qt.Key_Escape || (ctrl && key === Qt.Key_Slash) || text === "?" || key === Qt.Key_Return || key === Qt.Key_Enter) hide()
    else if (key === Qt.Key_Down || text === "j") scrollBy(Style.spacing.popupRowHeight)
    else if (key === Qt.Key_Up || text === "k") scrollBy(-Style.spacing.popupRowHeight)
    else if (key === Qt.Key_PageDown || key === Qt.Key_Space) scrollBy(flick.height * 0.9)
    else if (key === Qt.Key_PageUp) scrollBy(-flick.height * 0.9)
    else if (key === Qt.Key_Home || text === "g") flick.contentY = 0
    else if (key === Qt.Key_End || text === "G") scrollBy(flick.contentHeight)
    event.accepted = true
  }

  Keys.priority: Keys.BeforeItem
  Keys.onPressed: function(event) { root.handleKey(event) }

  Rectangle {
    anchors.fill: parent
    color: Color.menu.scrim
    MouseArea {
      anchors.fill: parent
      acceptedButtons: Qt.AllButtons
      onClicked: root.hide()
    }
  }

  BorderSurface {
    id: card
    anchors.centerIn: parent
    width: Math.min(Style.space(680), parent.width - Style.gapsOut * 2)
    height: Math.min(body.implicitHeight + Style.spacing.controlHeight + column.spacing
      + contentTopInset + contentBottomInset, parent.height - Style.gapsOut * 2)
    radius: Style.cornerRadius
    color: Color.popups.background
    borderSpec: Border.surfaceSpec("popups", "border", Color.popups.border, Math.max(1, Style.space(2)))
    padding: Style.spacing.popupPadding

    MouseArea { anchors.fill: parent; acceptedButtons: Qt.AllButtons }

    Column {
      id: column
      anchors.fill: parent
      anchors.topMargin: card.contentTopInset
      anchors.rightMargin: card.contentRightInset
      anchors.bottomMargin: card.contentBottomInset
      anchors.leftMargin: card.contentLeftInset
      spacing: Style.spacing.md

      Item {
        width: parent.width
        height: Style.spacing.controlHeight
        Text {
          anchors.left: parent.left
          anchors.verticalCenter: parent.verticalCenter
          text: "Keyboard shortcuts"
          color: root.foreground
          font.family: root.fontFamily
          font.pixelSize: Style.font.title
          font.bold: true
        }
        Text {
          anchors.right: parent.right
          anchors.verticalCenter: parent.verticalCenter
          text: "j/k scroll · Esc closes"
          color: root.muted
          font.family: root.fontFamily
          font.pixelSize: Style.font.caption
        }
      }

      Flickable {
        id: flick
        width: parent.width
        height: parent.height - Style.spacing.controlHeight - parent.spacing
        contentWidth: width
        contentHeight: body.implicitHeight
        clip: true
        boundsBehavior: Flickable.StopAtBounds
        ScrollBar.vertical: ScrollBar { policy: ScrollBar.AsNeeded }

        Column {
          id: body
          width: flick.width
          spacing: Style.spacing.lg

          Repeater {
            model: root.sections.length
            delegate: Column {
              id: sectionColumn
              required property int index
              readonly property var section: root.sections[index] || ({ title: "", rows: [] })
              width: body.width
              spacing: Style.spacing.xs

              PanelSectionHeader {
                width: parent.width
                text: String(sectionColumn.section.title || "")
                foreground: root.foreground
              }

              Repeater {
                model: sectionColumn.section.rows.length
                delegate: Item {
                  id: keyRow
                  required property int index
                  readonly property var row: sectionColumn.section.rows[index] || ({})
                  width: sectionColumn.width
                  height: Math.max(keysText.implicitHeight, actionText.implicitHeight) + Style.spacing.xs

                  Text {
                    id: keysText
                    anchors.left: parent.left
                    anchors.leftMargin: Style.spacing.rowPaddingX
                    anchors.top: parent.top
                    width: root.keyColumnWidth
                    wrapMode: Text.Wrap
                    text: String(keyRow.row.keys || "")
                    color: root.foreground
                    font.family: root.fontFamily
                    font.pixelSize: Style.font.bodySmall
                    font.bold: true
                  }
                  Text {
                    id: actionText
                    anchors.left: keysText.right
                    anchors.right: parent.right
                    anchors.leftMargin: Style.spacing.md
                    anchors.rightMargin: Style.spacing.rowPaddingX
                    anchors.top: parent.top
                    wrapMode: Text.WordWrap
                    text: String(keyRow.row.action || "")
                    color: root.muted
                    font.family: root.fontFamily
                    font.pixelSize: Style.font.bodySmall
                  }
                }
              }
            }
          }
        }
      }
    }
  }
}
