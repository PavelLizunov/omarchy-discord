pragma ComponentBehavior: Bound
import QtQuick
import QtQuick.Controls
import qs.Commons
import qs.Ui

import "../Api.js" as Api

import "../Emoji.js" as Emoji
import "../Keymap.js" as Keymap

// E on a timeline row: a modal emoji picker inside the panel window. The
// search field filters by name; the grid is navigated with the arrows (or
// Ctrl+h/j/k/l, since plain letters type into the filter), Enter picks,
// Esc clears the filter then closes. Sections, in order: the message's own
// reactions ("Toggle" — picking one you already reacted with removes it),
// frequently used (persisted by the service), server emoji (list_emoji, one
// section per guild, the selected guild first; rendered through the media
// cache), then the unicode catalogue.
// Emits picked(emoji) in wire form; the caller decides react vs unreact.
FocusScope {
  id: root

  property bool shown: false
  property var service: null
  property string messageId: ""
  property var reactions: []
  readonly property var frequent: service ? service.frequentEmoji : []
  readonly property var serverEmoji: service ? service.serverEmoji : []
  readonly property var catalog: service && service.emojiCatalog.length ? service.emojiCatalog : Emoji.FALLBACK
  property string query: ""
  property int cursor: 0
  readonly property int gridLimit: 400

  signal picked(string emoji)
  signal closeRequested()

  // Empty while hidden: the grid's Repeater would otherwise instantiate every
  // cell (and request every custom emoji image) the moment list_emoji lands.
  readonly property var sections: shown
    ? Emoji.sections(reactions, frequent, serverEmoji, catalog, query, gridLimit, service ? service.selectedGuildId : "")
    : []
  readonly property var flat: Emoji.flatten(sections)
  readonly property color foreground: Color.popups.text
  readonly property color muted: Api.secondaryColor(Color.muted, Color.foreground, Color.background)
  readonly property string fontFamily: Style.font.family
  readonly property int cellSize: Math.max(Style.space(40), Style.font.heading + Style.spacing.md)
  readonly property int columns: Math.max(1, Math.floor(gridWidth / cellSize))
  readonly property real gridWidth: Math.max(cellSize, column.width - Style.spacing.rowPaddingX * 2)
  readonly property var cursorCell: cursor >= 0 && cursor < flat.length ? flat[cursor].cell : null
  readonly property string cursorLabel: cursorCell
    ? (cursorCell.toggle ? (cursorCell.me ? "Remove your " : "Add ") + cursorCell.label : cursorCell.label) : ""

  visible: shown
  enabled: shown

  function show(message) {
    messageId = message ? String(message.id || "") : ""
    reactions = message && Array.isArray(message.reactions) ? message.reactions : []
    query = ""
    searchField.text = ""
    cursor = 0
    shown = true
    if (service) service.ensureEmojiCatalog()
    Qt.callLater(function() { if (root.shown) searchField.forceActiveFocus() })
  }

  function hide() {
    shown = false
    closeRequested()
  }

  function setQuery(text) {
    var next = String(text || "")
    if (next === query) return
    query = next
    cursor = 0
  }

  function move(dx, dy) {
    var next = Emoji.move(sections, flat, cursor, dx, dy, columns)
    if (next >= 0) cursor = next
    ensureVisible()
  }

  // Keep the cursor cell inside the grid viewport.
  function ensureVisible() {
    if (cursor < 0 || cursor >= flat.length) return
    var pos = flat[cursor]
    var sectionItem = sectionRepeater.itemAt(pos.section)
    if (!sectionItem) return
    var top = sectionItem.y + sectionItem.gridY + Math.floor(pos.index / columns) * cellSize
    var bottom = top + cellSize
    if (top < flick.contentY) flick.contentY = top
    else if (bottom > flick.contentY + flick.height) flick.contentY = bottom - flick.height
  }

  function pick(index) {
    var item = flat[index]
    if (!item) return
    var emoji = String(item.cell.emoji || "")
    hide()
    if (emoji) picked(emoji)
  }

  function handleKey(event) {
    var key = event.key
    var ctrl = (event.modifiers & Qt.ControlModifier) !== 0
    var shift = (event.modifiers & Qt.ShiftModifier) !== 0
    if (key === Qt.Key_Escape) {
      if (query) { searchField.text = ""; setQuery("") }
      else hide()
    }
    else if (key === Qt.Key_Down || (ctrl && key === Qt.Key_J)) move(0, 1)
    else if (key === Qt.Key_Up || (ctrl && key === Qt.Key_K)) move(0, -1)
    else if (key === Qt.Key_Right || (ctrl && key === Qt.Key_L) || (key === Qt.Key_Tab && !shift)) move(1, 0)
    else if (key === Qt.Key_Left || (ctrl && key === Qt.Key_H) || key === Qt.Key_Backtab || (key === Qt.Key_Tab && shift)) move(-1, 0)
    else if (key === Qt.Key_PageDown) move(0, 3)
    else if (key === Qt.Key_PageUp) move(0, -3)
    else if (key === Qt.Key_Return || key === Qt.Key_Enter) pick(cursor)
    else return
    event.accepted = true
  }

  onFlatChanged: if (cursor >= flat.length) cursor = Math.max(0, flat.length - 1)

  Rectangle {
    anchors.fill: parent
    color: Color.menu.scrim
    MouseArea {
      anchors.fill: parent
      acceptedButtons: Qt.AllButtons
      onClicked: root.hide()
    }
  }

  BorderSurface {
    id: card
    anchors.centerIn: parent
    width: Math.min(Style.space(520), parent.width - Style.gapsOut * 2)
    height: Math.min(Style.space(460), parent.height - Style.gapsOut * 2)
    radius: Style.cornerRadius
    color: Color.popups.background
    borderSpec: Border.surfaceSpec("popups", "border", Color.popups.border, Math.max(1, Style.space(2)))
    padding: Style.spacing.popupPadding

    MouseArea { anchors.fill: parent; acceptedButtons: Qt.AllButtons }

    Column {
      id: column
      anchors.fill: parent
      anchors.topMargin: card.contentTopInset
      anchors.rightMargin: card.contentRightInset
      anchors.bottomMargin: card.contentBottomInset
      anchors.leftMargin: card.contentLeftInset
      spacing: Style.spacing.md

      TextField {
        id: searchField
        width: parent.width
        placeholderText: "React with…"
        Keys.priority: Keys.BeforeItem
        Keys.onPressed: function(event) { root.handleKey(event) }
        onTextChanged: root.setQuery(text)
      }

      Flickable {
        id: flick
        width: parent.width
        height: parent.height - searchField.height - statusLine.height - parent.spacing * 2
        contentWidth: width
        contentHeight: body.implicitHeight
        clip: true
        boundsBehavior: Flickable.StopAtBounds
        ScrollBar.vertical: ScrollBar { policy: ScrollBar.AsNeeded }

        Text {
          anchors.centerIn: parent
          visible: root.flat.length === 0
          text: root.query ? "No emoji matches “" + root.query + "”." : "No emoji available."
          color: root.muted
          font.family: root.fontFamily
          font.pixelSize: Style.font.body
        }

        Column {
          id: body
          width: flick.width
          spacing: Style.spacing.sm

          Repeater {
            id: sectionRepeater
            model: root.sections.length
            delegate: Column {
              id: sectionColumn
              required property int index
              readonly property var section: root.sections[index] || ({ title: "", cells: [] })
              readonly property real gridY: header.height + spacing
              width: body.width
              spacing: Style.spacing.xs

              PanelSectionHeader {
                id: header
                width: parent.width
                text: String(sectionColumn.section.title || "")
                foreground: root.foreground
              }

              Grid {
                x: Style.spacing.rowPaddingX
                width: root.gridWidth
                columns: root.columns
                columnSpacing: 0
                rowSpacing: 0

                Repeater {
                  model: sectionColumn.section.cells.length
                  delegate: BorderSurface {
                    id: cellItem
                    required property int index
                    readonly property var cell: sectionColumn.section.cells[index] || ({})
                    // Sections shrink under a filter while stale delegates
                    // are still being torn down: guard the lookup.
                    readonly property int flatIndex: {
                      var base = 0
                      for (var s = 0; s < sectionColumn.index; s++) {
                        var sec = root.sections[s]
                        if (sec) base += sec.cells.length
                      }
                      return base + index
                    }
                    readonly property bool hasCursor: flatIndex === root.cursor
                    readonly property string emojiFile: cell.custom && root.service
                      ? String(root.service.emojiPath(cell.custom, false) || "") : ""
                    readonly property int emojiPx: Math.round(Style.font.heading)
                    width: root.cellSize
                    height: root.cellSize
                    radius: Style.cornerRadius
                    color: hasCursor || cellMouse.containsMouse
                      ? Style.hoverFillFor(root.foreground, Color.accent)
                      : (cell.me ? Style.selectedFillFor(root.foreground, Color.accent) : "transparent")
                    borderSpec: hasCursor
                      ? Border.controlSpec("hover-cursor", root.foreground, Color.accent)
                      : (cell.me ? Border.controlSpec("selected", root.foreground, Color.accent) : Border.none())

                    Image {
                      visible: cellItem.emojiFile !== ""
                      anchors.centerIn: parent
                      width: cellItem.emojiPx
                      height: cellItem.emojiPx
                      asynchronous: true
                      fillMode: Image.PreserveAspectFit
                      sourceSize.width: cellItem.emojiPx * 2
                      sourceSize.height: cellItem.emojiPx * 2
                      source: cellItem.emojiFile ? "file://" + cellItem.emojiFile : ""
                      onStatusChanged: if (status === Image.Error && root.service) root.service.mediaError(cellItem.emojiFile)
                    }
                    Text {
                      visible: cellItem.emojiFile === ""
                      anchors.centerIn: parent
                      width: parent.width - Style.spacing.xs
                      horizontalAlignment: Text.AlignHCenter
                      elide: Text.ElideRight
                      text: cellItem.cell.custom ? String(cellItem.cell.label || "") : String(cellItem.cell.emoji || "")
                      color: root.foreground
                      font.family: root.fontFamily
                      font.pixelSize: cellItem.cell.custom ? Style.font.caption : Style.font.heading
                    }
                    // Reaction count on Toggle cells.
                    Text {
                      visible: !!cellItem.cell.toggle && cellItem.cell.count > 0
                      anchors.right: parent.right
                      anchors.bottom: parent.bottom
                      anchors.margins: Style.spacing.xxs
                      text: String(cellItem.cell.count || 0)
                      color: root.muted
                      font.family: root.fontFamily
                      font.pixelSize: Style.font.caption
                    }
                    MouseArea {
                      id: cellMouse
                      anchors.fill: parent
                      hoverEnabled: true
                      cursorShape: Qt.PointingHandCursor
                      onContainsMouseChanged: if (containsMouse) root.cursor = cellItem.flatIndex
                      onClicked: root.pick(cellItem.flatIndex)
                    }
                  }
                }
              }
            }
          }
        }
      }

      Text {
        id: statusLine
        width: parent.width
        elide: Text.ElideRight
        text: (root.cursorLabel ? root.cursorLabel + " · " : "") + Keymap.footer("picker")
        color: root.muted
        font.family: root.fontFamily
        font.pixelSize: Style.font.caption
      }
    }
  }
}
