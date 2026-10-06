import QtQuick
import QtQuick.Controls

ComboBox {
  id: root
  font.family: Style.font.family
  font.pixelSize: Style.font.bodySmall
  implicitHeight: Math.max(Style.spacing.controlHeight, label.implicitHeight + Style.spacing.sm * 2)
  leftPadding: Style.spacing.sm
  rightPadding: Style.space(22)
  contentItem: Text {
    id: label
    text: root.displayText
    color: Color.foreground
    elide: Text.ElideRight
    verticalAlignment: Text.AlignVCenter
    font: root.font
  }
  indicator: Text {
    x: root.width - width - Style.spacing.sm
    y: (root.height-height)/2
    text: "⌄"; color: Color.foreground; font: root.font
  }
  background: BorderSurface {
    color: Style.controlFill(root.activeFocus, root.hovered, Color.foreground, Color.accent)
    borderSpec: Border.controlSpec(root.activeFocus ? "focus" : (root.hovered ? "hover-cursor" : "normal"), Color.foreground, Color.accent)
    radius: Style.cornerRadius
  }
  delegate: ItemDelegate {
    required property int index
    required property var modelData
    width: root.width
    highlighted: root.highlightedIndex === index
    contentItem: Text {
      text: String(modelData); color: Color.foreground; font: root.font; elide: Text.ElideRight
    }
    background: Rectangle {
      color: highlighted ? Style.selectedFillFor(Color.foreground, Color.accent) : Color.popups.background
    }
  }
  popup: Popup {
    popupType: Popup.Item
    closePolicy: Popup.CloseOnEscape | Popup.CloseOnPressOutsideParent
    y: root.height
    width: root.width
    padding: Style.spacing.xs
    implicitHeight: list.contentHeight + padding*2
    contentItem: ListView {
      id: list
      clip: true
      implicitHeight: contentHeight
      model: root.popup.visible ? root.delegateModel : null
      currentIndex: root.highlightedIndex
      boundsBehavior: Flickable.StopAtBounds
    }
    background: BorderSurface {
      color: Color.popups.background
      radius: Style.cornerRadius
      borderSpec: Border.flat(Color.popups.border, Math.max(1, Style.normalBorderWidth))
    }
  }
}
