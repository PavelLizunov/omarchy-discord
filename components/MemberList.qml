pragma ComponentBehavior: Bound
import QtQuick
import QtQuick.Controls
import QtQuick.Effects
import Quickshell
import qs.Commons
import qs.Ui

import "../Api.js" as Api

// Member pane (the fourth focus zone, only while shown): the groups of a
// member_list_update as headers ("Online — 1,204") with the served member
// rows beneath (avatar through the media cache, display name, status dot,
// activity line). The model is Service.memberList, replaced wholesale by
// every update; presence_update patches arrive as a new list too. Keys:
// j/k move over member rows, g/G ends, Y copies "@username", Esc leaves;
// Alt+h/l bubble to the panel as zone moves.
FocusScope {
  id: root

  property var service: null
  property var list: null
  property bool loading: false
  property bool timedOut: false
  property bool active: false
  property string channelName: ""

  signal escapeRequested()
  signal moveZone(string direction)
  signal copied()

  readonly property var rows: Api.memberRows(list)
  property string cursorId: ""
  readonly property int cursor: indexOfId(rows, cursorId)
  readonly property color foreground: Color.foreground
  readonly property color muted: Api.secondaryColor(Color.muted, Color.foreground, Color.background)
  readonly property string fontFamily: Style.font.family
  readonly property int avatarSize: Style.space(24)

  function indexOfId(list_, id) {
    if (!id) return -1
    for (var i = 0; i < list_.length; i++) if (String(list_[i].id || "") === id) return i
    return -1
  }

  function isMember(row) { return !!row && row.kind === "member" }

  function statusColor(status) {
    switch (String(status || "")) {
      case "online": return Color.accent
      case "idle": return root.muted
      case "dnd": return Color.urgent
      default: return Util.alpha(root.foreground, 0.35)
    }
  }

  function statusLabel(status) {
    switch (String(status || "")) {
      case "online": return "Online"
      case "idle": return "Idle"
      case "dnd": return "Do not disturb"
      default: return "Offline"
    }
  }

  // Step from `from` by `delta` (wrapping) to the next member row.
  function findMember(from, delta) {
    var count = rows.length
    if (!count) return -1
    var index = from
    for (var step = 0; step < count; step++) {
      index = ((index + delta) % count + count) % count
      if (isMember(rows[index])) return index
    }
    return -1
  }

  function setCursor(index) {
    if (index < 0 || index >= rows.length) return
    cursorId = String(rows[index].id || "")
    listView.positionViewAtIndex(index, ListView.Contain)
  }

  function moveCursor(delta) {
    var from = cursor
    if (from < 0) from = delta > 0 ? -1 : 0
    var next = findMember(from, delta)
    if (next >= 0) setCursor(next)
  }

  function ensureCursor() {
    if (cursor >= 0 && isMember(rows[cursor])) return
    var first = findMember(-1, 1)
    cursorId = first >= 0 ? String(rows[first].id || "") : ""
  }

  function copyCursor() {
    var row = cursor >= 0 ? rows[cursor] : null
    if (!isMember(row) || !row.user) return
    var name = String(row.user.username || "")
    if (!name) return
    Quickshell.clipboardText = "@" + name
    copied()
  }

  function handleKey(event) {
    var key = event.key
    var text = event.text
    var alt = (event.modifiers & Qt.AltModifier) !== 0
    if (alt && key === Qt.Key_H) moveZone("left")
    else if (alt && key === Qt.Key_L) moveZone("right")
    else if (alt) return
    else if (key === Qt.Key_Escape) escapeRequested()
    else if (key === Qt.Key_Down || text === "j") moveCursor(1)
    else if (key === Qt.Key_Up || text === "k") moveCursor(-1)
    else if (key === Qt.Key_Home || text === "g") { cursorId = ""; moveCursor(1) }
    else if (key === Qt.Key_End || text === "G") { cursorId = ""; moveCursor(-1) }
    else if (key === Qt.Key_PageDown) { for (var d = 0; d < 8; d++) moveCursor(1) }
    else if (key === Qt.Key_PageUp) { for (var u = 0; u < 8; u++) moveCursor(-1) }
    else if (text === "y" || text === "Y") copyCursor()
    else return
    event.accepted = true
  }

  onRowsChanged: ensureCursor()

  Keys.priority: Keys.BeforeItem
  Keys.onPressed: function(event) { root.handleKey(event) }

  BorderSurface {
    id: pane
    anchors.fill: parent
    radius: Style.cornerRadius
    color: Color.popups.background
    borderSpec: root.active
      ? Border.controlSpec("focus", root.foreground, Color.accent)
      : Border.flat(Color.popups.border, Math.max(1, Style.normalBorderWidth))
    padding: Style.spacing.sm

    Column {
      anchors.fill: parent
      anchors.margins: Style.spacing.sm
      spacing: Style.spacing.xs

      PanelSectionHeader {
        width: parent.width
        text: "Members"
        foreground: root.foreground
      }

      Text {
        width: parent.width
        visible: !root.rows.length
        wrapMode: Text.WordWrap
        leftPadding: Style.spacing.rowPaddingX
        text: root.timedOut ? "No member list for this channel."
          : (root.loading ? "Loading members…" : "No channel open.")
        color: root.muted
        font.family: root.fontFamily
        font.pixelSize: Style.font.body
      }

      ListView {
        id: listView
        width: parent.width
        height: parent.height - Style.spacing.controlHeight
        visible: root.rows.length > 0
        clip: true
        reuseItems: true
        cacheBuffer: Style.space(150)
        boundsBehavior: Flickable.StopAtBounds
        spacing: Style.spacing.xxs
        model: root.rows.length
        ScrollBar.vertical: ScrollBar { policy: ScrollBar.AsNeeded }

        delegate: Item {
          id: memberRow
          required property int index
          readonly property var row: root.rows[index] || ({})
          readonly property bool header: row.kind === "group"
          readonly property var user: row.user || ({})
          readonly property bool hasCursor: root.active && index === root.cursor && !header
          readonly property string avatarPath: !header && root.service && user.avatar_url
            ? String(root.service.mediaPath(String(user.avatar_url), 64) || "") : ""
          readonly property string displayName: String(user.display_name || user.username || "")
          readonly property string activity: String(row.activity || "")
          width: listView.width
          height: header ? Style.spacing.controlHeight
            : (activity ? Style.spacing.popupRowHeight + Style.font.caption * 1.4 : Style.spacing.popupRowHeight)

          PanelSectionHeader {
            visible: memberRow.header
            anchors.left: parent.left
            anchors.right: parent.right
            anchors.bottom: parent.bottom
            text: (String(memberRow.row.name || "") + " — " + Api.formatCount(memberRow.row.count)).toUpperCase()
            foreground: root.foreground
          }

          BorderSurface {
            visible: !memberRow.header
            anchors.fill: parent
            radius: Style.cornerRadius
            color: memberRow.hasCursor || rowMouse.containsMouse
              ? Style.hoverFillFor(root.foreground, Color.accent) : "transparent"
            borderSpec: memberRow.hasCursor
              ? Border.controlSpec("hover-cursor", root.foreground, Color.accent)
              : Border.none()

            Item {
              id: avatar
              anchors.left: parent.left
              anchors.leftMargin: Style.spacing.sm
              anchors.verticalCenter: parent.verticalCenter
              width: root.avatarSize
              height: root.avatarSize

              Rectangle {
                anchors.fill: parent
                radius: width / 2
                color: Util.alpha(root.foreground, 0.12)
                visible: !avatarEffect.visible
                Text {
                  anchors.centerIn: parent
                  text: Api.initials(memberRow.displayName).charAt(0)
                  color: root.foreground
                  font.family: root.fontFamily
                  font.pixelSize: Style.font.caption
                  font.bold: true
                }
              }
              Rectangle {
                id: avatarMask
                anchors.fill: parent
                radius: width / 2
                visible: false
                layer.enabled: true
              }
              Image {
                id: avatarImage
                anchors.fill: parent
                visible: false
                asynchronous: true
                cache: true
                fillMode: Image.PreserveAspectCrop
                sourceSize.width: root.avatarSize * 2
                sourceSize.height: root.avatarSize * 2
                source: memberRow.avatarPath ? "file://" + memberRow.avatarPath : ""
                onStatusChanged: if (status === Image.Error && root.service) root.service.mediaError(memberRow.avatarPath)
              }
              MultiEffect {
                id: avatarEffect
                anchors.fill: avatarImage
                source: avatarImage
                maskEnabled: true
                maskSource: avatarMask
                visible: memberRow.avatarPath !== "" && avatarImage.status === Image.Ready
              }
              // Status dot over the avatar's corner, ringed in the pane
              // colour so it reads on any avatar.
              Rectangle {
                anchors.right: parent.right
                anchors.bottom: parent.bottom
                anchors.margins: -Style.spacing.xxs
                width: Style.spacing.lg + Style.spacing.xxs * 2
                height: width
                radius: width / 2
                color: Color.popups.background
                Rectangle {
                  anchors.centerIn: parent
                  width: Style.spacing.lg
                  height: width
                  radius: width / 2
                  color: root.statusColor(memberRow.row.status)
                }
              }
            }

            Column {
              anchors.left: avatar.right
              anchors.right: parent.right
              anchors.leftMargin: Style.spacing.sm
              anchors.rightMargin: Style.spacing.sm
              anchors.verticalCenter: parent.verticalCenter
              spacing: 0

              Text {
                width: parent.width
                text: memberRow.displayName + (memberRow.user.bot ? "  BOT" : "")
                elide: Text.ElideRight
                color: memberRow.row.status === "offline" ? root.muted : root.foreground
                font.family: root.fontFamily
                font.pixelSize: Style.font.body
              }
              Text {
                width: parent.width
                visible: memberRow.activity !== ""
                text: memberRow.activity
                elide: Text.ElideRight
                color: root.muted
                font.family: root.fontFamily
                font.pixelSize: Style.font.caption
              }
            }

            MouseArea {
              id: rowMouse
              anchors.fill: parent
              hoverEnabled: true
              onClicked: {
                root.setCursor(memberRow.index)
                root.forceActiveFocus()
              }
            }
          }
        }
      }
    }
  }
}
