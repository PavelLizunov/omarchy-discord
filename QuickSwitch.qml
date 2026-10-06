import QtQuick
import Quickshell
import Quickshell.Wayland

Item {
  id: root
  property var service: null
  readonly property bool opened: view.opened
  property bool focusPrimed: false
  function open() { return view.open() }
  function close() { return view.close() }
  function toggle() { return view.toggle() }
  function pickScreen() {
    var name = service ? String(service.panelScreenName || "") : ""
    for (var i = 0; i < Quickshell.screens.length; i++)
      if (String(Quickshell.screens[i].name || "") === name) return Quickshell.screens[i]
    return null
  }
  Timer {
    id: prime
    interval: 75
    onTriggered: if (root.opened) root.focusPrimed = true
  }
  PanelWindow {
    visible: root.opened
    screen: root.opened ? root.pickScreen() : null
    color: "transparent"
    exclusionMode: ExclusionMode.Ignore
    WlrLayershell.namespace: "omarchy-discord-switcher"
    WlrLayershell.layer: WlrLayer.Overlay
    WlrLayershell.keyboardFocus: root.opened
      ? (root.focusPrimed ? WlrKeyboardFocus.OnDemand : WlrKeyboardFocus.Exclusive) : WlrKeyboardFocus.None
    anchors { top: true; bottom: true; left: true; right: true }
    onBackingWindowVisibleChanged: if (backingWindowVisible && root.opened) {
      root.focusPrimed = false
      prime.restart()
    }
    SwitcherView {
      id: view
      anchors.fill: parent
      service: root.service
      onCloseRequested: { root.focusPrimed = false; prime.stop() }
    }
  }
}
