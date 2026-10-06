pragma ComponentBehavior: Bound
import QtQuick
import QtQuick.Controls
import "../ui"

import "../Api.js" as Api

import "../Markdown.js" as Markdown

FocusScope {
  id: timeline

  property var messages: []
  property bool hasMore: false
  property bool loading: false
  property string channelId: ""
  property string selfId: ""
  property string lastReadMessageId: ""
  property bool active: false
  property bool viewing: active
  property var ctx: ({})
  property string cursorMessageId: ""
  property string armedDeleteId: ""
  readonly property int deleteArmMs: 3000
  property var revealed: ({})
  property var selectionOwner: null
  readonly property bool hasSelection: !!selectionOwner && selectionOwner.hasSelection

  signal requestHistory(string beforeId)
  signal escapeRequested()
  signal moveZone(string direction)
  signal cycleFocus(int delta)
  signal openLink(string url)
  signal copied()
  signal copyRequested(string text)
  signal linkCopied()
  signal activateMessage(string messageId)
  signal reachedBottom()
  signal replyRequested(string messageId)
  signal editRequested(string messageId)
  readonly property alias messageActions: actions
  signal deleteRequested(string messageId)
  signal reactRequested(string messageId)
  signal reactionToggled(string messageId, string emoji)

  function scrollToBottom() {
    adjusting = true
    list.positionViewAtEnd()
    adjusting = false
    pinned = true
    checkBottom()
  }

  function focusNewest() {
    if (!rows.length) return
    cursorMessageId = String(rows[rows.length - 1].id || "")
    scrollToBottom()
  }

  function ensureCursor(previousIds) {
    if (!rows.length) { cursorMessageId = ""; return }
    if (indexOfId(cursorMessageId) >= 0) return
    var next = pinned ? "" : neighbourId(previousIds, cursorMessageId)
    cursorMessageId = next || String(rows[rows.length - 1].id || "")
  }

  function neighbourId(oldIds, id) {
    if (!Array.isArray(oldIds) || !id) return ""
    var at = oldIds.indexOf(id)
    if (at < 0) return ""
    var present = ({})
    for (var i = 0; i < ids.length; i++) present[ids[i]] = true
    for (var d = 1; d < oldIds.length; d++) {
      if (at + d < oldIds.length && present[oldIds[at + d]]) return oldIds[at + d]
      if (at - d >= 0 && present[oldIds[at - d]]) return oldIds[at - d]
    }
    return ""
  }

  readonly property color foreground: Color.foreground
  readonly property color muted: Api.secondaryColor(Color.muted, Color.foreground, Color.background)
  readonly property color accent: Color.accent
  readonly property string fontFamily: Style.font.family
  readonly property int groupWindowMs: 10 * 60 * 1000
  readonly property int historyThreshold: Style.space(120)
  readonly property int doubleTapMs: 400

  property var rows: []
  property var ids: []
  property bool pinned: true
  property bool adjusting: false
  property string anchorId: ""
  property real anchorOffset: 0
  property string historyRequestedFor: ""
  property string reachedId: ""
  property double lastGAt: 0

  readonly property int cursorIndex: indexOfId(cursorMessageId)
  readonly property string newestId: rows.length
    ? String(rows[rows.length - 1].id || "") : ""

  readonly property int unreadIndex: {
    if (!lastReadMessageId || !rows.length) return -1
    var k = indexOfId(lastReadMessageId)
    if (k < 0 || k + 1 >= rows.length) return -1
    for (var i = k + 1; i < rows.length; i++) {
      var author = rows[i] && rows[i].author ? rows[i].author : ({})
      if (String(author.id || "") !== selfId) return k + 1
    }
    return -1
  }

  readonly property var entries: {
    var map = ({})
    var list = rows
    var unreadAt = unreadIndex
    for (var i = 0; i < list.length; i++) {
      var message = list[i] || ({})
      var prev = i > 0 ? list[i - 1] : null
      var day = !prev || dayKey(prev) !== dayKey(message) ? dayLabel(message) : ""
      var unread = i === unreadAt
      var grouped = false
      if (prev && !day && !unread && !message.system && !prev.system && !message.reply_to) {
        var a = message.author ? String(message.author.id || "") : ""
        var b = prev.author ? String(prev.author.id || "") : ""
        var t1 = new Date(String(message.timestamp || "")).getTime()
        var t0 = new Date(String(prev.timestamp || "")).getTime()
        grouped = a !== "" && a === b && isFinite(t1) && isFinite(t0)
          && t1 - t0 >= 0 && t1 - t0 < groupWindowMs
      }
      map[String(message.id || "")] = { message: message, day: day, unread: unread, grouped: grouped }
    }
    return map
  }

  function indexOfId(id) {
    if (!id) return -1
    var list = rows
    for (var i = list.length - 1; i >= 0; i--)
      if (String(list[i].id || "") === id) return i
    return -1
  }

  function dayKey(message) {
    var d = new Date(String(message && message.timestamp || ""))
    return isNaN(d.getTime()) ? "" : Qt.formatDate(d, "yyyy-MM-dd")
  }

  function dayLabel(message) {
    var d = new Date(String(message && message.timestamp || ""))
    if (isNaN(d.getTime())) return ""
    var today = new Date()
    var yesterday = new Date(today.getTime() - 86400000)
    var key = Qt.formatDate(d, "yyyy-MM-dd")
    if (key === Qt.formatDate(today, "yyyy-MM-dd")) return "Today"
    if (key === Qt.formatDate(yesterday, "yyyy-MM-dd")) return "Yesterday"
    return Qt.formatDate(d, "dddd d MMMM yyyy")
  }

  function idRows(list, from, to) {
    var out = []
    for (var i = from; i < to; i++) out.push({ mid: list[i] })
    return out
  }

  function sameSlice(nextIds, offset, oldIds) {
    for (var i = 0; i < oldIds.length; i++)
      if (nextIds[offset + i] !== oldIds[i]) return false
    return true
  }

  function syncModel(next) {
    var nextIds = []
    for (var i = 0; i < next.length; i++) nextIds.push(String(next[i] && next[i].id || ""))
    var oldIds = ids
    captureAnchor()
    adjusting = true
    rows = next
    ids = nextIds
    var fast = oldIds.length > 0 && nextIds.length > 0
    var k = fast ? nextIds.indexOf(oldIds[0]) : -1
    if (fast && k >= 0 && k + oldIds.length <= nextIds.length && sameSlice(nextIds, k, oldIds)) {
      if (k > 0) idModel.insert(0, idRows(nextIds, 0, k))
      var tailFrom = k + oldIds.length
      if (tailFrom < nextIds.length) idModel.append(idRows(nextIds, tailFrom, nextIds.length))
    } else {
      idModel.clear()
      if (nextIds.length) idModel.append(idRows(nextIds, 0, nextIds.length))
      if (!nextIds.length) pinned = true
    }
    ensureCursor(oldIds)
    Qt.callLater(restoreAnchor)
  }

  function bodyOffset(item) {
    var delegate = item
    if (!delegate) return 0
    if (typeof delegate.forceLayout === "function") delegate.forceLayout()
    return isFinite(delegate.bodyY) ? delegate.bodyY : 0
  }

  function captureAnchor() {
    anchorId = ""
    if (pinned || !rows.length) return
    var index = nearestIndexAt(list.contentY + Style.spacing.xxs)
    var item = index >= 0 ? list.itemAtIndex(index) : null
    if (!item) return
    anchorId = String(rows[index].id || "")
    anchorOffset = list.contentY - item.y - bodyOffset(item)
  }

  function restoreAnchor() {
    applyAnchor()
    settleTimer.restart()
  }

  function applyAnchor() {
    if (!rows.length) return
    if (pinned) {
      list.positionViewAtEnd()
      return
    }
    var k = indexOfId(anchorId)
    if (k < 0) return
    list.positionViewAtIndex(k, ListView.Beginning)
    list.forceLayout()
    var item = list.itemAtIndex(k)
    if (item) list.contentY = item.y + bodyOffset(item) + anchorOffset
  }

  Timer {
    id: settleTimer
    interval: 50
    onTriggered: {
      timeline.applyAnchor()
      timeline.adjusting = false
      timeline.checkBottom()
    }
  }

  function setCursorIndex(index) {
    if (!rows.length) return
    index = Math.max(0, Math.min(index, rows.length - 1))
    cursorMessageId = String(rows[index].id || "")
    list.positionViewAtIndex(index, ListView.Contain)
  }

  function moveCursor(delta) {
    if (!rows.length) return
    var index = cursorIndex < 0 ? rows.length - 1 : cursorIndex
    var next = index + delta
    if (next < 0) {
      setCursorIndex(0)
      requestHistoryNow(false)
      return
    }
    setCursorIndex(next)
  }

  function goTop() {
    if (!rows.length) return
    setCursorIndex(0)
    list.positionViewAtBeginning()
    requestHistoryNow(false)
  }

  function nearestIndexAt(y) {
    var step = Style.spacing.lg
    var limit = y + list.height
    for (var probe = y; probe <= limit; probe += step) {
      var index = list.indexAt(Style.spacing.xxs, probe)
      if (index >= 0) return index
    }
    return -1
  }

  function pageMove(direction) {
    if (!rows.length) return
    var page = Math.max(Style.spacing.popupRowHeight, list.height - Style.spacing.popupRowHeight)
    var minY = list.originY
    var maxY = Math.max(minY, list.contentHeight - list.height + minY)
    var target = Math.max(minY, Math.min(maxY, list.contentY + direction * page))
    adjusting = true
    list.contentY = target
    adjusting = false
    pinned = list.atYEnd
    var probeY = direction < 0 ? list.contentY + Style.spacing.xxs
      : list.contentY + list.height - Style.spacing.xxs
    var index = nearestIndexAt(probeY)
    if (index < 0) index = direction < 0 ? 0 : rows.length - 1
    cursorMessageId = String(rows[index].id || "")
    if (direction < 0 && list.contentY <= minY) requestHistoryNow(false)
    checkBottom()
  }

  function requestHistoryNow(dedupe) {
    if (loading || !hasMore || !rows.length) return
    var beforeId = String(rows[0].id || "")
    if (!beforeId) return
    if (dedupe && beforeId === historyRequestedFor) return
    historyRequestedFor = beforeId
    requestHistory(beforeId)
  }

  function maybeRequestHistoryOnScroll() {
    if (adjusting || !hasMore || loading || !rows.length) return
    if (list.contentY - list.originY <= historyThreshold) requestHistoryNow(true)
  }

  function checkBottom() {
    if (adjusting || !viewing || !rows.length || !list.atYEnd) return
    if (newestId === reachedId) return
    reachedId = newestId
    reachedBottom()
  }

  function stickIfPinned() {
    if (!pinned || adjusting) return
    adjusting = true
    list.positionViewAtEnd()
    adjusting = false
    checkBottom()
  }

  function copyCursorMessage() {
    var index = cursorIndex
    if (index < 0) return
    var text = Markdown.plainText(rows[index].content, ctx)
    var attachments = Array.isArray(rows[index].attachments) ? rows[index].attachments : []
    for (var i = 0; i < attachments.length; i++)
      if (attachments[i] && attachments[i].url) text += (text ? "\n" : "") + String(attachments[i].url)
    if (!text) return
    copyRequested(text)
    copied()
  }

  function noteSelection(item) {
    if (selectionOwner === item) return
    if (selectionOwner) selectionOwner.clearSelection()
    selectionOwner = item
  }

  function releaseSelection(item) {
    if (selectionOwner === item) selectionOwner = null
  }

  function clearSelection() {
    if (selectionOwner) selectionOwner.clearSelection()
    selectionOwner = null
  }

  function copySelection() {
    if (!hasSelection) return
    var text = String(selectionOwner.selection).replace(/[\u2028\u2029]/g, "\n")
    if (!text) return
    copyRequested(text)
    copied()
  }

  function copyLink(url) {
    var link = String(url || "")
    if (!link) return
    copyRequested(link)
    linkCopied()
  }

  function copyCursorLink() {
    if (cursorIndex < 0) return
    copyLink(Markdown.firstLink(rows[cursorIndex]))
  }

  function openCursorLink() {
    var index = cursorIndex
    if (index < 0) return
    var url = Markdown.firstLink(rows[index])
    if (!url) return
    openLink(url)
  }

  function hasCoveredSpoiler(row) {
    if (!row || revealed[String(row.id || "")]) return false
    var attachments = Array.isArray(row.attachments) ? row.attachments : []
    for (var i = 0; i < attachments.length; i++)
      if (attachments[i] && attachments[i].spoiler) return true
    return false
  }

  function reveal(messageId) {
    var id = String(messageId || "")
    if (!id || revealed[id]) return
    var next = ({})
    for (var key in revealed) next[key] = true
    next[id] = true
    revealed = next
  }

  function activateCursor() {
    if (cursorIndex < 0) return
    if (hasCoveredSpoiler(rows[cursorIndex])) reveal(cursorMessageId)
    else activateMessage(cursorMessageId)
  }

  function isOwnRow(index) {
    var author = index >= 0 && rows[index] && rows[index].author ? rows[index].author : null
    return !!(author && selfId && String(author.id || "") === selfId && !rows[index].pending)
  }

  function disarmDelete() {
    disarmTimer.stop()
    if (armedDeleteId) armedDeleteId = ""
  }

  function requestDelete() {
    var index = cursorIndex
    if (index < 0 || !isOwnRow(index)) return
    var id = cursorMessageId
    if (armedDeleteId === id) {
      disarmDelete()
      deleteRequested(id)
      return
    }
    armedDeleteId = id
    disarmTimer.restart()
  }

  function handleKey(event) {
    var key = event.key
    var text = event.text
    var alt = (event.modifiers & Qt.AltModifier) !== 0
    var now = Date.now()
    var wasG = now - lastGAt <= doubleTapMs
    lastGAt = 0
    var armed = armedDeleteId !== ""
    if (armed && text !== "D" && !(actions.activeFocus && (key === Qt.Key_Return || key === Qt.Key_Enter || key === Qt.Key_Space))) disarmDelete()
    if (actions.activeFocus && (key === Qt.Key_Return || key === Qt.Key_Enter || key === Qt.Key_Space)) return
    if (key === Qt.Key_Tab || key === Qt.Key_Backtab) {
      var buttons = actions.children.filter(function(item) { return item.visible && item.enabled && item.focusable })
      var index = -1
      for (var i = 0; i < buttons.length; i++) if (buttons[i].activeFocus) index = i
      var reverse = key === Qt.Key_Backtab || (event.modifiers & Qt.ShiftModifier) !== 0
      if (index < 0 && !reverse && buttons.length) buttons[0].forceActiveFocus()
      else if (reverse && index === 0) timeline.forceActiveFocus()
      else if (index >= 0 && index + (reverse ? -1 : 1) >= 0 && index + (reverse ? -1 : 1) < buttons.length)
        buttons[index + (reverse ? -1 : 1)].forceActiveFocus()
      else cycleFocus(reverse ? -1 : 1)
      event.accepted = true
      return
    }

    if ((event.modifiers & Qt.ControlModifier) !== 0) {
      if (key !== Qt.Key_C || !hasSelection) return
      copySelection()
      event.accepted = true
      return
    }

    if (alt && key === Qt.Key_H) moveZone("left")
    else if (alt && key === Qt.Key_L) moveZone("right")
    else if (alt) return
    else if (key === Qt.Key_Escape) {
      if (!armed) {
        if (hasSelection) clearSelection()
        else escapeRequested()
      }
    }
    else if (key === Qt.Key_Down || text === "j") moveCursor(1)
    else if (key === Qt.Key_Up || text === "k") moveCursor(-1)
    else if (key === Qt.Key_PageUp) pageMove(-1)
    else if (key === Qt.Key_PageDown) pageMove(1)
    else if (key === Qt.Key_Home) goTop()
    else if (key === Qt.Key_End || text === "G") focusNewest()
    else if (text === "g") { if (wasG) goTop(); else lastGAt = now }
    else if (key === Qt.Key_Return || key === Qt.Key_Enter) activateCursor()
    else if (text === "Y") copyCursorMessage()
    else if (text === "O") openCursorLink()
    else if (text === "L") copyCursorLink()
    else if (text === "R") { if (cursorIndex >= 0 && !rows[cursorIndex].pending) replyRequested(cursorMessageId) }
    else if (text === "D") requestDelete()
    else if (text === "E") { if (cursorIndex >= 0) reactRequested(cursorMessageId) }
    else return
    event.accepted = true
  }

  Keys.priority: Keys.BeforeItem
  Keys.onPressed: function(event) { handleKey(event) }

  onMessagesChanged: syncModel(Array.isArray(messages) ? messages : [])
  Component.onCompleted: {
    if (!ids.length) syncModel(Array.isArray(messages) ? messages : [])
    ensureCursor()
  }
  onActiveChanged: {
    if (active) ensureCursor()
    checkBottom()
    if (!active) { disarmDelete(); clearSelection() }
  }
  onViewingChanged: checkBottom()
  onChannelIdChanged: {
    pinned = true
    historyRequestedFor = ""
    reachedId = ""
    cursorMessageId = ""
    lastGAt = 0
    revealed = ({})
    disarmDelete()
    clearSelection()
  }
  onCursorMessageIdChanged: if (armedDeleteId && armedDeleteId !== cursorMessageId) disarmDelete()

  Timer {
    id: disarmTimer
    interval: timeline.deleteArmMs
    onTriggered: timeline.disarmDelete()
  }

  ListModel {
    id: idModel
  }

  BorderSurface {
    id: frame
    anchors.fill: parent
    radius: Style.cornerRadius
    color: Color.popups.background
    borderSpec: timeline.active
      ? Border.controlSpec("focus", timeline.foreground, timeline.accent)
      : Border.flat(Color.popups.border, Math.max(1, Style.normalBorderWidth))
    padding: Style.spacing.sm

    ListView {
      id: list
      anchors.fill: parent
      anchors.margins: frame.padding + Style.normalBorderWidth
      anchors.bottomMargin: frame.padding + Style.normalBorderWidth + actions.height + Style.spacing.xs
      clip: true
      reuseItems: false
      cacheBuffer: Style.space(150)
      boundsBehavior: Flickable.StopAtBounds
      spacing: Style.spacing.xxs
      focus: false
      keyNavigationEnabled: false
      model: idModel
      ScrollBar.vertical: ScrollBar { policy: ScrollBar.AsNeeded }

      onContentYChanged: {
        if (timeline.adjusting) return
        timeline.pinned = list.atYEnd
        timeline.maybeRequestHistoryOnScroll()
      }
      onContentHeightChanged: if (timeline.pinned) Qt.callLater(timeline.stickIfPinned)
      onHeightChanged: if (timeline.pinned) Qt.callLater(timeline.stickIfPinned)
      onAtYEndChanged: timeline.checkBottom()

      header: Item {
        width: list.width
        height: Style.spacing.controlHeight

        Text {
          anchors.centerIn: parent
          visible: timeline.loading || (!timeline.hasMore && timeline.rows.length > 0)
          text: timeline.loading ? "Loading history" : "Beginning of conversation"
          color: timeline.muted
          font.family: timeline.fontFamily
          font.pixelSize: Style.font.caption
        }
      }

      delegate: Column {
        id: row
        required property string mid
        readonly property var entry: timeline.entries[mid] || ({})
        readonly property var message: entry.message || ({})
        readonly property string day: String(entry.day || "")
        readonly property bool unread: !!entry.unread
        readonly property bool grouped: !!entry.grouped
        readonly property real bodyY: messageRow.y
        width: list.width
        spacing: Style.spacing.xxs

        Item {
          width: parent.width
          height: visible ? Style.spacing.controlHeight : 0
          visible: row.day !== ""

          Rectangle {
            anchors.left: parent.left
            anchors.right: dayText.left
            anchors.rightMargin: Style.spacing.controlGap
            anchors.verticalCenter: parent.verticalCenter
            height: Style.spacing.hairline
            color: Util.alpha(timeline.foreground, 0.2)
          }
          Text {
            id: dayText
            anchors.centerIn: parent
            text: row.day
            color: timeline.muted
            font.family: timeline.fontFamily
            font.pixelSize: Style.font.caption
            font.bold: true
          }
          Rectangle {
            anchors.left: dayText.right
            anchors.leftMargin: Style.spacing.controlGap
            anchors.right: parent.right
            anchors.verticalCenter: parent.verticalCenter
            height: Style.spacing.hairline
            color: Util.alpha(timeline.foreground, 0.2)
          }
        }

        Item {
          width: parent.width
          height: visible ? Style.spacing.lg + Style.spacing.xs : 0
          visible: row.unread

          Rectangle {
            anchors.left: parent.left
            anchors.right: unreadText.left
            anchors.rightMargin: Style.spacing.controlGap
            anchors.verticalCenter: parent.verticalCenter
            height: Style.spacing.hairline
            color: Color.urgent
          }
          Text {
            id: unreadText
            anchors.right: parent.right
            anchors.rightMargin: Style.spacing.rowPaddingX
            anchors.verticalCenter: parent.verticalCenter
            text: "NEW"
            color: Color.urgent
            font.family: timeline.fontFamily
            font.pixelSize: Style.font.caption
            font.bold: true
          }
        }

        MessageRow {
          id: messageRow
          width: parent.width
          message: row.message
          grouped: row.grouped
          selfId: timeline.selfId
          ctx: timeline.ctx
          cursor: timeline.active && row.mid !== "" && row.mid === timeline.cursorMessageId
          armedDelete: row.mid !== "" && row.mid === timeline.armedDeleteId
          spoilersRevealed: !!timeline.revealed[row.mid]
          onClicked: {
            timeline.cursorMessageId = row.mid
            timeline.forceActiveFocus()
          }
          onRevealRequested: {
            timeline.cursorMessageId = row.mid
            timeline.reveal(row.mid)
          }
          onLinkActivated: function(url) { timeline.openLink(url) }
          onReactionClicked: function(emoji) {
            timeline.cursorMessageId = row.mid
            timeline.reactionToggled(row.mid, emoji)
          }
          onCopyLinkRequested: function(url) {
            timeline.cursorMessageId = row.mid
            timeline.copyLink(url)
          }
          onSelected: {
            timeline.noteSelection(messageRow)
            timeline.cursorMessageId = row.mid
            timeline.forceActiveFocus()
          }
          Component.onDestruction: timeline.releaseSelection(messageRow)
        }
      }
    }

    Flow {
      id: actions
      objectName: "message-actions"
      anchors.left: parent.left
      anchors.right: parent.right
      anchors.bottom: parent.bottom
      anchors.margins: frame.padding + Style.normalBorderWidth
      spacing: Style.spacing.xs
      visible: timeline.rows.length > 0
      enabled: !timeline.loading
      Keys.onEscapePressed: { timeline.disarmDelete(); timeline.forceActiveFocus() }
      readonly property bool available: timeline.cursorIndex >= 0 && !timeline.rows[timeline.cursorIndex].pending
      Button {
        text: "Reply"
        enabled: actions.available
        focusable: true
        tooltipText: "Reply to selected message (R)"
        onClicked: timeline.replyRequested(timeline.cursorMessageId)
      }
      Button {
        text: "React"
        enabled: actions.available
        focusable: true
        tooltipText: "React to selected message (E)"
        onClicked: timeline.reactRequested(timeline.cursorMessageId)
      }
      Button {
        text: "Copy"
        enabled: actions.available
        focusable: true
        tooltipText: "Copy selected message (Y)"
        onClicked: timeline.copyCursorMessage()
      }
      Button {
        text: "Edit"
        visible: timeline.isOwnRow(timeline.cursorIndex)
        focusable: true
        onClicked: timeline.editRequested(timeline.cursorMessageId)
      }
      Button {
        text: timeline.armedDeleteId ? "Confirm delete" : "Delete"
        visible: timeline.isOwnRow(timeline.cursorIndex)
        focusable: true
        foreground: Color.urgent
        tooltipText: "Click twice within 3 seconds to delete selected message"
        onClicked: timeline.requestDelete()
      }
    }

    Text {
      anchors.centerIn: parent
      visible: !timeline.rows.length && !timeline.loading
      text: timeline.channelId ? "No messages yet." : "Pick a channel."
      color: timeline.muted
      font.family: timeline.fontFamily
      font.pixelSize: Style.font.body
    }
  }
}
