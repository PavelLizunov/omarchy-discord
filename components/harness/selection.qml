pragma ComponentBehavior: Bound
import QtQuick
import Quickshell
import QtTest
import qs.Commons
import qs.Ui

import "components" as Components
import "components/harness/Fixtures.js" as Fixtures

// Offscreen contract harness for selectable message text, the Ctrl+C copy and
// the hovered-link copy chip. `components/harness/run-selection.sh` builds a
// scratch config root of symlinks and runs this with QT_QPA_PLATFORM=offscreen;
// every pointer event is synthesized by QtTest inside this window, so nothing
// is ever injected into the Wayland session.
//
// What it pins down:
//   * a fenced code block with a 200-character URL wraps inside the column
//     instead of running off the right edge (Markdown.js blockCodeStyle);
//   * the body TextEdit never takes activeFocus, so the roving cursor and the
//     zone keys survive a drag-selection;
//   * one selection at a time, Ctrl+C copies it with real newlines, Y still
//     copies the whole message, Esc peels the selection before the zone;
//   * hovering a link offers the copy chip, and the chip copies the URL.
ShellRoot {
  id: harness

  property int failures: 0
  property int checks: 0
  property int escapes: 0
  property int copies: 0
  property int linkCopies: 0

  readonly property string longUrl: "https://login.composio.dev/oauth2/authorize?response_type=code&client_id=01234567-89ab-cdef-0123-456789abcdef&redirect_uri=https%3A%2F%2Fbackend.composio.dev%2Fapi%2Fv3%2Ftoolkits%2Fauth%2Fcallback&state=zzzz0000zzzz0000zzzz0000zzzz0000"

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

  // Five short rows so every delegate is laid out at once and no scrolling is
  // needed to reach one with the mouse.
  property var messages: {
    var t = Date.parse("2026-08-20T10:00:00.000Z")
    return [
      Fixtures.makeMessage(1, "200", "morning all", t, {}),
      Fixtures.makeMessage(2, "300", "```\n" + harness.longUrl + "\n```", t + 60000, {}),
      Fixtures.makeMessage(3, "200", "the link is " + harness.longUrl + " if you need it", t + 120000, {}),
      Fixtures.makeMessage(4, "300", "first line\nsecond line\nthird line", t + 180000, {}),
      Fixtures.makeMessage(5, "200", "last", t + 240000, {})
    ]
  }

  function check(name, actual, expected) {
    checks++
    var a = JSON.stringify(actual)
    var e = JSON.stringify(expected)
    if (a === e) { console.log("  ok   " + name); return }
    failures++
    console.log("  FAIL " + name + ": expected " + e + ", got " + a)
  }

  function press(name, modifiers) {
    var keys = {
      Escape: Qt.Key_Escape, Return: Qt.Key_Return, Ctrl_C: Qt.Key_C,
      Up: Qt.Key_Up, Down: Qt.Key_Down
    }
    var event = {
      key: keys[name] !== undefined ? keys[name] : 0,
      text: keys[name] !== undefined ? "" : name,
      modifiers: modifiers || Qt.NoModifier,
      accepted: false
    }
    timeline.handleKey(event)
    return event.accepted
  }

  // Depth-first walk over children (and a Flickable's contentItem) for the
  // first object the predicate accepts.
  function find(item, pred) {
    if (!item) return null
    if (pred(item)) return item
    var kids = item.children || []
    for (var i = 0; i < kids.length; i++) {
      var hit = find(kids[i], pred)
      if (hit) return hit
    }
    return null
  }

  function collect(item, pred, out) {
    if (!item) return out
    if (pred(item)) out.push(item)
    var kids = item.children || []
    for (var i = 0; i < kids.length; i++) collect(kids[i], pred, out)
    return out
  }

  function isMessageRow(item) {
    return item.hoverLink !== undefined && item.message !== undefined
  }

  function isBody(item) {
    return item.selectByMouse !== undefined && item.persistentSelection !== undefined
  }

  function rowFor(id) {
    var all = collect(timeline, isMessageRow, [])
    for (var i = 0; i < all.length; i++)
      if (String(all[i].message.id || "") === String(id)) return all[i]
    return null
  }

  function bodyOf(row) { return find(row, isBody) }

  // First point inside `item` that reports a link, scanning its laid-out box.
  function linkPoint(item) {
    for (var y = 2; y < item.height; y += 4)
      for (var x = 2; x < item.width; x += 4)
        if (item.linkAt(x, y)) return { x: x, y: y, url: item.linkAt(x, y) }
    return null
  }

  function clipboard() { return String(Quickshell.clipboardText) }

  FloatingWindow {
    id: window
    title: "Selection harness"
    color: Color.background
    implicitWidth: 720
    implicitHeight: 620

    FocusScope {
      id: scope
      anchors.fill: parent
      anchors.margins: Style.spacing.panelPadding
      focus: true

      Components.Timeline {
        id: timeline
        anchors.fill: parent
        focus: true
        messages: harness.messages
        hasMore: false
        loading: false
        channelId: "9000"
        selfId: "100"
        active: true
        ctx: harness.ctx

        onEscapeRequested: harness.escapes++
        onCopied: harness.copies++
        onLinkCopied: harness.linkCopies++
      }
    }
  }

  TestCase {
    id: tc
    when: false
    name: "selection"

    function runWrapping() {
      console.log("wrapping — no message body runs off the column")
      var bodies = harness.collect(timeline, harness.isBody, [])
      harness.check("every row has a body", bodies.length, 5)
      var over = 0
      for (var i = 0; i < bodies.length; i++)
        if (bodies[i].contentWidth > bodies[i].width + 0.5) over++
      harness.check("no body overflows its width", over, 0)

      var fence = harness.bodyOf(harness.rowFor(2))
      harness.check("the fenced 200-character URL wraps",
        fence.contentWidth <= fence.width + 0.5, true)
      harness.check("and wraps onto several lines",
        fence.contentHeight > Style.font.body * 2, true)

      var prose = harness.bodyOf(harness.rowFor(3))
      harness.check("a bare long URL in prose wraps too",
        prose.contentWidth <= prose.width + 0.5, true)
    }

    function runFocus() {
      console.log("selection — the roving cursor survives a drag")
      timeline.cursorMessageId = "5"
      var row = harness.rowFor(1)
      var body = harness.bodyOf(row)
      mousePress(body, 3, 3)
      mouseMove(body, Math.round(body.contentWidth) - 2, 3)
      mouseRelease(body, Math.round(body.contentWidth) - 2, 3)
      harness.check("the drag selected text", row.hasSelection, true)
      harness.check("the body never takes activeFocus", body.activeFocus, false)
      harness.check("the timeline keeps activeFocus", timeline.activeFocus, true)
      harness.check("the timeline owns the selection", timeline.selectionOwner === row, true)
      harness.check("selecting claims the cursor row", timeline.cursorMessageId, "1")

      harness.check("j still moves the cursor", harness.press("j"), true)
      harness.check("and moved it", timeline.cursorMessageId, "2")
      harness.press("k")
      harness.check("k moves it back", timeline.cursorMessageId, "1")
      harness.check("the selection survived j/k", timeline.hasSelection, true)
    }

    function runCopy() {
      console.log("selection — Ctrl+C, Y and one owner at a time")
      var row = harness.rowFor(1)
      var selected = row.selection
      Quickshell.clipboardText = ""
      var before = harness.copies
      harness.check("Ctrl+C is consumed", harness.press("Ctrl_C", Qt.ControlModifier), true)
      harness.check("Ctrl+C copied the selection", harness.clipboard(), String(selected))
      harness.check("and reported it through copied()", harness.copies - before, 1)

      // Y still copies the whole message, not the selection.
      Quickshell.clipboardText = ""
      timeline.cursorMessageId = "5"
      harness.press("Y")
      harness.check("Y still copies the whole cursor message", harness.clipboard(), "last")
      timeline.cursorMessageId = "1"

      // A selection in another row drops the first one.
      var other = harness.rowFor(4)
      harness.bodyOf(other).selectAll()
      harness.check("the new row owns the selection", timeline.selectionOwner === other, true)
      harness.check("the old row lost its selection", row.hasSelection, false)

      // Qt hands rich text back with U+2028 / U+2029 separators, never "\n".
      var raw = String(other.selection)
      harness.check("the raw selection carries Qt separators",
        raw.indexOf(String.fromCharCode(8232)) >= 0 || raw.indexOf(String.fromCharCode(8233)) >= 0, true)
      Quickshell.clipboardText = ""
      harness.press("Ctrl_C", Qt.ControlModifier)
      harness.check("the clipboard gets real newlines", harness.clipboard(), "first line\nsecond line\nthird line")

      // Ctrl+C with nothing selected falls through to the panel.
      timeline.clearSelection()
      harness.check("Ctrl+C without a selection is not consumed",
        harness.press("Ctrl_C", Qt.ControlModifier), false)
    }

    function runEscape() {
      console.log("selection — Esc peels the selection before the zone")
      var row = harness.rowFor(4)
      harness.bodyOf(row).selectAll()
      var before = harness.escapes
      harness.check("the first Esc is consumed", harness.press("Escape"), true)
      harness.check("the first Esc cleared the selection", timeline.hasSelection, false)
      harness.check("and did not leave the zone", harness.escapes - before, 0)
      harness.press("Escape")
      harness.check("the second Esc leaves the zone", harness.escapes - before, 1)
    }

    function runClicksAndClears() {
      console.log("selection — a click moves the cursor, leaving clears")
      timeline.cursorMessageId = "1"
      var body = harness.bodyOf(harness.rowFor(3))
      mouseClick(body, 3, 3)
      harness.check("clicking message text moves the roving cursor", timeline.cursorMessageId, "3")
      harness.check("a plain click leaves nothing selected", timeline.hasSelection, false)

      harness.bodyOf(harness.rowFor(1)).selectAll()
      timeline.active = false
      harness.check("leaving the zone clears the selection", timeline.hasSelection, false)
      timeline.active = true

      harness.bodyOf(harness.rowFor(1)).selectAll()
      timeline.channelId = "9001"
      harness.check("changing channel clears the selection", timeline.hasSelection, false)
      timeline.channelId = "9000"
    }

    function runHoverChip() {
      console.log("hovered link — the copy chip")
      var row = harness.rowFor(3)
      var body = harness.bodyOf(row)
      var hit = harness.linkPoint(body)
      harness.check("the row's URL is a link", hit !== null && hit.url === harness.longUrl, true)
      mouseMove(body, hit.x, hit.y)
      harness.check("hovering a link offers it", row.hoverLink, harness.longUrl)

      var chip = harness.find(row, function(i) { return i.text === "Copy link" })
      harness.check("the chip is shown", chip !== null && chip.visible, true)

      Quickshell.clipboardText = ""
      var before = harness.linkCopies
      mouseClick(chip, 2, 2)
      harness.check("the chip copies the URL", harness.clipboard(), harness.longUrl)
      harness.check("and reports it through linkCopied()", harness.linkCopies - before, 1)
      harness.check("copying a link moves the cursor to its row", timeline.cursorMessageId, "3")

      // The chip covers the text it is offered for, so hoveredLink goes empty
      // the moment it appears: the offer must not clear synchronously.
      mouseMove(body, 1, Math.round(body.height) - 1)
      harness.check("the offer survives the pointer leaving the link", row.hoverLink, harness.longUrl)

      // L copies the cursor row's first link, without the chip.
      timeline.clearSelection()
      Quickshell.clipboardText = ""
      before = harness.linkCopies
      timeline.cursorMessageId = "3"
      harness.check("L is consumed", harness.press("L"), true)
      harness.check("L copies the row's first link", harness.clipboard(), harness.longUrl)
      harness.check("and reports it too", harness.linkCopies - before, 1)
    }

    function go() {
      runWrapping()
      runFocus()
      runCopy()
      runEscape()
      runClicksAndClears()
      runHoverChip()
      console.log(harness.failures === 0
        ? "HARNESS PASS (" + harness.checks + " checks)"
        : "HARNESS FAIL (" + harness.failures + "/" + harness.checks + " checks)")
      Qt.exit(harness.failures === 0 ? 0 : 1)
    }
  }

  // Let the window map and every delegate lay out before asserting.
  Timer {
    interval: 500
    running: true
    onTriggered: tc.go()
  }
}
