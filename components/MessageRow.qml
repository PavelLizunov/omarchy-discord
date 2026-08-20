pragma ComponentBehavior: Bound
import QtQuick
import qs.Commons
import qs.Ui

import "../Markdown.js" as Markdown

// One timeline row: optional header (author + time), reply line, markdown
// content, attachments, embeds, reactions. Height follows content; the
// Timeline's ListView reads implicitHeight.
Item {
  id: root

  property var message: ({})
  property bool grouped: false
  property bool cursor: false
  property string selfId: ""
  property var ctx: ({})

  signal clicked()
  signal linkActivated(string url)

  readonly property color foreground: Color.foreground
  readonly property color accent: Color.accent
  readonly property string fontFamily: Style.font.family
  readonly property var author: message && message.author ? message.author : ({})
  readonly property bool system: !!(message && message.system)
  readonly property bool mentionsSelf: !!(message && message.mentions_self)
  readonly property bool edited: !!(message && message.edited_timestamp)
  readonly property var replyTo: message && message.reply_to ? message.reply_to : null
  readonly property var attachments: message && Array.isArray(message.attachments) ? message.attachments : []
  readonly property var embeds: message && Array.isArray(message.embeds) ? message.embeds : []
  readonly property var reactions: message && Array.isArray(message.reactions) ? message.reactions : []
  readonly property date when: new Date(String(message && message.timestamp || ""))
  readonly property bool hasTime: !isNaN(when.getTime())
  readonly property string timeText: hasTime ? Qt.formatTime(when, "HH:mm") : ""
  readonly property string fullTimeText: hasTime ? Qt.formatDateTime(when, "dddd d MMMM yyyy HH:mm:ss") : ""
  readonly property string html: Markdown.render(message ? message.content : "", ctx)
  readonly property bool showHeader: !grouped && !system

  readonly property int sidePad: Style.spacing.rowPaddingX
  readonly property int barWidth: Style.spacing.xs

  implicitHeight: body.implicitHeight + (showHeader ? Style.spacing.sm : Style.spacing.xxs) * 2

  BorderSurface {
    anchors.fill: parent
    radius: Style.cornerRadius
    color: root.cursor ? Style.hoverFillFor(root.foreground, root.accent)
      : (root.mentionsSelf ? Util.alpha(Color.urgent, 0.08)
        : (mouse.containsMouse ? Style.hoverFillFor(root.foreground, root.accent) : "transparent"))
    borderSpec: root.cursor
      ? Border.controlSpec("hover-cursor", root.foreground, root.accent)
      : Border.none()
  }

  // Mention bar along the left edge.
  Rectangle {
    visible: root.mentionsSelf
    anchors.left: parent.left
    anchors.top: parent.top
    anchors.bottom: parent.bottom
    anchors.margins: Style.spacing.xxs
    width: root.barWidth
    radius: width / 2
    color: Color.urgent
  }

  MouseArea {
    id: mouse
    anchors.fill: parent
    hoverEnabled: true
    acceptedButtons: Qt.LeftButton
    onClicked: root.clicked()
  }

  Column {
    id: body
    anchors.left: parent.left
    anchors.right: parent.right
    anchors.top: parent.top
    anchors.leftMargin: root.sidePad + root.barWidth
    anchors.rightMargin: root.sidePad
    anchors.topMargin: root.showHeader ? Style.spacing.sm : Style.spacing.xxs
    spacing: Style.spacing.xxs

    // Reply line
    Text {
      width: parent.width
      visible: !!root.replyTo
      elide: Text.ElideRight
      text: root.replyTo
        ? "↳ " + String(root.replyTo.author_display_name || "unknown") + ": "
          + (String(root.replyTo.preview || "") || "(message unavailable)")
        : ""
      color: Color.muted
      font.family: root.fontFamily
      font.pixelSize: Style.font.caption
    }

    // Header: author + time
    Item {
      width: parent.width
      height: root.showHeader ? headerRow.implicitHeight : 0
      visible: root.showHeader

      Row {
        id: headerRow
        spacing: Style.spacing.controlGap

        Text {
          text: String(root.author.display_name || root.author.username || "unknown")
          color: root.foreground
          font.family: root.fontFamily
          font.pixelSize: Style.font.body
          font.bold: true
        }
        Text {
          visible: !!root.author.bot
          anchors.verticalCenter: parent.verticalCenter
          text: "BOT"
          color: Color.muted
          font.family: root.fontFamily
          font.pixelSize: Style.font.caption
        }
        Text {
          id: timeLabel
          anchors.verticalCenter: parent.verticalCenter
          text: root.timeText
          color: Color.muted
          font.family: root.fontFamily
          font.pixelSize: Style.font.caption

          MouseArea {
            id: timeMouse
            anchors.fill: parent
            hoverEnabled: true
            acceptedButtons: Qt.NoButton
          }
          PanelToolTip {
            visible: timeMouse.containsMouse && root.fullTimeText !== ""
            text: root.fullTimeText
          }
        }
      }
    }

    // Content
    Text {
      width: parent.width
      visible: text !== ""
      textFormat: root.system ? Text.PlainText : Text.RichText
      wrapMode: Text.Wrap
      text: root.system ? String(root.message.content || "")
        : root.html + (root.edited ? " <span style=\"color:" + Color.muted + "\">(edited)</span>" : "")
      color: root.system ? Color.muted : root.foreground
      font.family: root.fontFamily
      font.pixelSize: Style.font.body
      font.italic: root.system
      onLinkActivated: function(link) { root.linkActivated(link) }
    }
    // Edited marker for messages with empty content (attachment-only edits).
    Text {
      visible: root.edited && !root.system && root.html === ""
      text: "(edited)"
      color: Color.muted
      font.family: root.fontFamily
      font.pixelSize: Style.font.caption
    }

    // Attachments: filename + size (media arrives in Phase 2)
    Repeater {
      model: root.attachments.length
      delegate: Text {
        required property int index
        readonly property var attachment: root.attachments[index] || ({})
        width: body.width
        elide: Text.ElideMiddle
        text: " " + String(attachment.filename || "attachment")
          + "  " + Markdown.formatSize(attachment.size)
          + (attachment.spoiler ? "  (spoiler)" : "")
        color: root.accent
        font.family: root.fontFamily
        font.pixelSize: Style.font.bodySmall
      }
    }

    // Embeds: left-bordered block with title (link) + description
    Repeater {
      model: root.embeds.length
      delegate: Item {
        id: embedRow
        required property int index
        readonly property var embed: root.embeds[index] || ({})
        readonly property string title: String(embed.title || "")
        readonly property string description: String(embed.description || "")
        readonly property string url: String(embed.url || "")
        width: body.width
        height: visible ? embedColumn.implicitHeight + Style.spacing.sm * 2 : 0
        visible: title !== "" || description !== ""

        Rectangle {
          anchors.fill: parent
          radius: Style.cornerRadius
          color: Util.alpha(root.foreground, 0.05)
        }
        Rectangle {
          anchors.left: parent.left
          anchors.top: parent.top
          anchors.bottom: parent.bottom
          width: root.barWidth
          radius: width / 2
          color: root.accent
        }
        Column {
          id: embedColumn
          anchors.left: parent.left
          anchors.right: parent.right
          anchors.top: parent.top
          anchors.leftMargin: Style.spacing.lg
          anchors.rightMargin: Style.spacing.sm
          anchors.topMargin: Style.spacing.sm
          spacing: Style.spacing.xxs

          Text {
            width: parent.width
            visible: embedRow.title !== ""
            textFormat: Text.RichText
            wrapMode: Text.Wrap
            text: embedRow.url
              ? "<a href=\"" + Markdown.escapeHtml(embedRow.url) + "\" style=\"color:" + root.accent + "\">"
                + Markdown.escapeHtml(embedRow.title) + "</a>"
              : "<b>" + Markdown.escapeHtml(embedRow.title) + "</b>"
            color: root.foreground
            linkColor: root.accent
            font.family: root.fontFamily
            font.pixelSize: Style.font.body
            font.bold: true
            onLinkActivated: function(link) { root.linkActivated(link) }
          }
          Text {
            width: parent.width
            visible: embedRow.description !== ""
            wrapMode: Text.Wrap
            maximumLineCount: 6
            elide: Text.ElideRight
            text: Markdown.plainText(embedRow.description, root.ctx)
            color: root.foreground
            font.family: root.fontFamily
            font.pixelSize: Style.font.bodySmall
          }
        }
      }
    }

    // Reactions
    Flow {
      width: parent.width
      visible: root.reactions.length > 0
      spacing: Style.spacing.xs

      Repeater {
        model: root.reactions.length
        delegate: BorderSurface {
          id: chip
          required property int index
          readonly property var reaction: root.reactions[index] || ({})
          readonly property bool me: !!reaction.me
          readonly property string emoji: {
            var raw = String(reaction.emoji || "")
            var m = /^([^:]+):\d+$/.exec(raw)
            return m ? ":" + m[1] + ":" : raw
          }
          radius: Style.cornerRadius
          color: me ? Style.selectedFillFor(root.foreground, root.accent)
            : Util.alpha(root.foreground, 0.06)
          borderSpec: me
            ? Border.controlSpec("selected", root.foreground, root.accent)
            : Border.none()
          implicitWidth: reactionLabel.implicitWidth + Style.spacing.md * 2
          implicitHeight: reactionLabel.implicitHeight + Style.spacing.xxs * 2

          Text {
            id: reactionLabel
            anchors.centerIn: parent
            text: chip.emoji + " " + String(Number(chip.reaction.count) || 0)
            color: root.foreground
            font.family: root.fontFamily
            font.pixelSize: Style.font.bodySmall
          }
        }
      }
    }
  }
}
