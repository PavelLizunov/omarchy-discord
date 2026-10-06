pragma ComponentBehavior: Bound
import QtQuick
import QtQuick.Controls
import Quickshell
import Quickshell.Io
import qs.Commons
import qs.Ui

import "components" as Components
import "components/harness/Fixtures.js" as Fixtures

ShellRoot {
  id: harness

  property var messages: Fixtures.initial()
  property bool hasMore: true
  property bool loading: false
  property bool active: true
  property string lastRead: messages.length > 10 ? String(messages[messages.length - 8].id) : ""
  property var log: []
  property int nextId: 2000

  readonly property var ctx: Fixtures.ctx({
    mentionColor: Color.accent,
    mentionBg: Util.alpha(Color.accent, 0.18),
    linkColor: Color.accent,
    codeBg: Util.alpha(Color.foreground, 0.08),
    spoilerColor: Color.muted,
    mutedColor: Color.muted,
    monoFamily: Style.font.family,
    fontSize: Style.font.body
  })

  function note(line) {
    var next = log.slice()
    next.push(line)
    if (next.length > 6) next.shift()
    log = next
    console.log("harness: " + line)
  }

  function prepend(count) {
    if (!messages.length) return
    var first = messages[0]
    var olderRows = Fixtures.older(Number(first.id), new Date(first.timestamp).getTime(), count || 30)
    messages = olderRows.concat(messages)
    note("prepended " + olderRows.length + " (first id now " + messages[0].id + ")")
  }

  function append() {
    var last = messages[messages.length - 1]
    var ms = Math.max(Date.now(), new Date(last.timestamp).getTime() + 1000)
    var who = (nextId % 2) ? "200" : "300"
    var row = Fixtures.makeMessage(nextId++, who, "new message #" + nextId + " <@100> just landed", ms, {})
    messages = messages.concat([row])
    note("appended " + row.id)
  }

  function state() {
    return JSON.stringify({
      count: messages.length,
      cursor: timeline.cursorMessageId,
      cursorIndex: timeline.cursorIndex,
      pinned: timeline.pinned,
      firstId: messages.length ? messages[0].id : "",
      newestId: timeline.newestId,
      unreadIndex: timeline.unreadIndex,
      loading: loading,
      hasMore: hasMore,
      active: active,
      focus: timeline.activeFocus
    })
  }

  Timer {
    id: historyTimer
    interval: 600
    onTriggered: {
      harness.prepend(30)
      harness.loading = false
      if (harness.messages.length > 200) harness.hasMore = false
    }
  }

  IpcHandler {
    target: "harness"
    function prepend(): void { harness.prepend(30) }
    function append(): void { harness.append() }
    function state(): string { return harness.state() }
    function toggleActive(): void { harness.active = !harness.active }
    function markRead(): void { harness.lastRead = harness.timelineNewest() }
    function focus(): void { timeline.forceActiveFocus() }
    function shot(path: string): void { harness.shot(path) }
    function key(name: string): string { return harness.pressKey(name) }
  }

  function timelineNewest() { return timeline.newestId }

  function shot(path) {
    scope.grabToImage(function(result) {
      var ok = result.saveToFile(path)
      harness.note("shot " + path + (ok ? "" : " FAILED"))
    })
  }

  function pressKey(name) {
    var map = {
      Escape: Qt.Key_Escape, Return: Qt.Key_Return, Home: Qt.Key_Home, End: Qt.Key_End,
      PgUp: Qt.Key_PageUp, PgDn: Qt.Key_PageDown, Up: Qt.Key_Up, Down: Qt.Key_Down,
      AltH: Qt.Key_H, AltL: Qt.Key_L
    }
    var event = {
      key: map[name] !== undefined ? map[name] : 0,
      text: map[name] !== undefined ? "" : name,
      modifiers: name.indexOf("Alt") === 0 ? Qt.AltModifier : Qt.NoModifier,
      accepted: false
    }
    timeline.handleKey(event)
    return event.accepted ? "accepted" : "ignored"
  }

  FloatingWindow {
    id: window
    title: "Timeline harness"
    color: Color.background
    implicitWidth: 760
    implicitHeight: 620
    minimumSize: Qt.size(420, 320)

    FocusScope {
      id: scope
      anchors.fill: parent
      anchors.margins: Style.spacing.panelPadding
      focus: true

      Column {
        anchors.fill: parent
        spacing: Style.spacing.panelGap

        Row {
          spacing: Style.spacing.controlGap
          Button { text: "Prepend 30"; foreground: Color.foreground; onClicked: harness.prepend(30) }
          Button { text: "Append"; foreground: Color.foreground; onClicked: harness.append() }
          Button { text: harness.active ? "Deactivate" : "Activate"; foreground: Color.foreground; onClicked: harness.active = !harness.active }
          Button { text: "Focus timeline"; foreground: Color.foreground; onClicked: timeline.forceActiveFocus() }
        }

        Components.Timeline {
          id: timeline
          width: parent.width
          height: parent.height - Style.spacing.controlHeight - logText.height - parent.spacing * 2
          focus: true
          messages: harness.messages
          hasMore: harness.hasMore
          loading: harness.loading
          channelId: "9000"
          selfId: "100"
          lastReadMessageId: harness.lastRead
          active: harness.active
          ctx: harness.ctx

          onRequestHistory: function(beforeId) {
            harness.note("requestHistory(" + beforeId + ")")
            harness.loading = true
            historyTimer.restart()
          }
          onEscapeRequested: harness.note("escapeRequested()")
          onMoveZone: function(direction) { harness.note("moveZone(" + direction + ")") }
          onOpenLink: function(url) { harness.note("openLink(" + url + ")") }
          onCopied: harness.note("copied() clipboard=" + JSON.stringify(String(Quickshell.clipboardText).slice(0, 40)))
          onActivateMessage: function(id) { harness.note("activateMessage(" + id + ")") }
          onReachedBottom: harness.note("reachedBottom() newest=" + timeline.newestId)
        }

        Text {
          id: logText
          width: parent.width
          text: harness.log.join("\n") || "signals appear here"
          color: Color.muted
          font.family: Style.font.family
          font.pixelSize: Style.font.caption
          wrapMode: Text.NoWrap
          elide: Text.ElideRight
          height: Style.font.caption * 1.4 * 6
        }
      }
    }

    Component.onCompleted: Qt.callLater(function() { timeline.forceActiveFocus() })
  }
}
