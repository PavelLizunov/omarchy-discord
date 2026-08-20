import QtQuick
import QtQuick.Controls
import Quickshell
import qs.Commons
import qs.Ui

import "Api.js" as Api

// Phase 0 panel: login form, then a bare two-column guild/channel browser.
// Host contract: root Item with shell/manifest/service injected, `opened`,
// open(payloadJson) (JSON string), close(). Destroyed on hide unless the
// manifest sets keepLoaded, so authoritative state lives in Service.qml.
Item {
  id: root

  property var shell: null
  property var manifest: null
  property var service: null
  property bool opened: false
  property bool closingFromHost: false

  readonly property string pluginId: manifest && manifest.id
    ? String(manifest.id) : "quickshell.discord"
  readonly property color foreground: Color.foreground
  readonly property color background: Color.background
  readonly property color accent: Color.accent
  readonly property string fontFamily: Style.font.family
  readonly property var panelBorderSpec: Border.flat(Color.popups.border,
    Math.max(1, Style.normalBorderWidth))

  readonly property string lifecycle: service ? service.lifecycle : ""
  readonly property bool connected: !!(service && service.connected)
  readonly property bool ready: !!(service && service.ready)
  readonly property bool showLogin: connected
    && (lifecycle === "logged_out" || lifecycle === "reauth_needed")
  readonly property string errorText: service ? Api.redact(service.lastError) : ""

  // --- zones: "guilds" | "channels" ---
  property string zone: "guilds"
  property int guildCursor: 0
  property int channelCursor: 0
  property string selectedGuildId: ""
  property string requestedChannelId: ""

  readonly property var guildRows: {
    var rows = [{ kind: "dms", id: "dms", name: "Direct Messages",
      mention_count: dmMentionCount(), unread: "read" }]
    var guilds = service && Array.isArray(service.guilds) ? service.guilds : []
    for (var i = 0; i < guilds.length; i++) rows.push(guilds[i])
    return rows
  }
  readonly property var channelRows: {
    if (!service) return []
    if (selectedGuildId === "dms") return Array.isArray(service.dms) ? service.dms : []
    if (!selectedGuildId) return []
    return service.channelsFor(selectedGuildId)
  }
  readonly property bool channelsLoading: !!(service && selectedGuildId
    && selectedGuildId !== "dms" && service.isLoadingChannels(selectedGuildId))
  readonly property string selectedGuildName: {
    for (var i = 0; i < guildRows.length; i++)
      if (String(guildRows[i].id) === selectedGuildId) return String(guildRows[i].name || "")
    return ""
  }

  function dmMentionCount() {
    var dms = service && Array.isArray(service.dms) ? service.dms : []
    var total = 0
    for (var i = 0; i < dms.length; i++) total += Number(dms[i].mention_count) || 0
    return total
  }

  function textInputFocused() {
    return tokenField.activeFocus
  }

  // --- host contract ---
  function open(payloadJson) {
    var payload = ({})
    try { payload = JSON.parse(String(payloadJson || "{}")) || ({}) } catch (e) {}
    requestedChannelId = String(payload.channel || "")
    closingFromHost = false
    opened = true
    if (service) {
      service.setUiVisible("full-panel", true)
      service.refresh()
    }
    Qt.callLater(function() {
      focusScope.forceActiveFocus()
      if (root.showLogin) tokenField.forceActiveFocus()
    })
  }

  function close() {
    closingFromHost = true
    opened = false
    tokenField.clear()
    if (service) service.setUiVisible("full-panel", false)
    closingFromHost = false
  }

  function requestClose() {
    if (shell && typeof shell.hide === "function") shell.hide(pluginId)
    else close()
  }

  // --- cursor helpers ---
  function clampCursor(index, length) {
    if (length <= 0) return 0
    return ((index % length) + length) % length
  }

  function ensureCursors() {
    guildCursor = Math.max(0, Math.min(guildCursor, guildRows.length - 1))
    channelCursor = channelRows.length
      ? Math.max(0, Math.min(channelCursor, channelRows.length - 1)) : 0
    if (channelRows.length && !isSelectableChannel(channelRows[channelCursor]))
      moveChannelCursor(1)
  }

  function moveGuildCursor(delta) {
    if (!guildRows.length) return
    guildCursor = clampCursor(guildCursor + delta, guildRows.length)
    guildList.positionViewAtIndex(guildCursor, ListView.Contain)
  }

  function isSelectableChannel(row) {
    return row && String(row.type || "") !== "category"
  }

  function moveChannelCursor(delta) {
    var count = channelRows.length
    if (!count) return
    var index = channelCursor
    for (var step = 0; step < count; step++) {
      index = clampCursor(index + delta, count)
      if (isSelectableChannel(channelRows[index])) break
    }
    channelCursor = index
    channelList.positionViewAtIndex(channelCursor, ListView.Contain)
  }

  function selectGuild(index) {
    if (index < 0 || index >= guildRows.length) return
    guildCursor = index
    var row = guildRows[index]
    selectedGuildId = String(row.id || "")
    channelCursor = 0
    if (service && selectedGuildId !== "dms") service.loadChannels(selectedGuildId)
  }

  function enterChannels() {
    selectGuild(guildCursor)
    zone = "channels"
    channelCursor = -1
    moveChannelCursor(1)
  }

  function leaveChannels() {
    zone = "guilds"
  }

  function submitToken() {
    if (!service) return
    var token = tokenField.text
    tokenField.clear()
    service.login(token)
    token = ""
  }

  function handleKey(event) {
    if (textInputFocused()) {
      if (event.key === Qt.Key_Escape) {
        root.requestClose()
        event.accepted = true
      }
      return
    }
    var key = event.key
    var text = event.text
    if (key === Qt.Key_Escape) {
      if (zone === "channels") leaveChannels()
      else root.requestClose()
      event.accepted = true
      return
    }
    if (!ready) {
      if (text === "r") { if (service) service.refresh(); event.accepted = true }
      return
    }
    if (zone === "guilds") {
      if (key === Qt.Key_Down || text === "j") moveGuildCursor(1)
      else if (key === Qt.Key_Up || text === "k") moveGuildCursor(-1)
      else if (key === Qt.Key_Home || text === "g") { guildCursor = 0; guildList.positionViewAtBeginning() }
      else if (key === Qt.Key_End || text === "G") { guildCursor = guildRows.length - 1; guildList.positionViewAtEnd() }
      else if (key === Qt.Key_Return || key === Qt.Key_Enter || key === Qt.Key_Right
          || text === "l") enterChannels()
      else if (text === "r") { if (service) service.refresh() }
      else return
      event.accepted = true
      return
    }
    if (zone === "channels") {
      if (key === Qt.Key_Down || text === "j") moveChannelCursor(1)
      else if (key === Qt.Key_Up || text === "k") moveChannelCursor(-1)
      else if (key === Qt.Key_Home || text === "g") { channelCursor = -1; moveChannelCursor(1) }
      else if (key === Qt.Key_End || text === "G") { channelCursor = 0; moveChannelCursor(-1) }
      else if (key === Qt.Key_Left || text === "h") leaveChannels()
      else if (key === Qt.Key_Return || key === Qt.Key_Enter) { /* Phase 1 opens the channel */ }
      else if (text === "r") { if (service) service.loadChannels(selectedGuildId, true) }
      else return
      event.accepted = true
    }
  }

  onGuildRowsChanged: ensureCursors()
  onChannelRowsChanged: ensureCursors()
  onShowLoginChanged: if (showLogin && opened) Qt.callLater(function() { tokenField.forceActiveFocus() })
  onReadyChanged: {
    if (ready && !selectedGuildId && guildRows.length) selectGuild(0)
    if (!ready) zone = "guilds"
  }

  Component.onDestruction: {
    tokenField.clear()
    if (service) service.setUiVisible("full-panel", false)
  }

  FloatingWindow {
    id: window
    visible: root.opened
    title: "Omarchy Discord"
    color: root.background
    implicitWidth: 900
    implicitHeight: 640
    minimumSize: Qt.size(560, 400)

    onVisibleChanged: {
      if (!visible && root.opened && !root.closingFromHost) root.requestClose()
    }

    FocusScope {
      id: focusScope
      anchors.fill: parent
      focus: true
      Keys.priority: Keys.BeforeItem
      Keys.onPressed: function(event) { root.handleKey(event) }

      Column {
        anchors.fill: parent
        anchors.margins: Style.spacing.panelPadding
        spacing: Style.spacing.panelGap

        // Header
        Item {
          width: parent.width
          height: Style.spacing.controlHeight

          Row {
            anchors.left: parent.left
            anchors.verticalCenter: parent.verticalCenter
            spacing: Style.spacing.controlGap

            DiscordIcon {
              anchors.verticalCenter: parent.verticalCenter
              iconSize: Style.font.heading
              color: root.foreground
            }
            Text {
              anchors.verticalCenter: parent.verticalCenter
              text: "Omarchy Discord"
              color: root.foreground
              font.family: root.fontFamily
              font.pixelSize: Style.font.title
            }
            Text {
              anchors.verticalCenter: parent.verticalCenter
              text: root.service ? root.service.statusText
                + (root.service.user ? " · " + String(root.service.user.display_name
                  || root.service.user.username || "") : "") : ""
              color: Color.muted
              font.family: root.fontFamily
              font.pixelSize: Style.font.bodySmall
            }
          }

          Row {
            anchors.right: parent.right
            anchors.verticalCenter: parent.verticalCenter
            spacing: Style.spacing.controlGap

            Button {
              visible: root.ready
              text: "Log out"
              foreground: root.foreground
              onClicked: if (root.service) root.service.logout()
            }
            Button {
              text: "Close"
              foreground: root.foreground
              onClicked: root.requestClose()
            }
          }
        }

        // Body
        Item {
          id: body
          width: parent.width
          height: parent.height - Style.spacing.controlHeight - footer.height
            - parent.spacing * 2

          // Status (backend down / starting / connecting)
          Column {
            anchors.centerIn: parent
            visible: !root.showLogin && !root.ready
            spacing: Style.spacing.lg
            width: Math.min(parent.width, Style.space(420))

            Text {
              width: parent.width
              horizontalAlignment: Text.AlignHCenter
              text: root.service ? root.service.statusText : "Loading"
              color: root.foreground
              font.family: root.fontFamily
              font.pixelSize: Style.font.heading
            }
            Text {
              width: parent.width
              horizontalAlignment: Text.AlignHCenter
              wrapMode: Text.WordWrap
              text: {
                if (!root.service) return "The Discord service is not loaded."
                if (!root.service.daemon.runtimeChecked) return "Checking the backend install."
                if (root.service.daemon.setupBusy) return "Installing the bundled backend."
                if (!root.service.daemon.runtimeAvailable) return "The backend is not installed. Run scripts/setup.sh from the plugin directory."
                if (!root.connected) return "Waiting for the backend socket. Press r to retry."
                return "Connecting to Discord."
              }
              color: Color.muted
              font.family: root.fontFamily
              font.pixelSize: Style.font.body
            }
            Button {
              anchors.horizontalCenter: parent.horizontalCenter
              visible: !!(root.service && root.service.daemon.runtimeAvailable
                && !root.service.daemon.running)
              text: "Start backend"
              foreground: root.foreground
              onClicked: if (root.service) root.service.startBackend()
            }
          }

          // Login form
          Column {
            anchors.centerIn: parent
            visible: root.showLogin
            spacing: Style.spacing.lg
            width: Math.min(parent.width, Style.space(460))

            Text {
              width: parent.width
              text: root.lifecycle === "reauth_needed" ? "Login required again" : "Log in to Discord"
              color: root.foreground
              font.family: root.fontFamily
              font.pixelSize: Style.font.heading
            }
            Text {
              width: parent.width
              wrapMode: Text.WordWrap
              text: "Paste a user token and press Enter. It goes straight to the backend and into the keyring; it is never written to disk or shown here. QR login arrives in a later phase."
              color: Color.muted
              font.family: root.fontFamily
              font.pixelSize: Style.font.body
            }
            TextField {
              id: tokenField
              width: parent.width
              password: true
              placeholderText: "Discord user token"
              enabled: !(root.service && root.service.loginBusy)
              onAccepted: root.submitToken()
            }
            Row {
              spacing: Style.spacing.controlGap
              Button {
                text: root.service && root.service.loginBusy ? "Logging in" : "Log in"
                foreground: root.foreground
                enabled: !(root.service && root.service.loginBusy)
                onClicked: root.submitToken()
              }
              Text {
                anchors.verticalCenter: parent.verticalCenter
                text: "Terminal alternative: omarchy-discord-backend login"
                color: Color.muted
                font.family: root.fontFamily
                font.pixelSize: Style.font.caption
              }
            }
          }

          // Two-column browser
          Row {
            anchors.fill: parent
            visible: root.ready
            spacing: Style.spacing.panelGap

            BorderSurface {
              id: guildPane
              width: Math.round(parent.width * 0.34)
              height: parent.height
              radius: Style.cornerRadius
              color: Color.popups.background
              borderSpec: root.zone === "guilds"
                ? Border.controlSpec("focus", root.foreground, root.accent)
                : root.panelBorderSpec
              padding: Style.spacing.sm

              Column {
                anchors.fill: parent
                anchors.margins: Style.spacing.sm
                spacing: Style.spacing.xs

                PanelSectionHeader {
                  width: parent.width
                  text: "Servers"
                  foreground: root.foreground
                }

                ListView {
                  id: guildList
                  width: parent.width
                  height: parent.height - Style.spacing.controlHeight
                  clip: true
                  reuseItems: true
                  cacheBuffer: Style.space(150)
                  boundsBehavior: Flickable.StopAtBounds
                  spacing: Style.spacing.xxs
                  model: root.guildRows.length
                  ScrollBar.vertical: ScrollBar { policy: ScrollBar.AsNeeded }

                  delegate: Item {
                    id: guildRow
                    required property int index
                    readonly property var row: root.guildRows[index] || ({})
                    readonly property bool hasCursor: root.zone === "guilds" && index === root.guildCursor
                    readonly property bool selected: String(row.id) === root.selectedGuildId
                    readonly property int mentions: Number(row.mention_count) || 0
                    width: guildList.width
                    height: Style.spacing.popupRowHeight

                    BorderSurface {
                      anchors.fill: parent
                      radius: Style.cornerRadius
                      color: guildRow.hasCursor
                        ? Style.hoverFillFor(root.foreground, root.accent)
                        : (guildRow.selected ? Style.selectedFillFor(root.foreground, root.accent)
                          : (guildMouse.containsMouse ? Style.hoverFillFor(root.foreground, root.accent)
                            : "transparent"))
                      borderSpec: guildRow.hasCursor
                        ? Border.controlSpec("hover-cursor", root.foreground, root.accent)
                        : Border.none()

                      Text {
                        anchors.left: parent.left
                        anchors.right: guildBadge.visible ? guildBadge.left : parent.right
                        anchors.leftMargin: Style.spacing.rowPaddingX
                        anchors.rightMargin: Style.spacing.sm
                        anchors.verticalCenter: parent.verticalCenter
                        text: String(guildRow.row.name || "")
                        elide: Text.ElideRight
                        color: root.foreground
                        font.family: root.fontFamily
                        font.pixelSize: Style.font.body
                        font.bold: String(guildRow.row.unread || "read") !== "read"
                      }
                      Text {
                        id: guildBadge
                        anchors.right: parent.right
                        anchors.rightMargin: Style.spacing.rowPaddingX
                        anchors.verticalCenter: parent.verticalCenter
                        visible: guildRow.mentions > 0
                        text: guildRow.mentions > 99 ? "99+" : String(guildRow.mentions)
                        color: Color.urgent
                        font.family: root.fontFamily
                        font.pixelSize: Style.font.bodySmall
                        font.bold: true
                      }
                      MouseArea {
                        id: guildMouse
                        anchors.fill: parent
                        hoverEnabled: true
                        onClicked: {
                          root.zone = "guilds"
                          root.guildCursor = guildRow.index
                          root.enterChannels()
                        }
                      }
                    }
                  }
                }
              }
            }

            BorderSurface {
              id: channelPane
              width: parent.width - guildPane.width - parent.spacing
              height: parent.height
              radius: Style.cornerRadius
              color: Color.popups.background
              borderSpec: root.zone === "channels"
                ? Border.controlSpec("focus", root.foreground, root.accent)
                : root.panelBorderSpec
              padding: Style.spacing.sm

              Column {
                anchors.fill: parent
                anchors.margins: Style.spacing.sm
                spacing: Style.spacing.xs

                PanelSectionHeader {
                  width: parent.width
                  text: root.selectedGuildName || "Channels"
                  foreground: root.foreground
                }

                Text {
                  width: parent.width
                  visible: !root.channelRows.length
                  text: !root.selectedGuildId ? "Pick a server with Enter or l."
                    : (root.channelsLoading ? "Loading channels"
                      : (root.selectedGuildId === "dms" ? "No direct messages." : "No visible channels."))
                  color: Color.muted
                  font.family: root.fontFamily
                  font.pixelSize: Style.font.body
                  leftPadding: Style.spacing.rowPaddingX
                }

                ListView {
                  id: channelList
                  width: parent.width
                  height: parent.height - Style.spacing.controlHeight
                  visible: root.channelRows.length > 0
                  clip: true
                  reuseItems: true
                  cacheBuffer: Style.space(150)
                  boundsBehavior: Flickable.StopAtBounds
                  spacing: Style.spacing.xxs
                  model: root.channelRows.length
                  ScrollBar.vertical: ScrollBar { policy: ScrollBar.AsNeeded }

                  delegate: Item {
                    id: channelRow
                    required property int index
                    readonly property var row: root.channelRows[index] || ({})
                    readonly property bool category: String(row.type || "") === "category"
                    readonly property bool hasCursor: root.zone === "channels"
                      && index === root.channelCursor && !category
                    readonly property int mentions: Number(row.mention_count) || 0
                    width: channelList.width
                    height: category ? Style.spacing.controlHeight : Style.spacing.popupRowHeight

                    PanelSectionHeader {
                      visible: channelRow.category
                      anchors.left: parent.left
                      anchors.right: parent.right
                      anchors.bottom: parent.bottom
                      text: String(channelRow.row.name || "").toUpperCase()
                      foreground: root.foreground
                    }

                    BorderSurface {
                      visible: !channelRow.category
                      anchors.fill: parent
                      radius: Style.cornerRadius
                      color: channelRow.hasCursor || channelMouse.containsMouse
                        ? Style.hoverFillFor(root.foreground, root.accent) : "transparent"
                      borderSpec: channelRow.hasCursor
                        ? Border.controlSpec("hover-cursor", root.foreground, root.accent)
                        : Border.none()

                      Text {
                        id: channelGlyph
                        anchors.left: parent.left
                        anchors.leftMargin: Style.spacing.rowPaddingX
                          + (channelRow.row.parent_id ? Style.spacing.lg : 0)
                        anchors.verticalCenter: parent.verticalCenter
                        text: Api.channelGlyph(channelRow.row.type)
                        color: Color.muted
                        font.family: root.fontFamily
                        font.pixelSize: Style.font.body
                      }
                      Text {
                        anchors.left: channelGlyph.right
                        anchors.right: channelBadge.visible ? channelBadge.left : parent.right
                        anchors.leftMargin: Style.spacing.sm
                        anchors.rightMargin: Style.spacing.sm
                        anchors.verticalCenter: parent.verticalCenter
                        text: String(channelRow.row.name || "")
                        elide: Text.ElideRight
                        color: channelRow.row.muted ? Color.muted : root.foreground
                        font.family: root.fontFamily
                        font.pixelSize: Style.font.body
                        font.bold: String(channelRow.row.unread || "read") !== "read"
                      }
                      Text {
                        id: channelBadge
                        anchors.right: parent.right
                        anchors.rightMargin: Style.spacing.rowPaddingX
                        anchors.verticalCenter: parent.verticalCenter
                        visible: channelRow.mentions > 0
                        text: channelRow.mentions > 99 ? "99+" : String(channelRow.mentions)
                        color: Color.urgent
                        font.family: root.fontFamily
                        font.pixelSize: Style.font.bodySmall
                        font.bold: true
                      }
                      MouseArea {
                        id: channelMouse
                        anchors.fill: parent
                        hoverEnabled: true
                        onClicked: {
                          root.zone = "channels"
                          root.channelCursor = channelRow.index
                        }
                      }
                    }
                  }
                }
              }
            }
          }
        }

        // Footer: key hints + redacted error
        Column {
          id: footer
          width: parent.width
          spacing: Style.spacing.xs

          Text {
            width: parent.width
            visible: root.errorText !== ""
            wrapMode: Text.WordWrap
            text: root.errorText
            color: Color.urgent
            font.family: root.fontFamily
            font.pixelSize: Style.font.bodySmall
          }
          Text {
            width: parent.width
            visible: root.service && root.service.statusMessage !== ""
            text: root.service ? root.service.statusMessage : ""
            color: root.accent
            font.family: root.fontFamily
            font.pixelSize: Style.font.bodySmall
          }
          Text {
            width: parent.width
            elide: Text.ElideRight
            text: root.ready
              ? (root.zone === "guilds"
                ? "j/k or arrows move · Enter/l opens channels · r refreshes · Esc closes"
                : "j/k or arrows move · h/Esc back to servers · r reloads")
              : "Esc closes"
            color: Color.muted
            font.family: root.fontFamily
            font.pixelSize: Style.font.caption
          }
        }
      }
    }
  }
}
