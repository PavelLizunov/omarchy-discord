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
  property int cursor: 0
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
    for (var i = 0; i < entries.length; i++) {
      var entry = entries[i] || ({})
      var channel = entry.channel || ({})
      out.push({ kind: "channel", id: String(channel.id || ""), name: String(channel.name || ""),
        type: String(channel.type || ""), guildId: channel.guild_id ? String(channel.guild_id) : "",
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
    if (busy && !entries.length) return "Searching…"
    if (!entries.length) return query ? "No channel matches “" + query + "”." : "Nothing unread and nothing recent."
    return ""
  }
  readonly property string footerText: Keymap.footer("switcher")

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
    busy = true
    service.quickSwitch(text, function(list, err) {
      if (!root.opened || text !== root.query) return
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
    service.openPanel({ channel_id: row.id })
  }

  function handleKey(event) {
    var key = event.key
    var ctrl = (event.modifiers & Qt.ControlModifier) !== 0
    var shift = (event.modifiers & Qt.ShiftModifier) !== 0
    if (key === Qt.Key_Escape) {
      if (query) { searchField.text = ""; setQuery("") }
      else close()
    }
    else if (key === Qt.Key_Down || (key === Qt.Key_Tab && !shift) || (ctrl && (key === Qt.Key_J || key === Qt.Key_N))) moveCursor(1)
    else if (key === Qt.Key_Up || key === Qt.Key_Backtab || (key === Qt.Key_Tab && shift) || (ctrl && (key === Qt.Key_K || key === Qt.Key_P))) moveCursor(-1)
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
      anchors.horizontalCenter: parent.horizontalCenter
      y: Math.round(Math.max(Style.gapsOut, parent.height * 0.18))
      width: Math.min(Style.space(560), parent.width - Style.gapsOut * 2)
      height: Math.min(contentColumn.implicitHeight + card.contentTopInset + card.contentBottomInset,
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

        TextField {
          id: searchField
          width: parent.width
          placeholderText: root.loggedOut ? "Log in to search channels" : "Jump to a channel or DM…"
          readOnly: root.loggedOut
          font.pixelSize: Style.font.subtitle
          Keys.priority: Keys.BeforeItem
          Keys.onPressed: function(event) { root.handleKey(event) }
          onTextChanged: root.setQuery(text)
        }

        Text {
          width: parent.width
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
          width: parent.width
          height: Math.min(contentHeight, Style.spacing.popupRowHeight * 1.5 * 9)
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
              color: resultRow.row.unread ? root.foreground : root.muted
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

                Text {
                  text: resultRow.login ? "Log in to Discord" : String(resultRow.row.name || "")
                  elide: Text.ElideRight
                  width: Math.min(implicitWidth, parent.width)
                  color: resultRow.row.muted && !resultRow.row.unread ? root.muted : root.foreground
                  font.family: root.fontFamily
                  font.pixelSize: Style.font.body
                  font.bold: !!resultRow.row.unread
                }
                Text {
                  visible: text !== ""
                  width: Math.max(0, parent.width - x)
                  text: resultRow.login ? "" : String(resultRow.row.guildName || "")
                  elide: Text.ElideRight
                  color: root.muted
                  font.family: root.fontFamily
                  font.pixelSize: Style.font.bodySmall
                }
              }
              Text {
                width: parent.width
                visible: text !== ""
                text: resultRow.login ? "The panel opens on the login screen" : String(resultRow.row.preview || "")
                elide: Text.ElideRight
                color: root.muted
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
