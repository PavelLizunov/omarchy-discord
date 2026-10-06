import QtQuick
import "ui"

QtObject {
  id: root
  required property QtObject hostColor
  required property QtObject hostStyle
  property QtObject colorAdapter: Color
  property QtObject styleAdapter: Style

  function sync() {
    colorAdapter.foreground = hostColor.foreground
    colorAdapter.background = hostColor.background
    colorAdapter.accent = hostColor.accent
    colorAdapter.urgent = hostColor.urgent
    colorAdapter.muted = hostColor.muted
    colorAdapter.shellValues = hostColor.shellValues
    styleAdapter.cornerRadius = hostStyle.cornerRadius
    styleAdapter.gapsOut = hostStyle.gapsOut
    styleAdapter.resolvedFontFamily = hostStyle.font.family
    styleAdapter.fontFamily = hostStyle.font.family
    styleAdapter.applyShellValues(hostColor.shellValues)
  }

  property Connections colorChanges: Connections {
    target: root.hostColor
    function onForegroundChanged() { root.sync() }
    function onBackgroundChanged() { root.sync() }
    function onAccentChanged() { root.sync() }
    function onUrgentChanged() { root.sync() }
    function onMutedChanged() { root.sync() }
    function onShellValuesChanged() { root.sync() }
  }
  property Connections styleChanges: Connections {
    target: root.hostStyle
    function onCornerRadiusChanged() { root.sync() }
    function onGapsOutChanged() { root.sync() }
    function onResolvedFontFamilyChanged() { root.sync() }
  }
  Component.onCompleted: sync()
}
