
var PH_OPEN = ""
var PH_CLOSE = ""

function escapeHtml(text) {
  return String(text)
    .replace(/&/g, "&amp;")
    .replace(/</g, "&lt;")
    .replace(/>/g, "&gt;")
    .replace(/"/g, "&quot;")
}

function str(value, fallback) {
  return value === undefined || value === null ? String(fallback || "") : String(value)
}

function lookup(map, id) {
  if (!map) return ""
  var value = map[id]
  if (value === undefined || value === null) return ""
  return String(value)
}

function pad2(n) {
  return (n < 10 ? "0" : "") + n
}

var MONTHS = ["January", "February", "March", "April", "May", "June", "July",
  "August", "September", "October", "November", "December"]
var DAYS = ["Sunday", "Monday", "Tuesday", "Wednesday", "Thursday", "Friday", "Saturday"]

function fmtTime(d) { return pad2(d.getHours()) + ":" + pad2(d.getMinutes()) }
function fmtTimeSec(d) { return fmtTime(d) + ":" + pad2(d.getSeconds()) }
function fmtDateShort(d) { return pad2(d.getDate()) + "/" + pad2(d.getMonth() + 1) + "/" + d.getFullYear() }
function fmtDateLong(d) { return d.getDate() + " " + MONTHS[d.getMonth()] + " " + d.getFullYear() }

function fmtRelative(d, now) {
  var diff = Math.round(((now || Date.now()) - d.getTime()) / 1000)
  var past = diff >= 0
  var s = Math.abs(diff)
  var units = [["year", 31536000], ["month", 2592000], ["day", 86400],
    ["hour", 3600], ["minute", 60], ["second", 1]]
  for (var i = 0; i < units.length; i++) {
    var n = Math.floor(s / units[i][1])
    if (n >= 1 || i === units.length - 1) {
      var label = n + " " + units[i][0] + (n === 1 ? "" : "s")
      return past ? label + " ago" : "in " + label
    }
  }
  return "now"
}

function formatTimestamp(unix, style, now) {
  var seconds = Number(unix)
  if (!isFinite(seconds)) return ""
  var d = new Date(seconds * 1000)
  if (isNaN(d.getTime())) return ""
  switch (style) {
  case "t": return fmtTime(d)
  case "T": return fmtTimeSec(d)
  case "d": return fmtDateShort(d)
  case "D": return fmtDateLong(d)
  case "F": return DAYS[d.getDay()] + ", " + fmtDateLong(d) + " " + fmtTime(d)
  case "R": return fmtRelative(d, now)
  default: return fmtDateLong(d) + " " + fmtTime(d)
  }
}

function Stash() {
  this.items = []
}
Stash.prototype.put = function(html) {
  this.items.push(html)
  return PH_OPEN + (this.items.length - 1) + PH_CLOSE
}
Stash.prototype.restore = function(text) {
  var items = this.items
  var guard = 0
  while (text.indexOf(PH_OPEN) >= 0 && guard++ < 8) {
    text = text.replace(/(\d+)/g, function(_, n) {
      var v = items[Number(n)]
      return v === undefined ? "" : v
    })
  }
  return text
}

function span(style, inner) {
  return "<span style=\"" + style + "\">" + inner + "</span>"
}

function mentionHtml(ctx, label) {
  var style = ""
  if (ctx.mentionColor) style += "color:" + ctx.mentionColor + ";"
  if (ctx.mentionBg) style += "background-color:" + ctx.mentionBg + ";"
  return style ? span(style, "<b>" + label + "</b>") : "<b>" + label + "</b>"
}

function linkHtml(ctx, href, label) {
  var style = ctx.linkColor ? " style=\"color:" + ctx.linkColor + "\"" : ""
  return "<a href=\"" + href + "\"" + style + ">" + label + "</a>"
}

var SAFE_PATH_RE = /^\/[^\x00-\x1f"'<>&\\]*$/

function emojiImg(ctx, id, animated, name) {
  if (!ctx || typeof ctx.emojiPath !== "function") return ""
  var path = ""
  try { path = str(ctx.emojiPath(id, animated)) } catch (e) { return "" }
  if (!path || !SAFE_PATH_RE.test(path)) return ""
  var size = Math.max(8, Math.round(Number(ctx.emojiSize) || Math.round((Number(ctx.fontSize) || 12) * 1.4)))
  return "<img src=\"file://" + escapeHtml(path) + "\" width=\"" + size + "\" height=\"" + size
    + "\" alt=\":" + escapeHtml(name) + ":\">"
}

function codeStyle(ctx) {
  var style = ""
  if (ctx.monoFamily) style += "font-family:'" + String(ctx.monoFamily).replace(/'/g, "") + "';"
  if (ctx.codeBg) style += "background-color:" + ctx.codeBg + ";"
  return style
}

function blockCodeStyle(ctx) {
  return codeStyle(ctx) + "white-space:pre-wrap;"
}

var URL_RE = /https?:\/\/[^\s<>"'()\[\]|]+[^\s<>"'()\[\]|.,;:!?]/g
var URL_ESCAPED_RE = /https?:\/\/[^\s<>()\[\]&|]+(?:&amp;[^\s<>()\[\]&|]+)*/g

function trimUrl(url) {
  return url.replace(/[.,;:!?]+$/, "")
}

function protectInline(text, ctx, stash, plain) {
  text = text.replace(/(``|`)([^`\n]+?)\1/g, function(_, __, code) {
    return stash.put(plain ? code : "<code style=\"" + codeStyle(ctx) + "\">" + code + "</code>")
  })
  text = text.replace(/&lt;@!?(\d+)&gt;/g, function(_, id) {
    var name = lookup(ctx.users, id)
    var label = "@" + (name || (id === str(ctx.selfId) ? "you" : id))
    return stash.put(plain ? label : mentionHtml(ctx, escapeHtml(label)))
  })
  text = text.replace(/&lt;@&amp;(\d+)&gt;/g, function(_, id) {
    var label = "@" + (lookup(ctx.roles, id) || "role")
    return stash.put(plain ? label : mentionHtml(ctx, escapeHtml(label)))
  })
  text = text.replace(/&lt;#(\d+)&gt;/g, function(_, id) {
    var label = "#" + (lookup(ctx.channels, id) || id)
    return stash.put(plain ? label : mentionHtml(ctx, escapeHtml(label)))
  })
  text = text.replace(/&lt;(a?):(\w+):(\d+)&gt;/g, function(_, animated, name, id) {
    var img = plain ? "" : emojiImg(ctx, id, animated === "a", name)
    return stash.put(img || ":" + name + ":")
  })
  text = text.replace(/&lt;t:(-?\d+)(?::([tTdDfFR]))?&gt;/g, function(_, unix, style) {
    var label = formatTimestamp(unix, style || "f", ctx.now)
    if (!label) return stash.put("")
    return stash.put(plain ? label : "<u>" + escapeHtml(label) + "</u>")
  })
  text = text.replace(/\[([^\]\n]+)\]\((https?:\/\/(?:[^\s()]|\([^\s()]*\))+)\)/g, function(_, label, url) {
    return stash.put(plain ? label : linkHtml(ctx, url, label))
  })
  text = text.replace(URL_ESCAPED_RE, function(url) {
    var clean = trimUrl(url)
    var tail = url.slice(clean.length)
    return stash.put(plain ? clean : linkHtml(ctx, clean, clean)) + tail
  })
  return text
}

function forceSpoilerColor(html, color) {
  return html.replace(/style="([^"]*)"/g, function(_, style) {
    var kept = style.replace(/(?:background-)?color:[^;"]*;?/g, "")
    if (kept && !/;$/.test(kept)) kept += ";"
    return "style=\"" + kept + "background-color:" + color + ";color:" + color + "\""
  })
}

