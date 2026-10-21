import QtQuick
import "ui" as PluginTheme
import Quickshell
import qs.Commons
import qs.Commons as Host
import qs.Ui
import Quickshell.Io
import "ui/Icons.js" as Icons
import "components" as Components

BarWidget {
  id: root

  moduleName: "quickshell.discord"

  ThemeSync { hostColor: Host.Color; hostStyle: Host.Style }

  readonly property var discord: bar && bar.shell
    ? bar.shell.serviceFor("quickshell.discord") : null
  readonly property color foreground: bar ? bar.foreground : PluginTheme.Color.foreground
  readonly property var hostWindow: QsWindow.window
  readonly property string screenName: hostWindow && hostWindow.screen
    ? String(hostWindow.screen.name || "") : ""
  readonly property bool panelOnThisScreen: !discord || !discord.panelScreenName
    || !screenName || discord.panelScreenName === screenName
  readonly property bool online: !!(discord && discord.showStructure)
  readonly property int mentionCount: discord ? discord.totalMentionCount : 0
  readonly property bool unreadDot: online && badgeText === ""
    && !!(discord && (discord.anyUnread || mentionCount > 0))
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

  implicitWidth: vertical ? button.implicitWidth : button.implicitWidth + quickMute.implicitWidth
  implicitHeight: button.implicitHeight + (vertical ? quickMute.implicitHeight : 0)

  Components.BarAudioControl { id: audioControl; service: root.discord }

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
        themeForeground:String(PluginTheme.Color.foreground), themeBackground:String(PluginTheme.Color.background),
        rosterForeground:String(popup.content.callPanel.roster.foreground),
        popupBackground:String(PluginTheme.Color.popups.background), popupForeground:String(PluginTheme.Color.popups.text),
        hostForeground:String(Host.Color.foreground), hostBackground:String(Host.Color.background),
        popupMapped:popup.backingWindowVisible, focusPrimed:popup.focusPrimed,
        compactWidth:popup.contentWidth, compactHeight:popup.contentHeight,
        quickMuteVisible:quickMute.visible, muteIconCount:quickMute.visible ? 1 : 0,
        audioButtonWidth:quickMute.width, trailingPadding:quickMute.width-muteDrawing.x-muteDrawing.width,
        iconGap:root.vertical ? null : muteDrawing.mapToItem(root,0,0).x-mark.mapToItem(root,mark.width,0).x,
        iconSize:mark.width, iconReady:mark.iconStatus===Image.Ready, microphoneEnabled:quickMute.enabled, muteIconSize:muteDrawing.width, muteIconReady:muteDrawing.status === Image.Ready, inCall:root.inCall,
        muted:!!(root.discord && root.discord.voice && root.discord.voice.muted),
        backendReady:!!(root.discord && root.discord.ready),
        callView:popup.content.callView,
        voiceRoom:String(root.discord && root.discord.voice ? root.discord.voice.channelId || "" : ""),
        rosterCount:popup.content.callPanel.roster.rows.length,
        singleClickAudio:popup.content.callPanel.roster.singleClickAudio,
        unreadDot:root.unreadDot, unreadDotVisible:unreadDrawing.visible, mentionCount:root.mentionCount,
        indicatorRevision:"upper-badge-v1", badgeText:root.badgeText,
        badgeAboveIcon:mentionBadge.visible && mentionBadge.mapToItem(root,0,0).y < mark.mapToItem(root,0,0).y,
        deafened:!!(root.discord && root.discord.voice.deafened), audioBusy:audioControl.busy,
        microphoneIcon:root.discord && root.discord.voice.deafened ? "headphonesOff" : root.discord && root.discord.voice.muted ? "microphoneOff" : "microphone"})
    }
  }

  WidgetButton {
    id: quickMute
    width: implicitWidth
    height: implicitHeight
    objectName: "bar-quick-mute"
    bar: root.bar
    visible: true
    enabled: audioControl.available && !audioControl.busy
    dimmed: !root.inCall
    activeFocusOnTab: true
    Accessible.role: Accessible.Button
    Accessible.name: tooltipText
    Keys.onReturnPressed: function(event) { triggerPress(event.modifiers & Qt.ShiftModifier ? Qt.RightButton : Qt.LeftButton) }
    Keys.onSpacePressed: function(event) { triggerPress(event.modifiers & Qt.ShiftModifier ? Qt.RightButton : Qt.LeftButton) }
    x: root.vertical ? 0 : button.width
    y: root.vertical ? button.height : 0
    fixedWidth: root.vertical ? root.barSize : Style.bar.statusSlot - Style.space(5)
    fixedHeight: root.vertical ? Style.bar.statusSlot : -1
    text: ""
    labelVisible: false
    hasVisualContent: true
    Image {
      id: muteDrawing
      anchors.horizontalCenterOffset: root.vertical || root.badgeText ? 0 : -Style.space(2)
      objectName: "bar-mute-icon"
      anchors.centerIn: parent
      width: Math.round(Style.bar.iconCanvas * 0.75)
      height: width
      sourceSize.width: Math.max(1, Math.ceil(width * Screen.devicePixelRatio))
      sourceSize.height: Math.max(1, Math.ceil(height * Screen.devicePixelRatio))
      source: Icons.source(root.discord && root.discord.voice.deafened ? "barHeadphonesOff" : root.discord && root.discord.voice.muted ? "barMicrophoneOff" : "barMicrophone",
        root.callSilent ? PluginTheme.Color.urgent : root.foreground, sourceSize.width)
    }
    active: root.callSilent
    tooltipText: !root.inCall ? "Audio controls unavailable · join a voice channel"
      : root.discord.voice.deafened ? "Discord sound off · left click: enable microphone and sound · right click or Shift+Return: restore previous microphone state"
      : (root.discord.voice.muted ? "Left click: unmute microphone" : "Left click: mute microphone")
        + " · Right click or Shift+Return: mute microphone and Discord sound"
    onPressed: function(mouseButton) {
      if (!enabled) return
      if (mouseButton === Qt.LeftButton) audioControl.toggleMicrophone()
      else if (mouseButton === Qt.RightButton) audioControl.toggleSilence()
    }
    Rectangle {anchors.fill:parent;visible:quickMute.activeFocus;color:"transparent";border.width:1;border.color:root.foreground}
  }

  onSettingsChanged: syncSettings()
  onDiscordChanged: syncSettings()
  Component.onCompleted: syncSettings()

  TextMetrics {
    id: badgeMetrics
    text: root.badgeText
    font.family: root.bar ? root.bar.fontFamily : Style.font.family
    font.pixelSize: Style.font.bodySmall
    font.bold: true
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
        mark.implicitWidth + (root.badgeText ? badgeMetrics.advanceWidth : 0)
          + Style.space(5))
    fixedHeight: root.vertical ? Style.bar.statusSlot : -1

    Item {
      anchors.horizontalCenterOffset: root.vertical || root.badgeText ? 0 : Style.space(1)
      anchors.centerIn: parent
      width: mark.implicitWidth + (!root.vertical && root.badgeText ? badgeMetrics.advanceWidth : 0)
      height: mark.implicitHeight
      enabled: false

      Item {
        anchors.left: parent.left
        anchors.verticalCenter: parent.verticalCenter
        width: mark.implicitWidth
        height: mark.implicitHeight

        DiscordIcon {
          id: mark
          objectName: "bar-primary-icon"
          anchors.centerIn: parent
          iconSize: Style.bar.iconCanvas
          color: root.foreground
        }

        Rectangle {
          id: unreadDrawing
          objectName: "bar-unread-dot"
          visible: root.unreadDot
          anchors.right: parent.right
          anchors.top: parent.top
          anchors.topMargin: -Style.space(1)
          anchors.rightMargin: 0
          width: Style.space(5)
          height: width
          radius: width / 2
          color: Util.alpha(root.foreground, 0.7)
        }
      }

      Rectangle {
        id: mentionBadge
        objectName: "bar-mention-badge"
        anchors.right: parent.right
        anchors.top: parent.top
        anchors.topMargin: -Style.space(4)
        visible: root.badgeText !== ""
        width: badgeMetrics.advanceWidth + Style.space(4)
        height: Math.max(Style.space(12), count.implicitHeight)
        radius: height / 2
        color: PluginTheme.Color.urgent
        Text {
          id: count
          objectName: "bar-mention-count"
          anchors.centerIn: parent
          text: root.badgeText
          color: PluginTheme.Color.background
          font.family: root.bar ? root.bar.fontFamily : Style.font.family
          font.pixelSize: Style.font.bodySmall
          font.bold: true
          renderType: Text.NativeRendering
        }
      }
    }

    onPressed: function(mouseButton) {
      if (mouseButton === Qt.MiddleButton) root.middleClick()
      else if (mouseButton === Qt.RightButton) root.openFullPanel({})
      else root.toggle()
    }
  }
}
