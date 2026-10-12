import QtQuick
import "." as PluginTheme
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
    color: PluginTheme.Color.foreground
    elide: Text.ElideRight
    verticalAlignment: Text.AlignVCenter
    font: root.font
  }
  indicator: Text {
    x: root.width - width - Style.spacing.sm
    y: (root.height-height)/2
    text: "⌄"; color: PluginTheme.Color.foreground; font: root.font
  }
  background: BorderSurface {
    color: Style.controlFill(root.activeFocus, root.hovered, PluginTheme.Color.foreground, PluginTheme.Color.accent)
    borderSpec: Border.controlSpec(root.activeFocus ? "focus" : (root.hovered ? "hover-cursor" : "normal"), PluginTheme.Color.foreground, PluginTheme.Color.accent)
    radius: Style.cornerRadius
  }
  delegate: ItemDelegate {
    required property int index
    required property var modelData
    width: root.width
    highlighted: root.highlightedIndex === index
    contentItem: Text {
      text: String(modelData); color: PluginTheme.Color.foreground; font: root.font; elide: Text.ElideRight
    }
    background: Rectangle {
      color: highlighted ? Style.selectedFillFor(PluginTheme.Color.foreground, PluginTheme.Color.accent) : PluginTheme.Color.popups.background
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
      color: PluginTheme.Color.popups.background
      radius: Style.cornerRadius
      borderSpec: Border.flat(PluginTheme.Color.popups.border, Math.max(1, Style.normalBorderWidth))
    }
  }
}
