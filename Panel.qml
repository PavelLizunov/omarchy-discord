import QtQuick
import Quickshell
import Quickshell.Hyprland
import qs.Commons as Host
import "ui"

// Quickshell owns windows and system actions; ClientView is the same Qt Quick
// consumer rendered by MCP. No backend or desktop access lives in that view.
Item {
  id: root
  property var shell: null
  property var manifest: null
  property var service: null
  readonly property string pluginId: manifest && manifest.id ? String(manifest.id) : "quickshell.discord"
  readonly property bool persistent: !!(service && service.persistentWindow)
  readonly property bool opened: persistent ? window.visible && view.windowActive : view.opened
  property bool persistentVisible: true
  property string parkWorkspace: ""
  property bool pendingFocus: false
  readonly property var toplevel: {
    var list = Hyprland.toplevels.values
    for (var i = 0; i < list.length; i++)
      if (String(list[i].title || "") === window.title) return list[i]
    return null
  }

  function syncTheme() {
    Color.foreground = Host.Color.foreground
    Color.background = Host.Color.background
    Color.accent = Host.Color.accent
    Color.urgent = Host.Color.urgent
    Color.muted = Host.Color.muted
    Color.shellValues = Host.Color.shellValues
    Style.cornerRadius = Host.Style.cornerRadius
    Style.gapsOut = Host.Style.gapsOut
    Style.resolvedFontFamily = Host.Style.font.family
    Style.fontFamily = Host.Style.font.family
    Style.applyShellValues(Host.Color.shellValues)
  }
  Connections {
    target: Host.Color
    function onForegroundChanged() { root.syncTheme() }
    function onBackgroundChanged() { root.syncTheme() }
    function onAccentChanged() { root.syncTheme() }
    function onUrgentChanged() { root.syncTheme() }
    function onMutedChanged() { root.syncTheme() }
    function onShellValuesChanged() { root.syncTheme() }
  }
  Connections {
    target: Host.Style
    function onCornerRadiusChanged() { root.syncTheme() }
    function onGapsOutChanged() { root.syncTheme() }
    function onResolvedFontFamilyChanged() { root.syncTheme() }
  }
  function moveWindow(workspace, follow) {
    Hyprland.dispatch("hl.dsp.window.move({ window = \"title:^(Omarchy Discord)$\", workspace = \""
      + workspace + "\"" + (follow ? "" : ", follow = false") + " })")
  }
  function summonHere() {
    var mon = Hyprland.focusedMonitor
    var ipc = mon ? mon.lastIpcObject : null
    var special = ipc && ipc.specialWorkspace ? String(ipc.specialWorkspace.name || "") : ""
    var ws = Hyprland.focusedWorkspace
    var target = special || (ws && ws.id !== undefined ? String(ws.id) : "")
    if (target) moveWindow(target, true)
    Hyprland.dispatch('hl.dsp.focus({ window = "title:^(Omarchy Discord)$" })')
  }
  function open(payload) {
    if (persistent) {
      persistentVisible = true
      pendingFocus = true
      Hyprland.refreshToplevels()
      summonRetry.restart()
    }
    view.open(payload)
  }
  function close() {
    if (persistent && opened) {
      if (parkWorkspace) moveWindow(parkWorkspace, false)
      else persistentVisible = false
    }
    view.close()
  }
  Component.onCompleted: {
    syncTheme()
    if (persistent) Hyprland.refreshToplevels()
  }
  onPersistentChanged: if (persistent) Hyprland.refreshToplevels()
  onToplevelChanged: if (toplevel && toplevel.workspace) {
    var name = String(toplevel.workspace.name || "")
    if (!parkWorkspace && name.indexOf("special:") === 0) parkWorkspace = name
  }
  Timer {
    id: summonRetry
    interval: 120
    repeat: true
    property int tries: 0
    onTriggered: {
      if (!root.pendingFocus) { stop(); tries = 0; return }
      root.summonHere()
      if (++tries >= 5) { root.pendingFocus = false; stop(); tries = 0 }
    }
  }
  FloatingWindow {
    id: window
    title: "Omarchy Discord"
    visible: root.persistent ? root.persistentVisible : view.opened
    color: Color.background
    implicitWidth: Style.space(1040)
    implicitHeight: Style.space(680)
    minimumSize: Qt.size(Style.space(640), Style.space(420))
    onVisibleChanged: if (!visible) {
      if (root.persistent) root.persistentVisible = false
      else view.close()
    }
    ClientView {
      id: view
      anchors.fill: parent
      service: root.service
      manifest: root.manifest
      mapped: window.visible
      windowActive: Window.active
      screenName: window.screen ? String(window.screen.name || "") : ""
      onCloseRequested: {
        if (root.shell && typeof root.shell.hide === "function") root.shell.hide(root.pluginId)
        else root.close()
      }
      onCopyRequested: function(text) { Quickshell.clipboardText = text }
      onLinkRequested: function(url) {
        if (/^https?:\/\//i.test(url)) Quickshell.execDetached(["xdg-open", url])
        else if (root.service) root.service.fail("Only HTTP and HTTPS links can be opened")
      }
    }
  }
}
