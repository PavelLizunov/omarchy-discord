pragma ComponentBehavior: Bound
import QtQuick
import QtQuick.Effects
import qs.Commons
import qs.Ui

import "../Api.js" as Api
import "../Markdown.js" as Markdown

// One timeline row: avatar + optional header (author + time), reply line,
// markdown content, attachments (inline image previews through the media
// cache, filename chips otherwise), embeds, reactions. Height follows
// content; the Timeline's ListView reads implicitHeight.
//
// Media comes through `ctx` (Service.markdownCtx): ctx.mediaPath(url, size)
// and ctx.emojiPath(id) return a cached local path or "" while fetching, and
// ctx.imagePreviews gates inline images. The row never talks to the service.
Item {
  id: root

  property var message: ({})
  property bool grouped: false
  property bool cursor: false
  // First D landed on this (own) row: show the confirm hint.
  property bool armedDelete: false
  // Spoiler attachments shown uncovered (Timeline keeps this per message id).
  property bool spoilersRevealed: false
  property string selfId: ""
  property var ctx: ({})

  signal clicked()
  signal linkActivated(string url)
  signal revealRequested()
  // A reaction chip was clicked (wire-form emoji).
  signal reactionClicked(string emoji)

  readonly property color foreground: Color.foreground
  readonly property color accent: Color.accent
  readonly property string fontFamily: Style.font.family
  readonly property var author: message && message.author ? message.author : ({})
  readonly property bool system: !!(message && message.system)
  // Optimistic row awaiting its gateway echo (Service.sendMessage).
  readonly property bool pending: !!(message && message.pending)
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
  readonly property bool previews: !(ctx && ctx.imagePreviews === false)
  readonly property bool hasSpoilerImages: {
    for (var i = 0; i < attachments.length; i++)
      if (attachments[i] && attachments[i].spoiler && isImageAttachment(attachments[i])) return true
    return false
  }
  readonly property string avatarPath: mediaPath(author.avatar_url, 64)
  readonly property string initials: {
    var name = String(author.display_name || author.username || "?").trim()
    return name ? name.charAt(0).toUpperCase() : "?"
  }

  readonly property int sidePad: Style.spacing.rowPaddingX
  readonly property int barWidth: Style.spacing.xs
  readonly property int avatarSize: Style.space(36)
  readonly property int gutter: system ? 0 : avatarSize + Style.spacing.md
  readonly property int maxImageWidth: Style.space(400)
  readonly property int maxImageHeight: Style.space(300)

  function mediaPath(url, size) {
    if (!url || !ctx || typeof ctx.mediaPath !== "function") return ""
    try { return String(ctx.mediaPath(String(url), size) || "") } catch (e) { return "" }
  }

  // A cached file failed to load (evicted): tell the service so it drops
  // the path and fetches again (once).
  function mediaError(path) {
    if (!path || !ctx || typeof ctx.mediaError !== "function") return
    try { ctx.mediaError(String(path)) } catch (e) {}
  }

  function emojiPath(id) {
    if (!id || !ctx || typeof ctx.emojiPath !== "function") return ""
    try { return String(ctx.emojiPath(String(id), false) || "") } catch (e) { return "" }
  }

  function isImageAttachment(attachment) {
    return !!attachment && String(attachment.content_type || "").indexOf("image/") === 0
      && String(attachment.url || "") !== ""
  }

  // Preview box for an image attachment: its own size scaled into the
  // width/height caps (400x300 default when Discord sent no dimensions).
  function previewSize(attachment, available) {
    var w = Math.max(0, Number(attachment.width) || 0)
    var h = Math.max(0, Number(attachment.height) || 0)
    if (!w || !h) { w = maxImageWidth; h = maxImageHeight }
    var maxW = Math.max(Style.space(40), Math.min(maxImageWidth, available))
    var scale = Math.min(1, maxW / w, maxImageHeight / h)
    return { width: Math.max(1, Math.round(w * scale)), height: Math.max(1, Math.round(h * scale)) }
  }

  implicitHeight: Math.max(body.implicitHeight + (showHeader ? Style.spacing.sm : Style.spacing.xxs) * 2,
    showHeader ? avatar.anchors.topMargin + avatarSize + Style.spacing.sm : 0)

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

  // Delete confirmation, top-right of the row.
  Text {
    visible: root.armedDelete
    anchors.right: parent.right
    anchors.top: parent.top
    anchors.rightMargin: root.sidePad
    anchors.topMargin: Style.spacing.sm
    text: "D again to delete · Esc cancels"
    color: Color.urgent
    font.family: root.fontFamily
    font.pixelSize: Style.font.caption
    font.bold: true
  }

  // Avatar (header rows only): the cached image masked round, an initial on
  // a muted disc until it lands.
  Item {
    id: avatar
    visible: root.showHeader
    anchors.left: parent.left
    anchors.top: parent.top
    anchors.leftMargin: root.sidePad + root.barWidth
    anchors.topMargin: Style.spacing.sm + (replyLine.visible ? replyLine.height + body.spacing : 0)
    width: root.avatarSize
    height: root.avatarSize

    Rectangle {
      anchors.fill: parent
      radius: width / 2
      color: Util.alpha(root.foreground, 0.1)
      visible: !avatarEffect.visible

      Text {
        anchors.centerIn: parent
        text: root.initials
        color: Color.muted
        font.family: root.fontFamily
        font.pixelSize: Style.font.subtitle
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
      source: root.avatarPath ? "file://" + root.avatarPath : ""
      onStatusChanged: if (status === Image.Error) root.mediaError(root.avatarPath)
    }
    MultiEffect {
      id: avatarEffect
      anchors.fill: avatarImage
      source: avatarImage
      maskEnabled: true
      maskSource: avatarMask
      visible: root.avatarPath !== "" && avatarImage.status === Image.Ready
    }
  }

  Column {
    id: body
    anchors.left: parent.left
    anchors.right: parent.right
    anchors.top: parent.top
    anchors.leftMargin: root.sidePad + root.barWidth + root.gutter
    anchors.rightMargin: root.sidePad
    anchors.topMargin: root.showHeader ? Style.spacing.sm : Style.spacing.xxs
    spacing: Style.spacing.xxs
    opacity: root.pending ? 0.55 : 1

    // Reply line
    Text {
      id: replyLine
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

    // Attachments: image previews inline (media cache), other files as
    // filename chips. A spoiler image stays covered until revealed (click or
    // Enter on the row).
    Repeater {
      model: root.attachments.length
      delegate: Loader {
        id: attachmentSlot
        required property int index
        readonly property var attachment: root.attachments[index] || ({})
        readonly property bool image: root.previews && root.isImageAttachment(attachment)
        width: body.width
        sourceComponent: image ? imagePreview : fileChip
        onLoaded: item.attachment = Qt.binding(function() { return attachmentSlot.attachment })
      }
    }

    // Embeds: left-bordered block with title (link), description, and when
    // previews are on a thumbnail (right) / image (below) via the cache.
    Repeater {
      model: root.embeds.length
      delegate: Item {
        id: embedRow
        required property int index
        readonly property var embed: root.embeds[index] || ({})
        readonly property string title: String(embed.title || "")
        readonly property string description: String(embed.description || "")
        readonly property string url: String(embed.url || "")
        // Off-CDN embed media (link previews of other sites) is never fetched.
        readonly property string thumbUrl: root.previews && Api.isCdnUrl(embed.thumbnail_url) ? String(embed.thumbnail_url) : ""
        readonly property string imageUrl: root.previews && Api.isCdnUrl(embed.image_url) ? String(embed.image_url) : ""
        readonly property string thumbPath: root.mediaPath(thumbUrl, 0)
        readonly property string imagePath: root.mediaPath(imageUrl, 0)
        readonly property int thumbSize: Style.space(64)
        readonly property var imageBox: root.previewSize({ width: 0, height: 0 },
          body.width - Style.spacing.lg - Style.spacing.sm)
        width: body.width
        height: visible ? Math.max(embedColumn.implicitHeight, embedThumb.visible ? thumbSize : 0) + Style.spacing.sm * 2 : 0
        visible: title !== "" || description !== "" || imageUrl !== "" || thumbUrl !== ""

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
        Image {
          id: embedThumb
          visible: embedRow.thumbUrl !== ""
          anchors.right: parent.right
          anchors.top: parent.top
          anchors.margins: Style.spacing.sm
          width: embedRow.thumbSize
          height: embedRow.thumbSize
          asynchronous: true
          fillMode: Image.PreserveAspectFit
          sourceSize.width: embedRow.thumbSize * 2
          sourceSize.height: embedRow.thumbSize * 2
          source: embedRow.thumbPath ? "file://" + embedRow.thumbPath : ""
          onStatusChanged: if (status === Image.Error) root.mediaError(embedRow.thumbPath)

          Rectangle {
            anchors.fill: parent
            radius: Style.cornerRadius
            color: Util.alpha(root.foreground, 0.08)
            visible: embedThumb.status !== Image.Ready
          }
        }
        Column {
          id: embedColumn
          anchors.left: parent.left
          anchors.right: parent.right
          anchors.top: parent.top
          anchors.leftMargin: Style.spacing.lg
          anchors.rightMargin: Style.spacing.sm + (embedThumb.visible ? embedRow.thumbSize + Style.spacing.sm : 0)
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
          Image {
            id: embedImage
            visible: embedRow.imageUrl !== ""
            width: status === Image.Ready ? paintedWidth : embedRow.imageBox.width
            height: status === Image.Ready ? paintedHeight : embedRow.imageBox.height
            asynchronous: true
            fillMode: Image.PreserveAspectFit
            sourceSize.width: embedRow.imageBox.width * 2
            sourceSize.height: embedRow.imageBox.height * 2
            source: embedRow.imagePath ? "file://" + embedRow.imagePath : ""
            onStatusChanged: if (status === Image.Error) root.mediaError(embedRow.imagePath); else if (status === Image.Ready) {
              // Fit the loaded size into the caps; paintedWidth follows.
              var box = root.previewSize({ width: implicitWidth, height: implicitHeight }, embedColumn.width)
              width = box.width
              height = box.height
            }

            Rectangle {
              anchors.fill: parent
              radius: Style.cornerRadius
              color: Util.alpha(root.foreground, 0.08)
              visible: embedImage.status !== Image.Ready
            }
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
          readonly property var custom: /^([^:]+):(\d+)$/.exec(String(reaction.emoji || ""))
          readonly property string emoji: custom ? ":" + custom[1] + ":" : String(reaction.emoji || "")
          readonly property string emojiFile: custom ? root.emojiPath(custom[2]) : ""
          readonly property int emojiPx: Math.round(Style.font.bodySmall * 1.4)
          radius: Style.cornerRadius
          color: me ? Style.selectedFillFor(root.foreground, root.accent)
            : Util.alpha(root.foreground, 0.06)
          borderSpec: me
            ? Border.controlSpec("selected", root.foreground, root.accent)
            : Border.none()
          implicitWidth: reactionRow.implicitWidth + Style.spacing.md * 2
          implicitHeight: Math.max(reactionRow.implicitHeight, chip.emojiPx) + Style.spacing.xxs * 2

          Row {
            id: reactionRow
            anchors.centerIn: parent
            spacing: Style.spacing.xs

            Image {
              visible: chip.emojiFile !== ""
              anchors.verticalCenter: parent.verticalCenter
              width: chip.emojiPx
              height: chip.emojiPx
              asynchronous: true
              fillMode: Image.PreserveAspectFit
              sourceSize.width: chip.emojiPx * 2
              sourceSize.height: chip.emojiPx * 2
              source: chip.emojiFile ? "file://" + chip.emojiFile : ""
              onStatusChanged: if (status === Image.Error) root.mediaError(chip.emojiFile)
            }
            Text {
              visible: chip.emojiFile === ""
              anchors.verticalCenter: parent.verticalCenter
              text: chip.emoji
              color: root.foreground
              font.family: root.fontFamily
              font.pixelSize: Style.font.bodySmall
            }
            Text {
              anchors.verticalCenter: parent.verticalCenter
              text: String(Number(chip.reaction.count) || 0)
              color: root.foreground
              font.family: root.fontFamily
              font.pixelSize: Style.font.bodySmall
            }
          }

          MouseArea {
            id: chipMouse
            anchors.fill: parent
            hoverEnabled: true
            cursorShape: Qt.PointingHandCursor
            onClicked: root.reactionClicked(String(chip.reaction.emoji || ""))
          }
          PanelToolTip {
            visible: chipMouse.containsMouse
            text: (chip.me ? "Remove your " : "React with ") + chip.emoji
          }
        }
      }
    }
  }

  // --- attachment delegates ---
  Component {
    id: fileChip

    Text {
      property var attachment: ({})
      elide: Text.ElideMiddle
      text: " " + String(attachment.filename || "attachment")
        + "  " + Markdown.formatSize(attachment.size)
        + (attachment.spoiler ? "  (spoiler)" : "")
      color: root.accent
      font.family: root.fontFamily
      font.pixelSize: Style.font.bodySmall
    }
  }

  Component {
    id: imagePreview

    Item {
      id: preview
      property var attachment: ({})
      readonly property bool spoiler: !!attachment.spoiler
      readonly property bool covered: spoiler && !root.spoilersRevealed
      readonly property string path: root.mediaPath(attachment.url, 0)
      readonly property var box: root.previewSize(attachment, width)
      implicitHeight: box.height

      // Muted placeholder while the file is fetched; doubles as the spoiler
      // cover (the image is not even loaded until revealed).
      Rectangle {
        width: preview.box.width
        height: preview.box.height
        radius: Style.cornerRadius
        color: Util.alpha(root.foreground, preview.covered ? 0.14 : 0.06)
        visible: preview.covered || previewImage.status !== Image.Ready

        Text {
          anchors.centerIn: parent
          text: preview.covered ? "SPOILER" : (previewImage.status === Image.Error ? "image unavailable" : "")
          color: preview.covered ? root.foreground : Color.muted
          font.family: root.fontFamily
          font.pixelSize: preview.covered ? Style.font.bodySmall : Style.font.caption
          font.bold: preview.covered
        }
        Text {
          anchors.bottom: parent.bottom
          anchors.horizontalCenter: parent.horizontalCenter
          anchors.bottomMargin: Style.spacing.sm
          visible: preview.covered
          text: "Enter or click to reveal"
          color: Color.muted
          font.family: root.fontFamily
          font.pixelSize: Style.font.caption
        }
      }
      Image {
        id: previewImage
        width: preview.box.width
        height: preview.box.height
        visible: !preview.covered && status === Image.Ready
        asynchronous: true
        fillMode: Image.PreserveAspectFit
        horizontalAlignment: Image.AlignLeft
        sourceSize.width: preview.box.width * 2
        sourceSize.height: preview.box.height * 2
        source: !preview.covered && preview.path ? "file://" + preview.path : ""
        onStatusChanged: if (status === Image.Error) root.mediaError(preview.path)
      }
      MouseArea {
        width: preview.box.width
        height: preview.box.height
        cursorShape: Qt.PointingHandCursor
        onClicked: {
          if (preview.covered) root.revealRequested()
          else if (preview.path) root.linkActivated(preview.path)
          else if (preview.attachment.url) root.linkActivated(String(preview.attachment.url))
        }
      }
    }
  }
}
