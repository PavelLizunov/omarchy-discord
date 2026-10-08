pragma ComponentBehavior: Bound
import QtQuick
import QtQuick.Controls
import "ui"

import "Api.js" as Api
import "Keymap.js" as Keymap
import "Markdown.js" as Markdown

Item {
  id: root

  property var service: null
  property bool opened: false
  property bool focusPrimed: false
  signal closeRequested()
  property string query: ""
  property var entries: []
  property string error: ""
  property bool busy: false
  property int queryGeneration: 0
  property int cursor: 0
  property string searchFilter: "all"
  readonly property int limit: 20
  readonly property int debounceMs: 80
  readonly property string fontFamily: Style.font.family
  readonly property color foreground: Color.popups.text
  readonly property color muted: Api.secondaryColor(Color.muted, Color.popups.text, Color.popups.background)
  readonly property bool loggedOut: !!(service && service.loggedOut)
  readonly property bool offline: !service || !service.connected
  readonly property var rows: {
    if (loggedOut) return [{ kind: "login" }]
    var out = []
    if (query && service && Array.isArray(service.guilds) && (searchFilter === "all" || searchFilter === "servers")) {
      var q = query.trim().toLowerCase()
      for (var g = 0; g < service.guilds.length; g++) {
        var gld = service.guilds[g]
        if (!gld) continue
        var gName = String(gld.name || "")
        if (gName.toLowerCase().indexOf(q) >= 0) {
          out.push({
            kind: "server",
            id: String(gld.id),
            name: gName,
            type: "server",
            guildId: String(gld.id),
            guildName: "Server",
            preview: "Open server channels",
            unread: gld.unread !== "read",
            mentions: Math.max(0, Number(gld.mention_count) || 0),
            muted: false
          })
        }
      }
    }
    if (query && service && service.channelsByGuild && (searchFilter === "all" || searchFilter === "voice")) {
      var qv = query.trim().toLowerCase()
      for (var gid in service.channelsByGuild) {
        var chs = service.channelsByGuild[gid]
        if (!Array.isArray(chs)) continue
        for (var c = 0; c < chs.length; c++) {
          var vch = chs[c]
          if (vch && (vch.type === "voice" || vch.type === "stage") && String(vch.name || "").toLowerCase().indexOf(qv) >= 0) {
            var gname = service.guildNames ? service.guildNames[gid] : ""
            out.push({
              kind: "channel",
              id: String(vch.id),
              name: String(vch.name),
              type: "voice",
              guildId: gid,
              guildName: gname || "Server",
              preview: "Open voice text chat",
              unread: false,
              mentions: 0,
              muted: false
            })
          }
        }
      }
    }
    for (var i = 0; i < entries.length; i++) {
      var entry = entries[i] || ({})
      var channel = entry.channel || ({})
      var chType = String(channel.type || "")
      var isDm = chType === "dm" || chType === "group_dm"
      var isVoice = chType === "voice" || chType === "stage"
      if (searchFilter === "servers") continue
      if (searchFilter === "dms" && !isDm) continue
      if (searchFilter === "voice" && !isVoice) continue
      if (searchFilter === "channels" && isDm) continue
      out.push({ kind: "channel", id: String(channel.id || ""), name: String(channel.name || ""),
        type: chType, guildId: channel.guild_id ? String(channel.guild_id) : "",
        guildName: entry.guild_name ? String(entry.guild_name) : (channel.guild_id ? "" : "Direct message"),
        preview: Markdown.plainText(String(entry.last_message_preview || ""), service ? service.markdownCtx : ({}))
          .replace(/\s+/g, " ").trim(),
        unread: Api.isUnread(channel), mentions: Math.max(0, Number(channel.mention_count) || 0),
        muted: !!channel.muted })
    }
    return out
  }
  readonly property string emptyText: {
    if (loggedOut) return ""
    if (offline) return "The Discord backend is not connected."
    if (service && !service.ready) return "Connecting to Discord…"
    if (error) return error
    if (busy && !rows.length) return "Searching…"
    if (!rows.length) return query ? "No results match “" + query + "”." : "Nothing unread and nothing recent."
    return ""
  }
  readonly property string footerText: "Select an item to open it · Esc closes"

  function open() {
    if (!opened) {
      query = ""
      entries = []
      error = ""
      cursor = 0
      focusPrimed = false
      opened = true
      if (service) service.setUiVisible("quick-switch", true)
      runQuery()
    }
    searchField.text = ""
    Qt.callLater(function() { if (root.opened) searchField.forceActiveFocus() })
    return "opened"
  }

  function close() {
    if (!opened) return "closed"
    opened = false
    queryGeneration++
    focusPrimed = false
    debounce.stop()
    if (service) service.setUiVisible("quick-switch", false)
    closeRequested()
    return "closed"
  }

  function toggle() {
    return opened ? close() : open()
  }


  function runQuery() {
    if (!service) return
    var text = query
    var generation = ++queryGeneration
    busy = true
    service.quickSwitch(text, function(list, err) {
      if (!root.opened || generation !== root.queryGeneration || text !== root.query) return
      root.busy = false
      root.error = String(err || "")
      root.entries = list
      root.cursor = 0
    })
  }

  function setQuery(text) {
    var next = String(text || "")
    if (next === query) return
    query = next
    queryGeneration++
    cursor = 0
    debounce.restart()
  }

  function moveCursor(delta) {
    if (!rows.length) return
    cursor = ((cursor + delta) % rows.length + rows.length) % rows.length
    list.positionViewAtIndex(cursor, ListView.Contain)
  }

  function activate(index) {
    var row = rows[index]
    if (!row || !service) return
    close()
    if (row.kind === "login") { service.openPanel({}); return }
    if (!row.id) return
    if (row.kind === "server" || row.type === "server") {
      Api.browseGuild(service, row.guildId)
      service.openPanel({})
      return
    }
    service.openPanel({ channel_id: row.id })
  }

  function cycleCategoryFocus(delta) {
    var stops = categoryButtons.children.filter(function(item) { return item.visible && typeof item.focusable === "boolean" })
    stops.push(searchField)
    var at = stops.length - 1
    for (var i = 0; i < stops.length; i++) if (stops[i].activeFocus) { at = i; break }
    stops[(at + delta + stops.length) % stops.length].forceActiveFocus()
  }

  Keys.priority: Keys.BeforeItem
  Keys.onPressed: function(event) { root.handleKey(event) }

  function handleKey(event) {
    var key = event.key
    var ctrl = (event.modifiers & Qt.ControlModifier) !== 0
    var shift = (event.modifiers & Qt.ShiftModifier) !== 0
    if (key === Qt.Key_Escape) {
      if (query) { searchField.text = ""; setQuery("") }
      else close()
    }
    else if (key === Qt.Key_Tab || key === Qt.Key_Backtab) cycleCategoryFocus(key === Qt.Key_Backtab || shift ? -1 : 1)
    else if (!searchField.activeFocus) return
    else if (key === Qt.Key_Down || (ctrl && (key === Qt.Key_J || key === Qt.Key_N))) moveCursor(1)
    else if (key === Qt.Key_Up || (ctrl && (key === Qt.Key_K || key === Qt.Key_P))) moveCursor(-1)
    else if (key === Qt.Key_Return || key === Qt.Key_Enter) activate(cursor)
    else if (key === Qt.Key_PageDown) moveCursor(Math.min(rows.length - 1, 8))
    else if (key === Qt.Key_PageUp) moveCursor(-Math.min(rows.length - 1, 8))
    else return
    event.accepted = true
  }

  onRowsChanged: if (cursor >= rows.length) cursor = Math.max(0, rows.length - 1)
  Component.onDestruction: if (service) service.setUiVisible("quick-switch", false)

  Timer {
    id: debounce
    interval: root.debounceMs
    onTriggered: root.runQuery()
  }


    Rectangle {
      anchors.fill: parent
      color: Color.menu.scrim
    }
    MouseArea {
      anchors.fill: parent
      acceptedButtons: Qt.AllButtons
      onClicked: root.close()
    }

    BorderSurface {
      id: card
      objectName: "search-card"
      anchors.horizontalCenter: parent.horizontalCenter
      y: Math.round(Math.max(Style.gapsOut, parent.height * 0.18))
      width: Math.min(Style.space(560), parent.width - Style.gapsOut * 2)
      height: Math.min(categoryHeader.implicitHeight + searchField.height + (searchFeedback.visible ? searchFeedback.implicitHeight : 0)
        + Math.min(root.rows.length * (Math.round(Style.spacing.popupRowHeight * 1.5) + Style.spacing.xxs), Style.spacing.popupRowHeight * 1.5 * 9)
        + searchFooter.implicitHeight + contentColumn.spacing * (searchFeedback.visible ? 4 : 3) + card.contentTopInset + card.contentBottomInset,
        parent.height - y - Style.gapsOut)
      radius: Style.cornerRadius
      color: Color.popups.background
      borderSpec: Border.surfaceSpec("popups", "border", Color.popups.border, Math.max(1, Style.space(2)))
      padding: Style.spacing.popupPadding

      MouseArea { anchors.fill: parent; acceptedButtons: Qt.AllButtons }

      Column {
        id: contentColumn
        anchors.fill: parent
        anchors.topMargin: card.contentTopInset
        anchors.rightMargin: card.contentRightInset
        anchors.bottomMargin: card.contentBottomInset
        anchors.leftMargin: card.contentLeftInset
        spacing: Style.spacing.md

        Column {
          id: categoryHeader
          width: parent.width
          spacing: Style.spacing.xxs

        Item {
          width: parent.width
          height: Style.spacing.controlHeight
          Text {
            anchors.left: parent.left
            anchors.verticalCenter: parent.verticalCenter
            text: "Search"
            color: root.foreground
            font.family: root.fontFamily
            font.pixelSize: Style.font.title
            font.bold: true
          }
          Text {
            anchors.right: parent.right
            anchors.verticalCenter: parent.verticalCenter
            text: "Servers · Channels · Voice · DMs"
            color: root.muted
            font.family: root.fontFamily
            font.pixelSize: Style.font.caption
          }
        }

          Flow {
            id: categoryButtons
            objectName: "search-categories"
            width: parent.width
            spacing: Style.spacing.xxs

            Button {
              iconName: "all"
              text: "All"
              focusable: true
              active: root.searchFilter === "all"
              fontSize: Style.font.caption
              onClicked: root.searchFilter = "all"
            }
            Button {
              iconName: "textChannel"
              text: "Channels"
              focusable: true
              active: root.searchFilter === "channels"
              fontSize: Style.font.caption
              onClicked: root.searchFilter = "channels"
            }
            Button {
              iconName: "headphones"
              text: "Voice"
              focusable: true
              active: root.searchFilter === "voice"
              fontSize: Style.font.caption
              onClicked: root.searchFilter = "voice"
            }
            Button {
              iconName: "navigation"
              text: "Servers"
              focusable: true
              active: root.searchFilter === "servers"
              fontSize: Style.font.caption
              onClicked: root.searchFilter = "servers"
            }
            Button {
              iconName: "user"
              text: "DMs"
              focusable: true
              active: root.searchFilter === "dms"
              fontSize: Style.font.caption
              onClicked: root.searchFilter = "dms"
            }
          }
        }

        TextField {
          id: searchField
          objectName: "search-input"
          width: parent.width
          placeholderText: root.loggedOut ? "Log in to search channels" : "Search servers, channels, voice rooms, or DMs…"
          readOnly: root.loggedOut
          font.pixelSize: Style.font.subtitle
          Keys.priority: Keys.BeforeItem
          Keys.onPressed: function(event) { root.handleKey(event) }
          onTextChanged: root.setQuery(text)
        }

        Text {
          width: parent.width
          id: searchFeedback
          visible: root.emptyText !== ""
          wrapMode: Text.WordWrap
          text: root.emptyText
          color: root.error ? Color.urgent : root.muted
          font.family: root.fontFamily
          font.pixelSize: Style.font.body
          leftPadding: Style.spacing.rowPaddingX
        }

        ListView {
          id: list
          objectName: "search-results"
          width: parent.width
          height: Math.max(0, Math.min(contentHeight, Style.spacing.popupRowHeight * 1.5 * 9,
            contentColumn.height - categoryHeader.height - searchField.height - (searchFeedback.visible ? searchFeedback.height : 0)
            - searchFooter.height - contentColumn.spacing * (searchFeedback.visible ? 4 : 3)))
          visible: root.rows.length > 0
          clip: true
          reuseItems: true
          cacheBuffer: Style.space(150)
          boundsBehavior: Flickable.StopAtBounds
          spacing: Style.spacing.xxs
          model: root.rows.length
          ScrollBar.vertical: ScrollBar { policy: ScrollBar.AsNeeded }

          delegate: BorderSurface {
            id: resultRow
            required property int index
            readonly property var row: root.rows[index] || ({})
            readonly property bool hasCursor: index === root.cursor
            readonly property bool login: String(row.kind || "") === "login"
            readonly property color paintedBackground: Api.blend(Style.hoverFillFor(root.foreground, Color.accent), Color.popups.background,
              hasCursor || rowMouse.containsMouse ? Style.hoverFillAlpha : 0)
            readonly property color secondary: Api.secondaryColor(Color.muted, root.foreground, paintedBackground)
            width: list.width
            height: Math.round(Style.spacing.popupRowHeight * 1.5)
            radius: Style.cornerRadius
            color: hasCursor || rowMouse.containsMouse
              ? Style.hoverFillFor(root.foreground, Color.accent) : "transparent"
            borderSpec: hasCursor
              ? Border.controlSpec("hover-cursor", root.foreground, Color.accent)
              : Border.none()

            Text {
              id: glyph
              anchors.left: parent.left
              anchors.leftMargin: Style.spacing.rowPaddingX
              anchors.verticalCenter: parent.verticalCenter
              width: Style.space(22)
              text: resultRow.login ? "" : Api.channelGlyph(resultRow.row.type)
              color: resultRow.row.unread ? root.foreground : resultRow.secondary
              font.family: root.fontFamily
              font.pixelSize: Style.font.body
            }

            Column {
              anchors.left: glyph.right
              anchors.right: badge.visible ? badge.left : (dot.visible ? dot.left : parent.right)
              anchors.leftMargin: Style.spacing.sm
              anchors.rightMargin: Style.spacing.rowPaddingX
              anchors.verticalCenter: parent.verticalCenter
              spacing: 0

              Row {
                width: parent.width
                spacing: Style.spacing.controlGap

                Rectangle {
                  height: Style.space(16)
                  radius: Style.space(3)
                  width: typeBadgeText.implicitWidth + Style.spacing.xs * 2
                  color: resultRow.row.type === "server" ? Util.alpha(Color.accent, 0.2)
                    : (resultRow.row.type === "voice" || resultRow.row.type === "stage" ? Util.alpha(Color.accent, 0.2)
                      : (resultRow.row.type === "dm" || resultRow.row.type === "group_dm" ? Util.alpha(root.foreground, 0.15)
                        : Util.alpha(root.foreground, 0.08)))

                  Text {
                    id: typeBadgeText
                    anchors.centerIn: parent
                    text: resultRow.row.type === "server" ? "SERVER"
                      : (resultRow.row.type === "voice" || resultRow.row.type === "stage" ? "VOICE"
                        : (resultRow.row.type === "dm" || resultRow.row.type === "group_dm" ? "DM" : "CHANNEL"))
                    color: Api.textColor(resultRow.row.type === "server" || resultRow.row.type === "voice" ? Color.accent : Color.muted, root.foreground,
                      Api.blend(resultRow.row.type === "server" || resultRow.row.type === "voice" || resultRow.row.type === "stage" ? Color.accent : root.foreground,
                        resultRow.paintedBackground, resultRow.row.type === "server" || resultRow.row.type === "voice" || resultRow.row.type === "stage" ? 0.2
                          : resultRow.row.type === "dm" || resultRow.row.type === "group_dm" ? 0.15 : 0.08))
                    font.family: root.fontFamily
                    font.pixelSize: Style.font.caption
                    font.bold: true
                  }
                }

                Text {
                  text: resultRow.login ? "Log in to Discord" : String(resultRow.row.name || "")
                  elide: Text.ElideRight
                  width: Math.min(implicitWidth, parent.width - Style.space(80))
                  color: resultRow.row.muted && !resultRow.row.unread ? resultRow.secondary : root.foreground
                  font.family: root.fontFamily
                  font.pixelSize: Style.font.body
                  font.bold: !!resultRow.row.unread
                }
                Text {
                  visible: text !== ""
                  width: Math.max(0, parent.width - x)
                  text: resultRow.login ? "" : String(resultRow.row.guildName || "")
                  elide: Text.ElideRight
                  color: resultRow.secondary
                  font.family: root.fontFamily
                  font.pixelSize: Style.font.bodySmall
                }
              }
              Text {
                width: parent.width
                visible: text !== ""
                text: resultRow.login ? "The panel opens on the login screen" : String(resultRow.row.preview || "")
                elide: Text.ElideRight
                color: resultRow.secondary
                font.family: root.fontFamily
                font.pixelSize: Style.font.caption
              }
            }

            Rectangle {
              id: dot
              anchors.right: parent.right
              anchors.rightMargin: Style.spacing.rowPaddingX
              anchors.verticalCenter: parent.verticalCenter
              visible: !!resultRow.row.unread && !(resultRow.row.mentions > 0)
              width: Style.spacing.lg
              height: width
              radius: width / 2
              color: Color.urgent
            }
            Rectangle {
              id: badge
              anchors.right: parent.right
              anchors.rightMargin: Style.spacing.sm
              anchors.verticalCenter: parent.verticalCenter
              visible: resultRow.row.mentions > 0
              width: Math.max(height, badgeText.implicitWidth + Style.spacing.sm * 2)
              height: Style.space(16)
              radius: height / 2
              color: Color.urgent

              Text {
                id: badgeText
                anchors.centerIn: parent
                text: resultRow.row.mentions > 99 ? "99+" : String(resultRow.row.mentions || 0)
                color: Color.popups.background
                font.family: root.fontFamily
                font.pixelSize: Style.font.caption
                font.bold: true
              }
            }

            MouseArea {
              id: rowMouse
              anchors.fill: parent
              hoverEnabled: true
              cursorShape: Qt.PointingHandCursor
              onContainsMouseChanged: if (containsMouse) root.cursor = resultRow.index
              onClicked: root.activate(resultRow.index)
            }
          }
        }

        Text {
          id: searchFooter
          objectName: "search-footer"
          width: parent.width
          elide: Text.ElideRight
          text: root.footerText
          color: root.muted
          font.family: root.fontFamily
          font.pixelSize: Style.font.caption
        }
      }
    }
}
