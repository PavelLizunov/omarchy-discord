import QtQuick

// The harness symlinks this in as QuickSwitch.qml: the real overlay is a
// PanelWindow (layer shell), which has no backend under
// QT_QPA_PLATFORM=offscreen, and Service.qml's Component would fail to
// resolve the type even though it never instantiates it (active: false).
Item {
  property var service: null
  function show() {}
  function hide() {}
}
