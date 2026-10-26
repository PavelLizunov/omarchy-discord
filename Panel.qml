import QtQuick
import "ui" as PluginTheme
import Quickshell
import Quickshell.Hyprland
import Quickshell.Io
import qs.Commons as Host
import "ui"

Item {
  id: root
  property var shell: null
  property var manifest: null
  property var service: null
  readonly property alias client: view
  readonly property string pluginId: manifest && manifest.id ? String(manifest.id) : "quickshell.discord"
  readonly property bool persistent: !!(service && service.persistentWindow)
  readonly property bool opened: persistent ? window.visible && view.windowActive : view.opened
  property bool persistentVisible: true
  property string parkWorkspace: ""
  property bool pendingFocus: false
  function applyLayoutMode(compact) {
    var target = 'window = "title:^(Omarchy Discord)$"'
    // Both explicit sizes must stay floating; a restored tile may be smaller than Compact.
    Hyprland.dispatch('hl.dsp.window.float({ ' + target + ', action = "on" })')
    Hyprland.dispatch('hl.dsp.window.resize({ ' + target + ', x = ' + Math.round(Style.space(compact ? 520 : 1040))
      + ', y = ' + Math.round(Style.space(compact ? 560 : 680)) + ', relative = false })')
  }
  readonly property var toplevel: {
    var list = Hyprland.toplevels.values
    for (var i = 0; i < list.length; i++)
      if (String(list[i].title || "") === window.title) return list[i]
    return null
  }

  IpcHandler {
    target: root.pluginId + ".ui"
    function status(): string {
      return JSON.stringify({ source: String(Qt.resolvedUrl("ClientView.qml")),
        composerSource: String(Qt.resolvedUrl("components/Composer.qml")),
        opened: view.opened, width: view.width, height: view.height,
        channelSelected: view.currentChannelId !== "", channelId: view.currentChannelId,
        textLength: view.composer.text.length, chatHeight: view.composer.height,
        draftCount: root.service ? Object.keys(root.service.drafts).length : 0,
        pendingCount: root.service ? Object.keys(root.service.pendingByNonce).length + Object.keys(root.service.uploadChannels).length : 0,
        attachments: view.composer.chips.length, canSend: view.composer.canSubmit,
        sendVisible: view.composer.sendControl.visible,
        sendX: view.composer.sendControl.mapToItem(view, 0, 0).x,
        sendY: view.composer.sendControl.mapToItem(view, 0, 0).y,
        sendWidth: view.composer.sendControl.width, sendHeight: view.composer.sendControl.height,
        attachVisible: view.composer.attachControl.visible,
        attachmentMessages: view.currentMessages.filter(function(row) { return row.attachments && row.attachments.length }).length })
    }
  }

  ThemeSync { hostColor: Host.Color; hostStyle: Host.Style }
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
    if (persistent) {
      if (parkWorkspace) moveWindow(parkWorkspace, false)
      else persistentVisible = false
    }
    view.close()
  }
  Component.onCompleted: {
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
  Timer {
    id:videoFloat;interval:150;property int remaining:0;repeat:true
    onTriggered:{
      Hyprland.dispatch('hl.dsp.window.float({ window = "title:^(Omacord Video)$", action = "on" })')
      Hyprland.dispatch('hl.dsp.window.resize({ window = "title:^(Omacord Video)$", x = 800, y = 580, relative = false })')
      if(--remaining<=0)stop()
    }
  }
  FloatingWindow {
    id: window
    title: "Omarchy Discord"
    visible: root.persistent ? root.persistentVisible : view.opened
    color: PluginTheme.Color.background
    implicitWidth: Style.space(view.compactMode ? 520 : 1040)
    implicitHeight: Style.space(view.compactMode ? 560 : 680)
    minimumSize: Qt.size(Style.space(view.compactMode ? 420 : 640), Style.space(420))
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
      onVideoWindowOpened: {videoFloat.remaining=3;videoFloat.restart()}
      onLayoutModeChanged: function(compact) {
        window.implicitWidth = Style.space(compact ? 520 : 1040)
        window.implicitHeight = Style.space(compact ? 560 : 680)
        Qt.callLater(function() { root.applyLayoutMode(compact) })
      }
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
