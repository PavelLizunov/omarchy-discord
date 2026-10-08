pragma ComponentBehavior: Bound
import QtQuick
import "../ui"
import "../Markdown.js" as Markdown
import "../Api.js" as Api

Item {
  id: root
  property var message: ({})
  property bool grouped: false
  property bool cursor: false
  property bool armedDelete: false
  property bool spoilersRevealed: false
  property string selfId: ""
  property var ctx: ({})
  property var markdownRenderer: function(text, context) { return Markdown.render(text, context) }
  signal clicked()
  signal linkActivated(string url)
  signal revealRequested()
  signal reactionClicked(string emoji)
  signal selected()
  signal copyLinkRequested(string url)
  readonly property bool hasSelection: content.selectionStart !== content.selectionEnd
  readonly property string selection: content.selectedText
  function clearSelection() { content.deselect() }
  readonly property var author: message.author || ({})
  readonly property bool system: !!message.system
  readonly property bool pending: !!message.pending
  readonly property bool mentionsSelf: !!message.mentions_self
  readonly property bool edited: !!message.edited_timestamp
  readonly property var replyTo: message.reply_to || null
  readonly property var attachments: Array.isArray(message.attachments) ? message.attachments : []
  readonly property var embeds: Array.isArray(message.embeds) ? message.embeds : []
  readonly property var reactions: Array.isArray(message.reactions) ? message.reactions : []
  readonly property bool showHeader: !grouped && !system
  readonly property color foreground: Color.foreground
  readonly property color paintedBackground: Api.blend(root.cursor || hover.hovered
    ? Style.hoverFillFor(Color.foreground, Color.accent)
    : root.mentionsSelf ? Util.alpha(Color.urgent, 0.08) : Qt.rgba(0, 0, 0, 0), Color.background,
    root.cursor || hover.hovered ? Style.hoverFillAlpha : root.mentionsSelf ? 0.08 : 0)
  readonly property color muted: Api.secondaryColor(Color.muted, Color.foreground, paintedBackground)
  readonly property color linkColor: Api.textColor(Color.accent, Color.foreground, paintedBackground)
  readonly property string rawContent: String(message.content || "")
  readonly property string html: markdownRenderer(rawContent, Object.assign({}, ctx,
    {linkColor:String(root.linkColor),emojiPath:undefined}))
  readonly property date when: new Date(String(message.timestamp || ""))
  readonly property string timeText: isNaN(when.getTime()) ? "" : Qt.formatTime(when, "HH:mm")
  readonly property string fullTimeText: isNaN(when.getTime()) ? "" : Qt.formatDateTime(when, "dddd d MMMM yyyy HH:mm:ss")
  readonly property bool hasSpoilerImages: attachments.some(function(a) { return !!a.spoiler })
  implicitHeight: body.implicitHeight + Style.spacing.sm * 2
  BorderSurface {
    anchors.fill: parent
    radius: Style.cornerRadius
    color: root.cursor || hover.hovered ? Style.hoverFillFor(Color.foreground, Color.accent)
      : root.mentionsSelf ? Util.alpha(Color.urgent, 0.08) : "transparent"
    borderSpec: root.cursor ? Border.controlSpec("hover-cursor", Color.foreground, Color.accent) : Border.none()
  }
  MouseArea { anchors.fill: parent; onClicked: root.clicked() }
  HoverHandler { id: hover }
  Column {
    id: body
    anchors.left: parent.left
    anchors.right: parent.right
    anchors.top: parent.top
    anchors.margins: Style.spacing.sm
    spacing: Style.spacing.xxs
    opacity: root.pending ? 0.55 : 1
    Row {
      width: parent.width
      visible: !!root.replyTo
      spacing: Style.spacing.xxs

      Text {
        text: "↳ "
        color: root.muted
        font.family: Style.font.family
        font.pixelSize: Style.font.caption
      }
      Text {
        text: root.replyTo ? String(root.replyTo.author_display_name || root.replyTo.author_username || "unknown") : ""
        color: Api.authorColor(root.replyTo ? root.replyTo.author_id : "", root.paintedBackground, root.foreground)
        font.family: Style.font.family
        font.pixelSize: Style.font.caption
        font.bold: true
      }
      Text {
        width: Math.max(0, body.width - x)
        elide: Text.ElideRight
        text: root.replyTo ? ": " + String(root.replyTo.preview || "(message unavailable)") : ""
        color: root.muted
        font.family: Style.font.family
        font.pixelSize: Style.font.caption
      }
    }
    Row {
      id: headerRow
      width: parent.width
      visible: root.showHeader
      spacing: Style.spacing.xs

      Text {
        id: authorLabel
        text: String(root.author.display_name || root.author.username || "unknown")
        textFormat: Text.PlainText
        elide: Text.ElideRight
        color: Api.authorColor(root.author.id, root.paintedBackground, Color.foreground)
        font.family: Style.font.family
        font.pixelSize: Style.font.body
        font.bold: true
      }

      Rectangle {
        id: botBadge
        visible: !!root.author.bot
        anchors.verticalCenter: parent.verticalCenter
        height: Style.space(14)
        width: botText.implicitWidth + Style.spacing.xs * 2
        radius: Style.space(3)
        color: Util.alpha(Color.accent, 0.2)
        Text {
          id: botText
          anchors.centerIn: parent
          text: "BOT"
          color: Api.textColor(Color.accent, Color.foreground, Api.blend(Color.accent, root.paintedBackground, 0.2))
          font.family: Style.font.family
          font.pixelSize: Style.font.caption
          font.bold: true
        }
      }

      Text {
        id: timeLabel
        anchors.verticalCenter: parent.verticalCenter
        text: root.timeText
        textFormat: Text.PlainText
        color: root.muted
        font.family: Style.font.family
        font.pixelSize: Style.font.caption
      }
    }
    TextEdit {
      id: content
      width: parent.width
      visible: text !== ""
      readOnly: true
      selectByMouse: true
      selectByKeyboard: false
      activeFocusOnPress: false
      persistentSelection: true
      selectionColor: Style.selectionFillFor(Color.foreground, Color.accent)
      selectedTextColor: Color.foreground
      textFormat: root.system ? TextEdit.PlainText : TextEdit.RichText
      wrapMode: TextEdit.Wrap
      text: root.system ? String(root.message.content || "") : root.html
      color: root.system ? root.muted : root.foreground
      font.family: Style.font.family
      font.pixelSize: Style.font.body
      font.italic: root.system
      onLinkActivated: function(url) { root.linkActivated(url) }
      onSelectedTextChanged: if (selectedText) root.selected()
      TapHandler { onSingleTapped: root.clicked() }
    }
    Text {
      visible: root.edited
      text: "(edited)"
      color: root.muted
      font.family: Style.font.family
      font.pixelSize: Style.font.caption
    }
    Repeater {
      model: root.attachments
      delegate: Text {
        required property var modelData
        width: body.width
        textFormat: Text.RichText
        wrapMode: Text.Wrap
        readonly property bool covered: !!modelData.spoiler && !root.spoilersRevealed
        text: covered ? "[spoiler attachment — Enter to reveal]"
          : '<a style="color:' + root.linkColor + '" href="' + Markdown.escapeHtml(String(modelData.url || "")) + '">'
            + Markdown.escapeHtml(String(modelData.filename || "attachment")) + '</a>  '
            + Markdown.formatSize(modelData.size)
        color: root.linkColor
        linkColor: root.linkColor
        font.family: Style.font.family
        font.pixelSize: Style.font.bodySmall
        onLinkActivated: function(url) { root.linkActivated(url) }
        TapHandler { onSingleTapped: if (parent.covered) root.revealRequested() }
      }
    }
    Repeater {
      model: root.embeds
      delegate: Text {
        required property var modelData
        width: body.width
        textFormat: Text.RichText
        wrapMode: Text.Wrap
        text: (modelData.url ? '<a style="color:' + root.linkColor + '" href="' + Markdown.escapeHtml(String(modelData.url)) + '">' : "")
          + Markdown.escapeHtml(String(modelData.title || modelData.url || "Link"))
          + (modelData.url ? "</a>" : "")
          + (modelData.description ? "<br>" + Markdown.escapeHtml(Markdown.plainText(String(modelData.description), root.ctx)) : "")
        color: root.muted
        linkColor: root.linkColor
        font.family: Style.font.family
        font.pixelSize: Style.font.bodySmall
        onLinkActivated: function(url) { root.linkActivated(url) }
      }
    }
    Flow {
      width: parent.width
      spacing: Style.spacing.xs
      Repeater {
        model: root.reactions
        delegate: Button {
          required property var modelData
          readonly property var custom: /^([^:]+):(\d+)$/.exec(String(modelData.emoji || ""))
          text: (custom ? ":" + custom[1] + ":" : String(modelData.emoji || "")) + " " + Number(modelData.count || 0)
          selected: !!modelData.me
          focusable: true
          horizontalPadding: Style.spacing.sm
          verticalPadding: Style.spacing.xxs
          onClicked: root.reactionClicked(String(modelData.emoji || ""))
        }
      }
    }
    Text {
      width: parent.width
      visible: root.armedDelete
      text: "D again to delete · Esc cancels"
      wrapMode: Text.Wrap
      color: Color.urgent
      font.family: Style.font.family
      font.pixelSize: Style.font.caption
    }
  }
}
