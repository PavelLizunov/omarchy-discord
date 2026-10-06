import QtQuick
import Quickshell
import qs.Commons
import qs.Ui

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
  readonly property string callGlyph: inCall ? (callSilent ? "\uf131" : "\uf130") : ""
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

  function openFullPanel(payload) {
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

  implicitWidth: button.implicitWidth
  implicitHeight: button.implicitHeight

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
    anchors.fill: parent
    bar: root.bar
    text: ""
    labelVisible: false
    hasVisualContent: true
    dimmed: !root.online
    tooltipText: root.tooltip
    fixedWidth: root.vertical ? root.barSize
      : Math.max(Style.bar.statusSlot,
        mark.implicitWidth + (root.badgeText ? badgeMetrics.advanceWidth + Style.space(4) : 0)
          + (root.inCall ? callMark.implicitWidth + Style.space(4) : 0)
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
        id: callMark
        anchors.verticalCenter: parent.verticalCenter
        visible: root.inCall
        text: root.callGlyph
        color: root.foreground
        font.family: root.bar ? root.bar.fontFamily : Style.font.family
        font.pixelSize: Style.font.bodySmall
        renderType: Text.NativeRendering
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
      else root.openFullPanel(null)
    }
  }
}
