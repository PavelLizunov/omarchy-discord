import QtQuick
import qs.Commons
import qs.Ui

// Per-monitor bar mark. The socket client lives in Service.qml only; this
// widget mirrors it through bar.shell.serviceFor and null-guards everything.
BarWidget {
  id: root

  moduleName: "quickshell.discord"

  readonly property var discord: bar && bar.shell
    ? bar.shell.serviceFor("quickshell.discord") : null
  readonly property color foreground: bar ? bar.foreground : Color.foreground
  readonly property bool online: !!(discord && discord.ready)
  readonly property int mentionCount: discord ? discord.totalMentionCount : 0
  readonly property bool showMentionCount:
    String(root.setting("showMentionCount", "On")) !== "Off"
  readonly property string middleClickAction:
    String(root.setting("middleClick", "Last unread DM"))
  readonly property string badgeText: showMentionCount && mentionCount > 0
    ? (mentionCount > 99 ? "99+" : String(mentionCount)) : ""
  readonly property string tooltip: {
    if (!discord) return "Omarchy Discord"
    var text = "Discord: " + discord.statusText
    if (discord.user && discord.user.username)
      text += " as " + String(discord.user.display_name || discord.user.username)
    if (mentionCount > 0) text += " · " + mentionCount + " mention" + (mentionCount === 1 ? "" : "s")
    return text
  }

  function openFullPanel(payload) {
    if (!bar || !bar.shell) return
    var encoded = JSON.stringify(payload || ({}))
    var host = bar.shell
    if (typeof host.isPluginOpen === "function" && host.isPluginOpen(moduleName)
        && !payload && typeof host.hide === "function") {
      host.hide(moduleName)
      return
    }
    if (typeof host.hide === "function" && typeof host.summon === "function") {
      // Remap an existing panel onto the workspace containing this bar.
      // Splitting hide and summon across event-loop turns lets Wayland finish
      // unmapping the old surface before the shell opens it here.
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
    openFullPanel(channelId ? { channel: channelId } : {})
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
          + Style.space(17))
    fixedHeight: root.vertical ? Style.bar.statusSlot : -1

    Row {
      anchors.centerIn: parent
      spacing: Style.space(4)
      enabled: false

      DiscordIcon {
        id: mark
        anchors.verticalCenter: parent.verticalCenter
        iconSize: Style.bar.iconFont
        color: root.foreground
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
