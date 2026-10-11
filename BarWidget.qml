import QtQuick
import Quickshell
import qs.Commons
import qs.Ui
import Quickshell.Io
import "ui/Icons.js" as Icons

BarWidget {
  id: root

  moduleName: "quickshell.discord"

  readonly property var discord: bar && bar.shell
    ? bar.shell.serviceFor("quickshell.discord") : null
  readonly property color foreground: bar ? bar.foreground : Color.foreground
  readonly property var hostWindow: QsWindow.window
  readonly property string screenName: hostWindow && hostWindow.screen
    ? String(hostWindow.screen.name || "") : ""
  readonly property bool panelOnThisScreen: !discord || !discord.panelScreenName
    || !screenName || discord.panelScreenName === screenName
  readonly property bool online: !!(discord && discord.showStructure)
  readonly property int mentionCount: discord ? discord.totalMentionCount : 0
  readonly property bool unreadDot: online && mentionCount === 0
    && !!(discord && discord.anyUnread)
  readonly property bool showMentionCount:
    String(root.setting("showMentionCount", "On")) !== "Off"
  readonly property string middleClickAction:
    String(root.setting("middleClick", "Last unread DM"))
  readonly property string badgeText: showMentionCount && mentionCount > 0
    ? (mentionCount > 99 ? "99+" : String(mentionCount)) : ""
  readonly property bool inCall: !!(discord && discord.voice
    && String(discord.voice.status || "idle") === "connected")
  readonly property bool callSilent: inCall
    && !!(discord.voice.muted || discord.voice.deafened)
  readonly property string tooltip: {
    if (!discord) return "Omarchy Discord"
    var text = "Discord: " + discord.statusText
    if (discord.user && discord.user.username)
      text += " as " + String(discord.user.display_name || discord.user.username)
    if (inCall) text += " · in voice" + (callSilent ? " (muted)" : "")
    if (mentionCount > 0) text += " · " + mentionCount + " mention" + (mentionCount === 1 ? "" : "s")
    else if (unreadDot) text += " · unread"
    return text
  }

  property bool opened: false
  property int openRequests: 0
  property int closeRequests: 0
  property bool popoutSwitchClosing: false
  function closeForPopoutSwitch() { popoutSwitchClosing = true; close() }
  function open() { popoutSwitchClosing = false; openRequests++; opened = true }
  function close() { closeRequests++; opened = false }
  function toggle() { opened ? close() : open() }

  function openFullPanel(payload) {
    close()
    if (!bar || !bar.shell) return
    var encoded = JSON.stringify(payload || ({}))
    var host = bar.shell
    var isOpen = (discord && discord.persistentWindow)
      ? (discord.panelMapped && discord.panelActive)
      : (typeof host.isPluginOpen === "function" && host.isPluginOpen(moduleName))
    if (isOpen && !payload && panelOnThisScreen && typeof host.hide === "function") {
      host.hide(moduleName)
      return
    }
    if (typeof host.hide === "function" && typeof host.summon === "function") {
      if (discord && discord.persistentWindow) {
        host.summon(moduleName, encoded)
        return
      }
      host.hide(moduleName)
      Qt.callLater(function() {
        if (root.bar && root.bar.shell) root.bar.shell.summon(root.moduleName, encoded)
      })
    } else if (typeof host.toggle === "function") host.toggle(moduleName, encoded)
  }

  function middleClick() {
    if (middleClickAction === "Raise panel") {
      openFullPanel({})
      return
    }
    var channelId = discord ? discord.unreadDmChannelId : ""
    openFullPanel(channelId ? { channel_id: channelId } : {})
  }

  function syncSettings() {
    if (discord) discord.applySettings(settings)
  }

  implicitWidth: vertical ? button.implicitWidth : button.implicitWidth + (inCall ? quickMute.implicitWidth : 0)
  implicitHeight: button.implicitHeight + (vertical && inCall ? quickMute.implicitHeight : 0)

  QuickPopup {
    id: popup
    anchorItem: button
    owner: root
    bar: root.bar
    open: root.opened
    service: root.discord
    popoutSwitchClosing: root.popoutSwitchClosing
    onFullRequested: function(payload) { root.openFullPanel(payload) }
  }

  IpcHandler {
    target: "quickshell.discord.quick"
    function evidence(): string {
      return JSON.stringify({source:String(Qt.resolvedUrl("BarWidget.qml")), opened:root.opened,
        compactComponent:!!popup.content, keyboardPanel:true, openRequests:root.openRequests, closeRequests:root.closeRequests,
        popupMapped:popup.backingWindowVisible, focusPrimed:popup.focusPrimed,
        compactWidth:popup.contentWidth, compactHeight:popup.contentHeight,
        quickMuteVisible:quickMute.visible, muteIconCount:quickMute.visible ? 1 : 0,
        iconSize:mark.width, muteIconSize:muteDrawing.width, muteIconReady:muteDrawing.status === Image.Ready, inCall:root.inCall,
        muted:!!(root.discord && root.discord.voice && root.discord.voice.muted),
        backendReady:!!(root.discord && root.discord.ready)})
    }
  }

  WidgetButton {
    id: quickMute
    width: implicitWidth
    height: implicitHeight
    objectName: "bar-quick-mute"
    bar: root.bar
    visible: root.inCall
    x: root.vertical ? 0 : button.width
    y: root.vertical ? button.height : 0
    fixedWidth: root.vertical ? root.barSize : Style.bar.statusSlot
    fixedHeight: root.vertical ? Style.bar.statusSlot : -1
    text: ""
    labelVisible: false
    hasVisualContent: true
    Image {
      id: muteDrawing
      objectName: "bar-mute-icon"
      anchors.centerIn: parent
      width: Style.bar.iconFont
      height: width
      sourceSize.width: Math.ceil(width * 2)
      sourceSize.height: Math.ceil(height * 2)
      source: Icons.source(root.discord && root.discord.voice.muted ? "microphoneOff" : "microphone",
        root.discord && root.discord.voice.muted ? Color.urgent : root.foreground)
    }
    active: !!(root.discord && root.discord.voice.muted)
    tooltipText: root.discord && root.discord.voice.muted ? "Unmute microphone" : "Mute microphone"
    onPressed: function(mouseButton) { if (mouseButton === Qt.LeftButton && root.discord) root.discord.toggleMute() }
  }

  onSettingsChanged: syncSettings()
  onDiscordChanged: syncSettings()
  Component.onCompleted: syncSettings()

  TextMetrics {
    id: badgeMetrics
    text: root.badgeText
    font.family: root.bar ? root.bar.fontFamily : Style.font.family
    font.pixelSize: Style.font.bodySmall
  }

  WidgetButton {
    id: button
    objectName: "bar-compact-toggle"
    width: implicitWidth
    height: implicitHeight
    bar: root.bar
    text: ""
    labelVisible: false
    hasVisualContent: true
    dimmed: !root.online
    tooltipText: root.tooltip
    fixedWidth: root.vertical ? root.barSize
      : Math.max(Style.bar.statusSlot,
        mark.implicitWidth + (root.badgeText ? badgeMetrics.advanceWidth + Style.space(4) : 0)
          + Style.space(17))
    fixedHeight: root.vertical ? Style.bar.statusSlot : -1

    Row {
      anchors.centerIn: parent
      spacing: Style.space(4)
      enabled: false

      Item {
        anchors.verticalCenter: parent.verticalCenter
        implicitWidth: mark.implicitWidth
        implicitHeight: mark.implicitHeight

        DiscordIcon {
          id: mark
          objectName: "bar-primary-icon"
          anchors.centerIn: parent
          iconSize: Style.bar.iconFont
          color: root.foreground
        }

        Rectangle {
          visible: root.unreadDot
          anchors.right: parent.right
          anchors.top: parent.top
          anchors.margins: -Style.space(1)
          width: Style.space(5)
          height: width
          radius: width / 2
          color: Util.alpha(root.foreground, 0.7)
        }
      }

      Text {
        anchors.verticalCenter: parent.verticalCenter
        visible: root.badgeText !== ""
        text: root.badgeText
        color: Color.urgent
        font.family: root.bar ? root.bar.fontFamily : Style.font.family
        font.pixelSize: Style.font.bodySmall
        font.bold: true
        renderType: Text.NativeRendering
      }
    }

    onPressed: function(mouseButton) {
      if (mouseButton === Qt.MiddleButton) root.middleClick()
      else if (mouseButton === Qt.RightButton) root.openFullPanel({})
      else root.toggle()
    }
  }
}