function formatInline(text, ctx, stash, plain) {
  if (plain) {
    return text
      .replace(/\|\|([\s\S]+?)\|\|/g, "$1")
      .replace(/\*\*\*([\s\S]+?)\*\*\*/g, "$1")
      .replace(/\*\*([\s\S]+?)\*\*/g, "$1")
      .replace(/__([\s\S]+?)__/g, "$1")
      .replace(/~~([\s\S]+?)~~/g, "$1")
      .replace(/\*([^*\n]+?)\*/g, "$1")
      .replace(/(^|[^A-Za-z0-9])_([^_\n]+?)_(?![A-Za-z0-9])/g, "$1$2")
  }
  var spoiler = ctx.spoilerColor
    ? "background-color:" + ctx.spoilerColor + ";color:" + ctx.spoilerColor
    : ""
  text = text.replace(/\|\|([\s\S]+?)\|\|/g, function(_, inner) {
    if (!spoiler) return "<s>" + inner + "</s>"
    var covered = forceSpoilerColor(stash.restore(formatMarks(inner)), ctx.spoilerColor)
    return stash.put(span(spoiler, covered))
  })
  return formatMarks(text)
}

function formatMarks(text) {
  text = text.replace(/\*\*\*([\s\S]+?)\*\*\*/g, "<b><i>$1</i></b>")
  text = text.replace(/\*\*([\s\S]+?)\*\*/g, "<b>$1</b>")
  text = text.replace(/__([\s\S]+?)__/g, "<u>$1</u>")
  text = text.replace(/~~([\s\S]+?)~~/g, "<s>$1</s>")
  text = text.replace(/\*([^*\n]+?)\*/g, "<i>$1</i>")
  text = text.replace(/(^|[^A-Za-z0-9])_([^_\n]+?)_(?![A-Za-z0-9])/g, "$1<i>$2</i>")
  return text
}

