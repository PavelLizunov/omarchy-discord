pragma ComponentBehavior: Bound
import QtQuick
import QtQuick.Controls
import "../ui"

import "../Api.js" as Api

FocusScope {
  id: root

  property var service: null
  property var list: null
  property bool voiceMode: false
  property bool compactHeader: false
  property string headingText: ""
  property var voiceUsers: []
  property bool loading: false
  property bool timedOut: false
  property bool active: false
  property string channelName: ""
  property bool dismissible: false
  property string audioUserId: ""
  property string audioUserName: ""
  readonly property alias audioControls: participantAudio
  signal closeRequested()
  function openAudio(row) {
    if (!voiceMode || !row || !row.user || !row.user.id || String(row.user.id) === String(service ? service.selfId : "")) return
    audioUserName = Api.userLabel(row.user, service ? service.knownUsers : null)
    audioUserId = String(row.user.id)
    participantAudio.focusControls()[0].forceActiveFocus()
  }
  function closeAudio() {audioUserId = "";root.forceActiveFocus()}
  onRowsChanged: {
    ensureCursor()
    if (audioUserId && indexOfId(rows,audioUserId)<0) closeAudio()
  }

  signal escapeRequested()
  signal moveZone(string direction)
  signal copied()
  signal copyRequested(string text)

  readonly property var rows: voiceMode
    ? voiceUsers.map(function(user) { return {kind:"member", id:String(user.id || ""), user:user, status:"voice", activity:""} })
    : Api.memberRows(list)
  property string cursorId: ""
  readonly property int cursor: indexOfId(rows, cursorId)
  readonly property color foreground: Color.popups.text
  readonly property color muted: Api.secondaryColor(Color.muted, Color.popups.text,
    Api.blend(Style.hoverFillFor(Color.popups.text, Color.accent), Color.popups.background, Style.hoverFillAlpha))
  readonly property string fontFamily: Style.font.family
  readonly property bool dense: width < Style.space(170)
  readonly property int avatarSize: Style.space(dense ? 18 : 24)

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
    copyRequested("@" + name)
    copied()
  }

  function handleKey(event) {
    var key = event.key
    var text = event.text
    var alt = (event.modifiers & Qt.AltModifier) !== 0
    if (alt && key === Qt.Key_H) moveZone("left")
    else if (alt && key === Qt.Key_L) moveZone("right")
    else if (alt) return
    else if (key === Qt.Key_Escape && audioUserId) closeAudio()
    else if (key === Qt.Key_Escape) escapeRequested()
    else if (key === Qt.Key_Down || text === "j") moveCursor(1)
    else if (key === Qt.Key_Up || text === "k") moveCursor(-1)
    else if (key === Qt.Key_Home || text === "g") { cursorId = ""; moveCursor(1) }
    else if (key === Qt.Key_End || text === "G") { cursorId = ""; moveCursor(-1) }
    else if (key === Qt.Key_PageDown) { for (var d = 0; d < 8; d++) moveCursor(1) }
    else if (key === Qt.Key_PageUp) { for (var u = 0; u < 8; u++) moveCursor(-1) }
    else if (voiceMode && (key === Qt.Key_Return || key === Qt.Key_Enter || key === Qt.Key_Menu || (key === Qt.Key_F10 && (event.modifiers & Qt.ShiftModifier)))) openAudio(rows[cursor])
    else if (text === "y" || text === "Y") copyCursor()
    else return
    event.accepted = true
  }

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
        visible: !root.compactHeader
        width: parent.width
        objectName: "people-heading"
        text: root.headingText || (root.voiceMode ? "In voice" :  (root.dismissible ? "Members · Esc closes" : "Members"))
        foreground: root.foreground
      }

      Text {
        id: voiceHeading
        width: parent.width
        visible: root.voiceMode && !root.compactHeader
        text: root.channelName
        textFormat: Text.PlainText
        wrapMode: Text.Wrap
        maximumLineCount: 2
        elide: Text.ElideRight
        color: root.muted; font.family: root.fontFamily; font.pixelSize: Style.font.bodySmall
      }
      Text {
        width: parent.width
        visible: !root.rows.length
        wrapMode: Text.WordWrap
        leftPadding: Style.spacing.rowPaddingX
        text: root.voiceMode ? "Voice participants unavailable." : root.timedOut ? "No member list for this channel."
          : (root.loading ? "Loading members…" : (root.channelName ? "Members unavailable for this channel." : "No channel open."))
        color: root.muted
        font.family: root.fontFamily
        font.pixelSize: Style.font.body
      }

      Flickable {
        id: participantViewport
        objectName: root.objectName === "voice-roster" ? "call-participant-viewport" : "participant-viewport"
        width: parent.width
        height: Math.max(0, parent.height - (root.compactHeader ? 0 : Style.spacing.controlHeight)
          - (voiceHeading.visible ? voiceHeading.height + Style.spacing.xs : 0))
        visible: root.audioUserId !== ""
        clip: true
        contentHeight: participantAudio.implicitHeight
        boundsBehavior: Flickable.StopAtBounds
        ScrollBar.vertical: ScrollBar {policy: ScrollBar.AsNeeded}
        function reveal(item) {
          var y = item.mapToItem(participantAudio,0,0).y
          contentY = Math.max(0,Math.min(Math.max(0,contentHeight-height), y < contentY ? y : y+item.height > contentY+height ? y+item.height-height : contentY))
        }
        ParticipantAudio {
          id: participantAudio
          objectName: root.objectName === "voice-roster" ? "call-participant-audio" : "participant-audio"
          width: participantViewport.width
          service: root.service
          userId: root.audioUserId
          userName: root.audioUserName
          connected: !!(root.service && root.service.voice && root.service.voice.status === "connected")
          callIdentity: root.service && root.service.voice ? String(root.service.voice.guildId)+":"+String(root.service.voice.channelId) : ""
          onClosed: root.closeAudio()
          onFocusRequested: function(item) {participantViewport.reveal(item)}
        }
      }
      ListView {
        id: listView
        width: parent.width
        height: parent.height - (root.compactHeader ? 0 : Style.spacing.controlHeight)
          - (voiceHeading.visible ? voiceHeading.height + Style.spacing.xs : 0)
          - (root.dismissible ? Style.spacing.controlHeight + Style.spacing.sm : 0)
        visible: root.rows.length > 0 && !root.audioUserId
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
          readonly property string displayName: Api.userLabel(user, root.service ? root.service.knownUsers : null)
          readonly property string activity: String(row.activity || "")
          readonly property bool talking: root.voiceMode && !!(root.service && root.service.speaking[String(user.id || "")])
          width: listView.width
          height: header ? Style.spacing.controlHeight
            : Math.max(Style.spacing.popupRowHeight, identity.implicitHeight + Style.spacing.sm * 2)

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

            Avatar {
              id: avatar
              anchors.left: parent.left
              anchors.leftMargin: Style.spacing.sm
              anchors.verticalCenter: parent.verticalCenter
              width: root.avatarSize
              height: root.avatarSize
              service: root.service
              url: memberRow.header ? "" : String(memberRow.user.avatar_url || "")
              name: memberRow.displayName
              objectName: root.voiceMode && !memberRow.header ? "voice-person-" + String(memberRow.user.id || "") : ""
              foreground: root.foreground
              fontFamily: root.fontFamily

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
                  color: root.voiceMode ? (memberRow.talking ? Color.accent : root.muted) : root.statusColor(memberRow.row.status)
                }
              }
            }

            Column {
              id: identity
              anchors.left: avatar.right
              anchors.right: parent.right
              anchors.leftMargin: Style.spacing.sm
              anchors.rightMargin: Style.spacing.sm
              anchors.verticalCenter: parent.verticalCenter
              spacing: 0

              Text {
                width: parent.width
                text: memberRow.displayName + (memberRow.user.bot ? "  BOT" : "")
                textFormat: Text.PlainText
                wrapMode: Text.Wrap
                maximumLineCount: 2
                elide: Text.ElideRight
                color: memberRow.row.status === "offline" ? root.muted : root.foreground
                font.family: root.fontFamily
                font.pixelSize: root.dense ? Style.font.bodySmall : Style.font.body
              }
              Text {
                width: parent.width
                visible: memberRow.activity !== "" && !root.dense
                text: memberRow.activity
                textFormat: Text.PlainText
                elide: Text.ElideRight
                color: root.muted
                font.family: root.fontFamily
                font.pixelSize: Style.font.caption
              }
            }

            MouseArea {
              id: rowMouse
              objectName: root.voiceMode ? "voice-row-"+String(memberRow.user.id || "") : ""
              anchors.fill: parent
              hoverEnabled: true
              acceptedButtons: Qt.LeftButton | Qt.RightButton
              onClicked: function(mouse) {
                root.setCursor(memberRow.index)
                root.forceActiveFocus()
                if (mouse.button === Qt.RightButton) root.openAudio(memberRow.row)
              }
              onDoubleClicked: root.openAudio(memberRow.row)
            }
            PanelToolTip {
              visible: rowMouse.containsMouse || memberRow.hasCursor
              text: memberRow.displayName + (memberRow.user.username ? "\n@" + String(memberRow.user.username) : "")
                + (root.voiceMode ? (memberRow.talking ? "\nSpeaking" : "\nIn voice") + " · Right-click for audio" : "")
                + (memberRow.activity ? "\n" + memberRow.activity : "")
            }
          }
        }
      }
      Button {
        visible: root.dismissible
        text: "Close members"
        focusable: true
        onClicked: root.closeRequested()
      }
    }
  }
}
