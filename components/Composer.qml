pragma ComponentBehavior: Bound
import QtQuick
import QtQuick.Controls
import qs.Commons
import qs.Ui

// The composer zone: a wrapping multi-line input (grows to maxLines, then
// scrolls), an optional mode line (reply / edit), and staged attachment
// chips. State that must survive panel destruction (drafts, staged files)
// lives in Service; this file keeps only modes and the chip cursor.
//
// Keys inside the input: Enter sends, Shift+Enter newlines, Up in an empty
// input edits your last message, Ctrl+V stages a clipboard image (text
// pastes fall through), Esc cancels edit/reply mode, then leaves the zone,
// Alt+h/l move zones, Tab/Shift+Tab cycle (through the chips first).
// On a chip: Left/Right/Tab move, x/Delete remove, Esc returns to the input.
FocusScope {
  id: composer

  property var service: null
  property string channelId: ""
  property string channelName: ""
  property bool active: false
  readonly property int maxLines: 6

  property string editingId: ""
  property string replyToId: ""
  property string replyToName: ""
  // -1: the text input owns the keyboard; >= 0: that chip does.
  property int chipCursor: -1
  property string savedDraft: ""
  property bool loadingText: false

  readonly property var chips: service && channelId && Array.isArray(service.staged[channelId])
    ? service.staged[channelId] : []
  readonly property bool chipFocused: chipCursor >= 0 && chipFocus.activeFocus
  readonly property bool inputFocused: input.activeFocus
  readonly property string text: input.text
  readonly property bool editing: editingId !== ""
  readonly property bool replying: replyToId !== ""
  readonly property bool uploading: {
    for (var i = 0; i < chips.length; i++) if (chips[i].uploading) return true
    return false
  }
  readonly property string modeText: editing ? "Editing · Esc cancels"
    : (replying ? "↳ Replying to " + replyToName + " · Esc cancels" : "")

  signal leave()
  signal moveZone(string direction)
  signal cycleFocus(int delta)

  readonly property color foreground: Color.foreground
  readonly property color accent: Color.accent
  readonly property string fontFamily: Style.font.family
  readonly property int lineHeight: Math.ceil(metrics.height)

  implicitHeight: layout.implicitHeight

  function focusInput() {
    chipCursor = -1
    input.forceActiveFocus()
  }

  function setText(value) {
    loadingText = true
    input.text = String(value || "")
    input.cursorPosition = input.length
    loadingText = false
  }

  function loadDraft() {
    setText(service ? service.draftFor(channelId) : "")
  }

  function clearInput() {
    setText("")
    if (service) service.setDraft(channelId, "")
  }

  function startReply(messageId, name) {
    if (editing) cancelEdit()
    replyToId = String(messageId || "")
    replyToName = String(name || "someone")
    focusInput()
  }

  function cancelReply() {
    replyToId = ""
    replyToName = ""
  }

  // Up in an empty input: load your newest message for editing. The draft
  // (empty by construction here, but kept for symmetry) comes back on cancel.
  function startEditLast() {
    if (!service || !channelId) return false
    var id = service.lastOwnMessageId(channelId)
    var message = id ? service.findMessage(channelId, id) : null
    if (!message) return false
    cancelReply()
    savedDraft = input.text
    editingId = id
    setText(message.content)
    return true
  }

  function cancelEdit() {
    if (!editing) return
    editingId = ""
    setText(savedDraft)
    savedDraft = ""
  }

  function submit() {
    if (!service || !channelId || uploading) return false
    var content = input.text
    if (editing) {
      if (!content.trim()) return false
      var id = editingId
      service.editMessage(channelId, id, content, function(ok) {
        if (ok && composer.editingId === id) {
          composer.editingId = ""
          composer.setText(composer.savedDraft)
          composer.savedDraft = ""
        }
      })
      return true
    }
    if (chips.length) {
      var replyId = replyToId
      return service.upload(channelId, content, replyId, function(ok) {
        // Failure keeps the chips and the text; the error lands in the footer.
        if (!ok) return
        composer.setText("")
        if (composer.replyToId === replyId) composer.cancelReply()
      })
    }
    if (!content.trim()) return false
    if (!service.sendMessage(channelId, content, replyToId)) return false
    clearInput()
    cancelReply()
    return true
  }

  function removeChip(index) {
    if (index < 0 || index >= chips.length || !service) return
    if (chips[index].uploading) return
    service.unstage(channelId, chips[index].path)
    var remaining = chips.length
    if (!remaining) { focusInput(); return }
    chipCursor = Math.min(index, remaining - 1)
  }

  function focusChip(index) {
    if (!chips.length) { focusInput(); return }
    chipCursor = Math.max(0, Math.min(index, chips.length - 1))
    chipFocus.forceActiveFocus()
  }

  function pasteFromClipboard() {
    if (!service || !channelId) { input.paste(); return }
    service.stageClipboardImage(channelId, function(staged) {
      if (!staged) input.paste()
    })
  }

  // Synthesized-key entry point (harness) mirroring the focus chain.
  function handleKey(event) {
    if (chipCursor >= 0) handleChipKey(event)
    else handleInputKey(event)
    return event.accepted
  }

  function handleInputKey(event) {
    var key = event.key
    var mods = event.modifiers
    var alt = (mods & Qt.AltModifier) !== 0
    var shift = (mods & Qt.ShiftModifier) !== 0
    var ctrl = (mods & Qt.ControlModifier) !== 0
    if (alt && key === Qt.Key_H) moveZone("left")
    else if (alt && key === Qt.Key_L) moveZone("right")
    // Alt+Up/Down (channel stepping) belongs to the panel.
    else if (alt) return
    else if ((key === Qt.Key_Return || key === Qt.Key_Enter) && !shift) submit()
    else if (key === Qt.Key_Escape) {
      if (editing) cancelEdit()
      else if (replying) cancelReply()
      else leave()
    }
    else if (key === Qt.Key_Up && !ctrl && !shift) {
      if (input.length !== 0 || editing) return
      if (!startEditLast()) return
    }
    else if (ctrl && key === Qt.Key_V) pasteFromClipboard()
    else if (key === Qt.Key_Tab) { if (chips.length) focusChip(0); else cycleFocus(1) }
    else if (key === Qt.Key_Backtab) cycleFocus(-1)
    else if (key === Qt.Key_Left && chips.length && input.cursorPosition === 0 && !shift) focusChip(chips.length - 1)
    else if (key === Qt.Key_Right && chips.length && input.cursorPosition === input.length && !shift) focusChip(0)
    else return
    event.accepted = true
  }

  function handleChipKey(event) {
    var key = event.key
    var text = event.text
    var alt = (event.modifiers & Qt.AltModifier) !== 0
    if (alt && key === Qt.Key_H) moveZone("left")
    else if (alt && key === Qt.Key_L) moveZone("right")
    else if (alt) return
    else if (key === Qt.Key_Escape) focusInput()
    else if (key === Qt.Key_Return || key === Qt.Key_Enter) submit()
    else if (key === Qt.Key_Tab) { if (chipCursor + 1 < chips.length) focusChip(chipCursor + 1); else { chipCursor = -1; cycleFocus(1) } }
    else if (key === Qt.Key_Backtab) { if (chipCursor > 0) focusChip(chipCursor - 1); else focusInput() }
    else if (key === Qt.Key_Right || text === "l") { if (chipCursor + 1 < chips.length) focusChip(chipCursor + 1); else focusInput() }
    else if (key === Qt.Key_Left || text === "h") { if (chipCursor > 0) focusChip(chipCursor - 1); else focusInput() }
    else if (key === Qt.Key_Delete || key === Qt.Key_Backspace || text === "x" || text === "X") removeChip(chipCursor)
    else return
    event.accepted = true
  }

  onChannelIdChanged: {
    editingId = ""
    savedDraft = ""
    cancelReply()
    chipCursor = -1
    loadDraft()
  }
  onChipsChanged: if (chipCursor >= chips.length) { if (chips.length) chipCursor = chips.length - 1; else if (chipFocus.activeFocus) focusInput(); else chipCursor = -1 }
  Component.onCompleted: loadDraft()

  Connections {
    target: composer.service
    ignoreUnknownSignals: true
    // A failed send hands its text back; anything typed since stays below it.
    function onDraftRestored(channelId) {
      if (channelId !== composer.channelId || composer.editing) return
      var restored = composer.service.draftFor(channelId)
      var current = input.text
      composer.setText(current ? restored + "\n" + current : restored)
      composer.service.setDraft(channelId, input.text)
    }
  }

  FontMetrics {
    id: metrics
    font: input.font
  }

  // Keyboard owner while a chip has the cursor (keeps keys out of the input).
  Item {
    id: chipFocus
    width: 0
    height: 0
    Keys.priority: Keys.BeforeItem
    Keys.onPressed: function(event) { composer.handleChipKey(event) }
    onActiveFocusChanged: if (!activeFocus && composer.chipCursor >= 0 && !input.activeFocus) composer.chipCursor = -1
  }

  Column {
    id: layout
    width: parent.width
    spacing: Style.spacing.xs

    Text {
      width: parent.width
      visible: composer.modeText !== ""
      leftPadding: Style.spacing.sm
      elide: Text.ElideRight
      text: composer.modeText
      color: composer.editing ? composer.accent : Color.muted
      font.family: composer.fontFamily
      font.pixelSize: Style.font.caption
    }

    Flow {
      width: parent.width
      visible: composer.chips.length > 0
      spacing: Style.spacing.sm

      Repeater {
        model: composer.chips.length
        delegate: AttachmentChip {
          required property int index
          item: composer.chips[index] || ({})
          cursor: composer.chipFocused && index === composer.chipCursor
          onClicked: composer.focusChip(index)
          onRemove: composer.removeChip(index)
        }
      }
    }

    BorderSurface {
      id: frame
      width: parent.width
      height: flick.height + Style.spacing.inputPaddingY * 2
      radius: Style.cornerRadius
      color: Style.controlFill(composer.inputFocused, composer.active, composer.foreground, composer.accent)
      borderSpec: composer.active
        ? Border.controlSpec("focus", composer.foreground, composer.accent)
        : Border.flat(Color.popups.border, Math.max(1, Style.normalBorderWidth))

      Flickable {
        id: flick
        anchors.left: parent.left
        anchors.right: parent.right
        anchors.verticalCenter: parent.verticalCenter
        anchors.leftMargin: Style.spacing.controlPaddingX
        anchors.rightMargin: Style.spacing.controlPaddingX
        height: Math.min(input.implicitHeight, composer.lineHeight * composer.maxLines
          + input.topPadding + input.bottomPadding)
        contentWidth: width
        contentHeight: input.implicitHeight
        clip: true
        boundsBehavior: Flickable.StopAtBounds
        ScrollBar.vertical: ScrollBar { policy: ScrollBar.AsNeeded }

        TextArea.flickable: TextArea {
          id: input
          focus: true
          wrapMode: TextArea.Wrap
          padding: 0
          topPadding: Style.spacing.xxs
          bottomPadding: Style.spacing.xxs
          background: null
          placeholderText: composer.channelId
            ? "Message " + (composer.channelName || "this channel") : "Pick a channel"
          placeholderTextColor: Color.muted
          color: composer.foreground
          selectionColor: Style.selectionFillFor(composer.foreground, composer.accent)
          selectedTextColor: composer.foreground
          font.family: composer.fontFamily
          font.pixelSize: Style.font.body
          enabled: composer.channelId !== ""
          activeFocusOnTab: false
          Keys.priority: Keys.BeforeItem
          Keys.onPressed: function(event) { composer.handleInputKey(event) }
          onTextChanged: {
            if (composer.loadingText || !composer.service || !composer.channelId) return
            if (composer.editing) return
            composer.service.setDraft(composer.channelId, text)
            if (text.length) composer.service.typing(composer.channelId)
          }
        }
      }
    }
  }
}