function headerPx(ctx, level) {
  var base = Number(ctx.fontSize) || 12
  var scale = level === 1 ? 1.45 : (level === 2 ? 1.25 : 1.1)
  return Math.round(base * scale)
}

function blocks(text, ctx, stash, plain) {
  text = text.replace(/```(?:([A-Za-z0-9_+#.-]{0,20})\n)?([\s\S]*?)```/g, function(_, lang, code) {
    code = code.replace(/\n$/, "")
    if (plain) return stash.put(code)
    return stash.put("<pre style=\"" + blockCodeStyle(ctx) + "\">" + code + "</pre>")
  })

  var muted = ctx.mutedColor ? "color:" + ctx.mutedColor : ""
  var quoteMark = plain ? "" : stash.put((muted ? span(muted, "&#9613;") : "&#9613;") + " ")
  var bullet = plain ? "- " : stash.put("&#8226; ")

  var lines = text.split("\n")
  var quoteRest = false
  for (var i = 0; i < lines.length; i++) {
    var line = lines[i]
    var m
    if (!quoteRest && /^&gt;&gt;&gt; /.test(line)) {
      quoteRest = true
      line = line.slice(13)
    } else if (!quoteRest && (m = /^&gt; ?(.*)$/.exec(line))) {
      line = m[1]
    }
    var quoted = quoteRest || m
    if ((m = /^(#{1,3}) +(.*)$/.exec(line))) {
      var level = m[1].length
      line = plain ? m[2]
        : stash.put("<span style=\"font-size:" + headerPx(ctx, level) + "px\"><b>")
          + m[2] + stash.put("</b></span>")
    } else if ((m = /^(\s*)[-*] +(.*)$/.exec(line))) {
      line = (plain ? "" : m[1].replace(/ /g, "&nbsp;")) + bullet + m[2]
    }
    if (quoted) line = quoteMark + line
    lines[i] = line
    m = null
  }
  return lines.join("\n")
}

function convert(content, ctx, plain) {
  ctx = ctx || {}
  var text = String(content === undefined || content === null ? "" : content)
  text = text.replace(/\r\n?/g, "\n").replace(/[\u0001\u0002]/g, "")
  text = escapeHtml(text)
  var stash = new Stash()
  text = blocks(text, ctx, stash, plain)
  text = protectInline(text, ctx, stash, plain)
  text = formatInline(text, ctx, stash, plain)
  if (!plain) text = text.replace(/\n/g, "<br>")
  text = stash.restore(text)
  return text
}

function unescapeHtml(text) {
  return String(text)
    .replace(/&quot;/g, "\"")
    .replace(/&gt;/g, ">")
    .replace(/&lt;/g, "<")
    .replace(/&amp;/g, "&")
}

function render(content, ctx) {
  try {
    return convert(content, ctx, false)
  } catch (e) {
    try {
      return escapeHtml(String(content === undefined || content === null ? "" : content))
        .replace(/\n/g, "<br>")
    } catch (e2) {
      return ""
    }
  }
}

function plainText(content, ctx) {
  try {
    return unescapeHtml(convert(content, ctx, true))
      .replace(/|/g, "")
      .replace(/\s+/g, " ")
      .trim()
  } catch (e) {
    try { return String(content || "") } catch (e2) { return "" }
  }
}

function firstLink(message) {
  try {
    if (!message || typeof message !== "object") return ""
    var content = str(message.content)
    var masked = /\[[^\]\n]+\]\((https?:\/\/(?:[^\s()]|\([^\s()]*\))+)\)/.exec(content)
    var bare = URL_RE.exec(content)
    URL_RE.lastIndex = 0
    if (masked && (!bare || masked.index <= bare.index)) return masked[1]
    if (bare) return trimUrl(bare[0])
    var attachments = Array.isArray(message.attachments) ? message.attachments : []
    for (var i = 0; i < attachments.length; i++) {
      var url = str(attachments[i] && attachments[i].url)
      if (url) return url
    }
    var embeds = Array.isArray(message.embeds) ? message.embeds : []
    for (var j = 0; j < embeds.length; j++) {
      var eurl = str(embeds[j] && embeds[j].url)
      if (eurl) return eurl
    }
    return ""
  } catch (e) {
    return ""
  }
}

function formatSize(bytes) {
  var n = Number(bytes) || 0
  if (n < 1024) return n + " B"
  if (n < 1024 * 1024) return (n / 1024).toFixed(1) + " KB"
  if (n < 1024 * 1024 * 1024) return (n / (1024 * 1024)).toFixed(1) + " MB"
  return (n / (1024 * 1024 * 1024)).toFixed(2) + " GB"
}

if (typeof module !== "undefined" && module.exports) {
  module.exports = { render: render, plainText: plainText, firstLink: firstLink,
    escapeHtml: escapeHtml, formatTimestamp: formatTimestamp, formatSize: formatSize }
}
