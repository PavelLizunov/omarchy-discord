pragma ComponentBehavior: Bound
import QtQuick
import QtQuick.Controls
import "../ui"

import "../Api.js" as Api

FocusScope {
  id: composer

  property var service: null
  property string channelId: ""
  property string channelName: ""
  property bool active: false
  readonly property int maxLines: 6
  property real attachmentHeightLimit: Style.space(80)
  readonly property bool canSubmit: !!service && channelId !== "" && !uploading
    && (editing ? text.trim().length > 0 : text.trim().length > 0 || chips.length > 0)
  readonly property alias sendControl: sendButton
  readonly property alias attachmentViewport: chipViewport

  property string editingId: ""
  property string replyToId: ""
  property string replyToName: ""
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
  signal switcherRequested()
  signal cheatsheetRequested()
  signal membersRequested()
  signal voiceRequested(string action)

  readonly property color foreground: Color.foreground
  readonly property color muted: Api.secondaryColor(Color.muted, Color.foreground, Color.background)
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

  function startEditLast() {
    if (!service || !channelId) return false
    var id = service.lastOwnMessageId(channelId)
    var message = id ? service.findMessage(channelId, id) : null
    if (!message) return false
    return startEdit(id)
  }

  function startEdit(id) {
    var message = service ? service.findMessage(channelId, id) : null
    if (!message || message.pending || String((message.author || {}).id || "") !== String(service.selfId)) return false
    if (editing) cancelEdit()
    cancelReply()
    savedDraft = input.text
    editingId = id
    setText(message.content)
    focusInput()
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
      if (!service.upload(channelId, content, replyToId)) return false
    } else {
      if (!content.trim()) return false
      if (!service.sendMessage(channelId, content, replyToId)) return false
    }
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
    Qt.callLater(ensureChipVisible)
  }

  function ensureChipVisible() {
    var item = chipRepeater.itemAt(chipCursor)
    if (!item) return
    if (item.y < chipViewport.contentY) chipViewport.contentY = item.y
    else if (item.y + item.height > chipViewport.contentY + chipViewport.height)
      chipViewport.contentY = item.y + item.height - chipViewport.height
  }

  function pasteFromClipboard() {
    if (!service || !channelId) { input.paste(); return }
    service.stageClipboardImage(channelId, function(staged) {
      if (!staged) input.paste()
    })
  }

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
    else if (alt && key === Qt.Key_M) membersRequested()
    else if (alt) return
    else if (ctrl && shift && key === Qt.Key_M) voiceRequested("mute")
    else if (ctrl && shift && key === Qt.Key_D) voiceRequested("deafen")
    else if (ctrl && shift && key === Qt.Key_H) voiceRequested("leave")
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
    else if (ctrl && key === Qt.Key_K) switcherRequested()
    else if (ctrl && key === Qt.Key_Slash) cheatsheetRequested()
    else if (key === Qt.Key_Tab) { if (chips.length) focusChip(0); else if (sendButton.enabled) sendButton.forceActiveFocus(); else cycleFocus(1) }
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
    var ctrl = (event.modifiers & Qt.ControlModifier) !== 0
    var shift = (event.modifiers & Qt.ShiftModifier) !== 0
    if (alt && key === Qt.Key_H) moveZone("left")
    else if (alt && key === Qt.Key_L) moveZone("right")
    else if (alt && key === Qt.Key_M) membersRequested()
    else if (alt) return
    else if (ctrl && shift && key === Qt.Key_M) voiceRequested("mute")
    else if (ctrl && shift && key === Qt.Key_D) voiceRequested("deafen")
    else if (ctrl && shift && key === Qt.Key_H) voiceRequested("leave")
    else if (key === Qt.Key_Escape) focusInput()
    else if ((event.modifiers & Qt.ControlModifier) && key === Qt.Key_K) switcherRequested()
    else if ((event.modifiers & Qt.ControlModifier) && key === Qt.Key_Slash) cheatsheetRequested()
    else if (key === Qt.Key_Return || key === Qt.Key_Enter) submit()
    else if (key === Qt.Key_Tab) { if (chipCursor + 1 < chips.length) focusChip(chipCursor + 1); else { chipCursor = -1; if (sendButton.enabled) sendButton.forceActiveFocus(); else cycleFocus(1) } }
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
    function onDraftRestored(channelId) {
      if (channelId !== composer.channelId) return
      var restored = composer.service.draftFor(channelId)
      var current = composer.editing ? composer.savedDraft : input.text
      var merged = current ? restored + "\n" + current : restored
      if (composer.editing) composer.savedDraft = merged
      else composer.setText(merged)
      composer.service.setDraft(channelId, merged)
    }
  }

  FontMetrics {
    id: metrics
    font: input.font
  }

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
      color: composer.editing ? composer.accent : composer.muted
      font.family: composer.fontFamily
      font.pixelSize: Style.font.caption
    }

    Flickable {
      id: chipViewport
      objectName: "attachment-viewport"
      width: parent.width
      height: Math.min(chipFlow.implicitHeight, composer.attachmentHeightLimit)
      visible: composer.chips.length > 0
      contentWidth: width
      contentHeight: chipFlow.implicitHeight
      clip: true
      boundsBehavior: Flickable.StopAtBounds
      ScrollBar.vertical: ScrollBar { policy: ScrollBar.AsNeeded }
      onWidthChanged: Qt.callLater(composer.ensureChipVisible)
      Flow {
        id: chipFlow
        width: chipViewport.width - Style.spacing.md
        spacing: Style.spacing.sm
        Repeater {
          id: chipRepeater
          model: composer.chips.length
          delegate: AttachmentChip {
            required property int index
            width: Math.min(implicitWidth, chipFlow.width)
            item: composer.chips[index] || ({})
            cursor: composer.chipFocused && index === composer.chipCursor
            onClicked: composer.focusChip(index)
            onRemove: composer.removeChip(index)
          }
        }
      }
    }

    Row {
      width: parent.width
      spacing: Style.spacing.xs
      layoutDirection: Qt.RightToLeft
      Button {
        id: sendButton
        objectName: "send-button"
        iconOnly: true
        text: composer.editing ? "Save" : "Send"
        iconName: composer.editing ? "save" : "send"
        enabled: composer.canSubmit
        focusable: true
        tooltipText: composer.editing ? "Save edit (Enter)" : "Send message (Enter)"
        onClicked: { composer.submit(); composer.focusInput() }
        Keys.onTabPressed: composer.cycleFocus(1)
        Keys.onBacktabPressed: composer.focusInput()
        Keys.onEscapePressed: composer.focusInput()
      }
    BorderSurface {
      id: frame
      width: parent.width - sendButton.width - parent.spacing
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
          placeholderTextColor: composer.muted
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
}
