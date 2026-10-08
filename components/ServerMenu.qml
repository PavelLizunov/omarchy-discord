import QtQuick
import "../ui"

FocusScope {
  id: root
  property var service: null
  property var guild: null
  property bool shown: false
  property string page: "menu"
  property real anchorX: 0
  property real anchorY: 0
  readonly property string guildId: guild ? String(guild.id || "") : ""
  readonly property bool busy: !!(service && service.guildActionBusy)
  readonly property bool isArchived: !!(service && typeof service.isArchived === "function" && service.isArchived(guildId))
  signal dismissed()
  function show(row, x, y) {
    guild = row; anchorX = x; anchorY = y; page = "menu"; shown = true
    if (service && service.actionError !== undefined) service.actionError = ""
    if (service && service.settingsError !== undefined) service.settingsError = ""
    Qt.callLater(function() { settings.forceActiveFocus() })
  }
  function hide() { shown = false; dismissed() }
  function action(name, fields) {
    if (!service || busy) return
    if (!service.serverAction(name, guildId, fields, function(ok) { if (ok) root.hide() }))
      service.actionError = "Not connected. Reconnect and try again."
  }
  function firstFocus() { Qt.callLater(function() { cancel.forceActiveFocus() }) }
  visible: shown
  enabled: shown
  Keys.onEscapePressed: hide()
  Keys.onTabPressed: function(event) { cycle(1); event.accepted = true }
  Keys.onBacktabPressed: function(event) { cycle(-1); event.accepted = true }
  function cycle(delta) {
    var items = page === "menu" ? [settings, markRead, archiveBtn, leave, cancel]
      : (page === "settings" ? [mute, cancel] : [cancel, confirm])
    var enabledItems = items.filter(function(it) { return it && it.visible && it.enabled })
    if (!enabledItems.length) return
    var index = -1
    for (var i = 0; i < enabledItems.length; i++) if (enabledItems[i].activeFocus) index = i
    var nextIndex = ((index + delta) % enabledItems.length + enabledItems.length) % enabledItems.length
    enabledItems[nextIndex].forceActiveFocus()
  }
  MouseArea { anchors.fill: parent; onClicked: root.hide() }
  BorderSurface {
    id: card
    objectName: "server-menu-card"
    width: Math.min(Style.space(360), root.width - Style.spacing.panelPadding * 2)
    height: body.implicitHeight + Style.spacing.lg * 2
    x: Math.max(Style.spacing.panelPadding, Math.min(root.anchorX, root.width - width - Style.spacing.panelPadding))
    y: Math.max(Style.spacing.panelPadding, Math.min(root.anchorY, root.height - height - Style.spacing.panelPadding))
    color: Color.popups.background
    radius: Style.cornerRadius
    borderSpec: Border.flat(Color.popups.border, Math.max(1, Style.normalBorderWidth))
    MouseArea { anchors.fill: parent }
    Column {
      id: body
      anchors.left: parent.left; anchors.right: parent.right; anchors.top: parent.top
      anchors.margins: Style.spacing.lg
      spacing: Style.spacing.sm
      Text {
        width: parent.width
        text: root.guild ? String(root.guild.name || "Unnamed server") : ""
        textFormat: Text.PlainText
        wrapMode: Text.Wrap; maximumLineCount: 2; elide: Text.ElideRight
        color: Color.foreground
        font.family: Style.font.family; font.pixelSize: Style.font.subtitle; font.bold: true
      }
      Text {
        width: parent.width; wrapMode: Text.WordWrap
        visible: root.page !== "menu"
        text: root.page === "leave" ? "Leave this server? You will need an invitation to join again. Server owners must transfer ownership first."
          : "Mute this server for your Discord account. Administration and roles are not available in Omacord."
        color: Color.foreground; font.family: Style.font.family; font.pixelSize: Style.font.bodySmall
      }
      Text {
        width: parent.width; wrapMode: Text.WordWrap
        text: root.service ? String(root.service.actionError || "") : ""
        textFormat: Text.PlainText
        visible: text !== ""
        color: Color.urgent; font.family: Style.font.family; font.pixelSize: Style.font.bodySmall
      }
      Button {
        id: settings; objectName: "server-settings"
        width: parent.width; visible: root.page === "menu"; text: "Server settings"; focusable: true
        onClicked: { root.page = "settings"; if (root.service) root.service.loadGuildSettings(root.guildId); root.firstFocus() }
      }
      Button {
        id: markRead; objectName: "server-mark-read"
        width: parent.width; visible: root.page === "menu"; text: root.busy ? "Marking read…" : "Mark all as read"; focusable: true; enabled: !root.busy
        onClicked: root.action("mark_guild_read", {})
      }
      Button {
        id: archiveBtn; objectName: "server-archive"
        width: parent.width; visible: root.page === "menu"
        text: root.isArchived ? "Unarchive server" : "Archive server"
        iconName: "archive"
        focusable: true
        onClicked: {
          if (root.service && typeof root.service.toggleArchive === "function") {
            root.service.toggleArchive(root.guildId, !root.isArchived)
          }
          root.hide()
        }
      }
      Button {
        id: leave; objectName: "server-leave"
        width: parent.width; visible: root.page === "menu"; text: "Leave server…"; foreground: Color.urgent; focusable: true
        onClicked: { root.page = "leave"; root.firstFocus() }
      }
      Text {
        width: parent.width; wrapMode: Text.WordWrap; textFormat: Text.PlainText
        visible: root.page === "settings" && text !== ""
        text: root.service ? String(root.service.settingsError || "") : ""
        color: Color.urgent; font.family: Style.font.family; font.pixelSize: Style.font.bodySmall
      }
      Button {
        id: mute; objectName: "server-mute"
        readonly property var value: root.service && root.service.guildMute ? root.service.guildMute[root.guildId] : undefined
        width: parent.width; visible: root.page === "settings"; enabled: typeof value === "boolean" && !root.busy
        text: typeof value !== "boolean" ? (root.service && root.service.settingsError ? "Settings unavailable" : "Loading settings…") : (value ? "Unmute server" : "Mute server")
        focusable: true
        onClicked: root.action("set_guild_mute", {muted: !value})
      }
      Button {
        id: cancel; objectName: "server-cancel"
        width: parent.width; text: root.page === "leave" ? "Cancel" : "Close"; focusable: true
        onClicked: root.hide()
      }
      Button {
        id: confirm; objectName: "server-leave-confirm"
        width: parent.width; visible: root.page === "leave"; text: root.busy ? "Leaving…" : "Leave server"
        enabled: !root.busy; foreground: Color.urgent; focusable: true
        onClicked: root.action("leave_guild", {confirmed:true})
      }
    }
  }
}
