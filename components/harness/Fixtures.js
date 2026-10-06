
var USERS = {
  "100": { id: "100", username: "me", display_name: "Matt", avatar_url: "", bot: false },
  "200": { id: "200", username: "ada", display_name: "Ada", avatar_url: "", bot: false },
  "300": { id: "300", username: "lin", display_name: "Lin", avatar_url: "", bot: false },
  "400": { id: "400", username: "buildbot", display_name: "BuildBot", avatar_url: "", bot: true }
}

var SELF_ID = "100"

var CONTENT = [
  "morning all",
  "anyone around? <@100> I pushed the **timeline** branch",
  "looks good so far. one thing: the `ListView` jumps when history loads",
  "```js\nfunction clamp(i, n) {\n  return ((i % n) + n) % n\n}\n```",
  "*italic* _also italic_ ***both*** __underline__ ~~strike~~",
  "spoiler ahead: ||the butler did it||",
  "> quoted line\nnot quoted",
  "# heading\n## subheading\n- bullet one\n- bullet two",
  "see https://github.com/quickshell-mirror/quickshell and [the docs](https://quickshell.org/docs/)",
  "ping <#9001> and <@&5001> about this, deadline <t:1767225600:F> (<t:1767225600:R>)",
  "custom emoji <:omarchy:1234> and <a:party:5678> :tada:",
  "<script>alert('xss')</script> & \"quotes\" <b>not bold</b>",
  "short",
  "a much longer message that should wrap across multiple lines when the window is narrow enough, to make sure implicitHeight follows wrapped rich text correctly and the cursor border hugs the whole row",
  "ok",
  "k"
]

function iso(ms) {
  return new Date(ms).toISOString()
}

function makeMessage(id, authorId, content, ms, extra) {
  var m = {
    id: String(id),
    channel_id: "9000",
    guild_id: "1",
    author: USERS[authorId],
    content: content,
    timestamp: iso(ms),
    edited_timestamp: null,
    nonce: "",
    reply_to: null,
    attachments: [],
    embeds: [],
    reactions: [],
    mentions_self: content.indexOf("<@100>") >= 0,
    system: false
  }
  if (extra) for (var k in extra) m[k] = extra[k]
  return m
}

function build(count, lastId, endMs) {
  var out = []
  var ms = endMs
  var id = lastId
  var authors = ["200", "300", "100", "400"]
  for (var i = count - 1; i >= 0; i--) {
    var authorId = authors[Math.floor(i / 3) % authors.length]
    var content = CONTENT[i % CONTENT.length]
    var extra = {}
    if (i % 17 === 5) extra.reply_to = { message_id: String(id - 3), author_display_name: "Ada", preview: "looks good so far. one thing: the ListView jumps when history loads" }
    if (i % 13 === 7) extra.attachments = [{ id: "a1", filename: "screenshot-2026-08-20.png", content_type: "image/png", size: 348211, url: "https://cdn.discordapp.com/attachments/1/2/screenshot.png", proxy_url: "", width: 1280, height: 720, spoiler: false }]
    if (i % 19 === 11) extra.embeds = [{ type: "rich", title: "quickshell-mirror/quickshell", description: "Flexible toolkit for making desktop shells with QtQuick. **Markdown** in descriptions is flattened.", url: "https://github.com/quickshell-mirror/quickshell", image_url: "", thumbnail_url: "", color: 0 }]
    if (i % 11 === 4) extra.reactions = [{ emoji: "👍", count: 3, me: true }, { emoji: "omarchy:1234", count: 1, me: false }]
    if (i % 23 === 9) extra.edited_timestamp = iso(ms + 60000)
    if (i % 29 === 14) { extra.system = true; extra.content = "Lin pinned a message to this channel." }
    out.unshift(makeMessage(id, authorId, content, ms, extra))
    id -= 1
    var gap = 40 * 1000
    if (i % 7 === 0) gap = 20 * 60 * 1000
    if (i % 31 === 0) gap = 30 * 60 * 60 * 1000
    ms -= gap
  }
  return out
}

function initial() {
  return build(80, 1080, Date.now() - 5 * 60 * 1000)
}

function older(firstId, firstMs, count) {
  return build(count, firstId - 1, firstMs - 5 * 60 * 1000)
}

function ctx(colors) {
  return {
    users: { "100": "Matt", "200": "Ada", "300": "Lin", "400": "BuildBot" },
    channels: { "9001": "dev" },
    roles: { "5001": "maintainers" },
    selfId: SELF_ID,
    mentionColor: colors.mentionColor,
    mentionBg: colors.mentionBg,
    linkColor: colors.linkColor,
    codeBg: colors.codeBg,
    spoilerColor: colors.spoilerColor,
    mutedColor: colors.mutedColor,
    monoFamily: colors.monoFamily,
    fontSize: colors.fontSize
  }
}
