
function assign(target, source) {
  var next = target && typeof target === "object" && !Array.isArray(target)
    ? target : ({})
  if (!source || typeof source !== "object" || Array.isArray(source)) return next
  for (var key in source) next[key] = source[key]
  return next
}

function shallowCopy(source) {
  return assign({}, source)
}

// Shared by server clicks and search, including retained older service instances.
function browseGuild(service, guildId) {
  var id = String(guildId || "")
  if (!service || !id) return
  var previous = service.currentChannelId
  service.pendingGuildEntry = ""
  service.currentChannelId = ""
  service.selectedGuildId = id
  if (previous) service.closeChannel(previous)
  if (id !== "dms") service.loadChannels(id)
}

function parseJson(text, fallback) {
  try {
    var parsed = JSON.parse(String(text || ""))
    return parsed === null ? fallback : parsed
  } catch (e) {
    return fallback
  }
}

var SECRET_FIELDS = "token|access_token|refresh_token|encrypted_token|ticket|code|code_verifier|client_secret|password"

function redact(value) {
  var text = String(value || "")
  text = text.replace(/(authorization\s*:\s*bearer\s+)[^\s]+/ig, "$1<redacted>")
  text = text.replace(/(authorization\s*:\s*)(?!bearer)[^\s,;]+/ig, "$1<redacted>")
  text = text.replace(new RegExp("(^|[?&\\s])((?:" + SECRET_FIELDS + ")=)[^&#\\s]+", "ig"), "$1$2<redacted>")
  text = text.replace(new RegExp("(\"(?:" + SECRET_FIELDS + ")\"\\s*:\\s*\")[^\"]+", "ig"), "$1<redacted>")
  return text
}

function onOff(value, fallback) {
  var text = String(value === undefined || value === null ? fallback : value)
  return text === "Off" ? "Off" : "On"
}

function oneOf(value, options, fallback) {
  var text = String(value === undefined || value === null ? "" : value)
  return options.indexOf(text) >= 0 ? text : fallback
}

function clampInt(value, min, max, fallback) {
  var n = Math.floor(Number(value))
  if (!isFinite(n)) n = fallback
  return Math.max(min, Math.min(max, n))
}

function luminance(color) {
  if (!color || color.r === undefined) return 0
  var chan = function(c) { return c <= 0.03928 ? c / 12.92 : Math.pow((c + 0.055) / 1.055, 2.4) }
  return 0.2126 * chan(color.r) + 0.7152 * chan(color.g) + 0.0722 * chan(color.b)
}

function contrastRatio(a, b) {
  var la = luminance(a)
  var lb = luminance(b)
  return (Math.max(la, lb) + 0.05) / (Math.min(la, lb) + 0.05)
}

var SECONDARY_MIN_CONTRAST = 4.5
var SECONDARY_ALPHA = 0.6

function textColor(preferred, foreground, background) {
  if (!background || background.r === undefined) return preferred || foreground
  var painted = function(color) { return blend(color, background, color.a === undefined ? 1 : color.a) }
  if (preferred && preferred.r !== undefined && contrastRatio(painted(preferred), background) >= SECONDARY_MIN_CONTRAST) return preferred
  if (foreground && foreground.r !== undefined && contrastRatio(painted(foreground), background) >= SECONDARY_MIN_CONTRAST) return foreground
  var black = Qt.rgba(0, 0, 0, 1), white = Qt.rgba(1, 1, 1, 1)
  return contrastRatio(black, background) > contrastRatio(white, background) ? black : white
}

function secondaryColor(muted, foreground, background) {
  if (muted && background && contrastRatio(blend(muted, background, muted.a === undefined ? 1 : muted.a), background) >= SECONDARY_MIN_CONTRAST) return muted
  if (!foreground || foreground.r === undefined) return muted
  var secondary = Qt.rgba(foreground.r, foreground.g, foreground.b, SECONDARY_ALPHA)
  if (background && contrastRatio(blend(secondary, background, secondary.a), background) < SECONDARY_MIN_CONTRAST)
    return textColor(foreground, foreground, background)
  return secondary
}

var AUTHOR_HUES_DARK = ["#68b6ef", "#5ec99b", "#e5a952", "#ba8fff", "#ef7f7f", "#4dd0e1", "#fbc02d"]
var AUTHOR_HUES_LIGHT = ["#1565c0", "#1b5e20", "#bf360c", "#4a148c", "#880e4f", "#006064", "#556b2f"]

function authorColor(userId, background, fallback) {
  var id = String(userId || "")
  if (!id) return fallback || "#cacccc"
  var hash = 0
  for (var i = 0; i < id.length; i++) hash = (hash * 31 + id.charCodeAt(i)) >>> 0
  var dark = luminance(background) < 0.3
  var hues = dark ? AUTHOR_HUES_DARK : AUTHOR_HUES_LIGHT
  return hues[hash % hues.length]
}

function blend(color, background, alpha) {
  if (!color || color.r === undefined || !background || background.r === undefined) return color
  var a = Math.max(0, Math.min(1, Number(alpha) || 0))
  return Qt.rgba(color.r * a + background.r * (1 - a), color.g * a + background.g * (1 - a),
    color.b * a + background.b * (1 - a), 1)
}

function lifecycleLabel(lifecycle, connected) {
  if (!connected) return "Backend not running"
  switch (String(lifecycle || "")) {
    case "ready": return "Connected"
    case "connecting": return "Connecting"
    case "starting": return "Starting"
    case "logged_out": return "Logged out"
    case "reauth_needed": return "Login required"
    case "qr_pending": return "Waiting for QR scan"
    case "error": return "Error"
    default: return "Waiting for backend"
  }
}

function channelGlyph(type) {
  switch (String(type || "")) {
    case "announcement": return "!"
    case "voice": return "V"
    case "forum": return "F"
    case "thread": return ">"
    case "category": return ""
    case "dm": return "@"
    case "group_dm": return "@@"
    default: return "#"
  }
}

function compareIds(a, b) {
  var x = String(a || "")
  var y = String(b || "")
  if (x.length !== y.length) return x.length - y.length
  return x < y ? -1 : (x > y ? 1 : 0)
}

function userLabel(user, knownUsers) {
  var value = user || {}
  var id = String(value.id || "")
  return String(value.display_name || value.username || (knownUsers && knownUsers[id])
    || (id ? "User #" + id : "Unknown user"))
}

function initials(name) {
  var words = String(name || "").trim().split(/[\s\-_]+/).filter(function(w) { return w.length > 0 })
  var out = ""
  for (var i = 0; i < words.length && i < 3; i++) out += words[i].charAt(0).toUpperCase()
  return out || "?"
}

function isHiddenChannelType(type) {
  var t = String(type || "")
  return t === "stage" || t === "thread"
}

function hasThreads(type) {
  var t = String(type || "")
  return t === "text" || t === "announcement" || t === "forum"
}

function isActiveThread(row) {
  return !!row && String(row.type || "") === "thread" && row.archived !== true
}

function threadCounts(channels) {
  var out = {}
  var list = Array.isArray(channels) ? channels : []
  for (var i = 0; i < list.length; i++) {
    var row = list[i]
    if (!isActiveThread(row) || !row.parent_id) continue
    var pid = String(row.parent_id)
    out[pid] = (out[pid] || 0) + 1
  }
  return out
}

function threadsOf(channels, parentId) {
  var pid = String(parentId || "")
  var list = Array.isArray(channels) ? channels : []
  var out = []
  for (var i = 0; i < list.length; i++)
    if (isActiveThread(list[i]) && String(list[i].parent_id || "") === pid) out.push(list[i])
  out.sort(function(a, b) {
    var c = compareIds(b.last_message_id || "", a.last_message_id || "")
    return c !== 0 ? c : compareIds(b.id, a.id)
  })
  return out
}

function visibleChannels(channels) {
  var list = Array.isArray(channels) ? channels : []
  var out = []
  for (var i = 0; i < list.length; i++) {
    var row = list[i]
    if (!row || isHiddenChannelType(row.type)) continue
    if (String(row.type || "") === "category") {
      var keep = false
      for (var j = i + 1; j < list.length; j++) {
        if (String(list[j].type || "") === "category") break
        if (!isHiddenChannelType(list[j].type)) { keep = true; break }
      }
      if (!keep) continue
    }
    out.push(row)
  }
  return out
}

function isOpenableChannel(row) {
  if (!row) return false
  var t = String(row.type || "")
  return t !== "category" && t !== "forum" && t !== "voice" && t !== "stage"
}

function isSelectableChannel(row) {
  if (!row) return false
  var t = String(row.type || "")
  return t !== "category" && t !== "stage"
}

function filterChannels(channels, filter) {
  var list = Array.isArray(channels) ? channels : []
  if (!filter || filter === "all") return list
  var out = []
  for (var i = 0; i < list.length; i++) {
    var row = list[i]
    if (!row) continue
    var t = String(row.type || "")
    if (t === "category") {
      var hasChild = false
      for (var j = i + 1; j < list.length; j++) {
        if (String(list[j].type || "") === "category") break
        if (channelMatchesFilter(list[j], filter)) { hasChild = true; break }
      }
      if (hasChild) out.push(row)
      continue
    }
    if (channelMatchesFilter(row, filter)) out.push(row)
  }
  return out
}

function channelMatchesFilter(row, filter) {
  if (!row) return false
  var t = String(row.type || "")
  if (filter === "text") return t === "text" || t === "announcement" || t === "thread" || t === "forum"
  if (filter === "voice") return t === "voice" || t === "stage"
  if (filter === "unread") return isUnread(row) || (Number(row.mention_count) || 0) > 0
  return true
}

function voiceOccupants(channels, channelId) {
  var list = Array.isArray(channels) ? channels : []
  var id = String(channelId || "")
  for (var i = 0; i < list.length; i++) {
    if (!list[i] || String(list[i].channel_id || "") !== id) continue
    return Array.isArray(list[i].users) ? list[i].users : []
  }
  return []
}

var LAST_CHANNEL_CAP = 32

function parseLastChannels(raw) {
  try {
    var data = JSON.parse(String(raw || ""))
    if (!Array.isArray(data)) return []
    var out = []
    for (var i = 0; i < data.length && out.length < LAST_CHANNEL_CAP; i++) {
      var item = data[i]
      if (!item || !item.g || !item.c) continue
      out.push({ g: String(item.g), c: String(item.c) })
    }
    return out
  } catch (e) {
    return []
  }
}

function lastChannelFor(list, guildId) {
  var id = String(guildId || "")
  var source = Array.isArray(list) ? list : []
  for (var i = 0; i < source.length; i++)
    if (source[i] && String(source[i].g) === id) return String(source[i].c)
  return ""
}

function bumpLastChannel(list, guildId, channelId) {
  var g = String(guildId || "")
  var c = String(channelId || "")
  var source = Array.isArray(list) ? list : []
  if (!g || !c) return source
  if (source.length && source[0] && String(source[0].g) === g && String(source[0].c) === c) return source
  var out = [{ g: g, c: c }]
  for (var i = 0; i < source.length && out.length < LAST_CHANNEL_CAP; i++) {
    if (!source[i] || String(source[i].g) === g) continue
    out.push({ g: String(source[i].g), c: String(source[i].c) })
  }
  return out
}

function guildEntryChannel(channels, remembered, allowDefault) {
  var list = Array.isArray(channels) ? channels : []
  var want = String(remembered || "")
  if (want) {
    for (var i = 0; i < list.length; i++)
      if (list[i] && String(list[i].id || "") === want && isOpenableChannel(list[i])) return want
  }
  if (!allowDefault) return ""
  var visible = visibleChannels(list)
  var first = ""
  for (var v = 0; v < visible.length; v++) {
    var row = visible[v]
    if (!isOpenableChannel(row)) continue
    if (String(row.name || "").toLowerCase() === "general") return String(row.id || "")
    if (!first) first = String(row.id || "")
  }
  return first
}

function memberRows(list) {
  var out = []
  if (!list) return out
  var groups = Array.isArray(list.groups) ? list.groups : []
  var members = Array.isArray(list.members) ? list.members : []
  var byGroup = {}
  var order = []
  for (var g = 0; g < groups.length; g++) {
    var gid = String(groups[g].id || "")
    byGroup[gid] = { id: gid, name: String(groups[g].name || gid), count: Math.max(0, Number(groups[g].count) || 0), members: [] }
    order.push(gid)
  }
  for (var m = 0; m < members.length; m++) {
    var row = members[m]
    if (!row || !row.user) continue
    var mg = String(row.group_id || "")
    if (!byGroup[mg]) { byGroup[mg] = { id: mg, name: mg || "Members", count: 0, members: [] }; order.push(mg) }
    byGroup[mg].members.push(row)
  }
  for (var o = 0; o < order.length; o++) {
    var group = byGroup[order[o]]
    out.push({ kind: "group", id: "group:" + group.id, name: group.name,
      count: Math.max(group.count, group.members.length) })
    for (var i = 0; i < group.members.length; i++) {
      var member = group.members[i]
      out.push({ kind: "member", id: String(member.user.id || ""), user: member.user,
        status: String(member.status || "offline"), activity: String(member.activity || "") })
    }
  }
  return out
}

function formatCount(n) {
  var value = Math.max(0, Math.floor(Number(n) || 0))
  var text = String(value)
  var out = ""
  while (text.length > 3) { out = "," + text.slice(-3) + out; text = text.slice(0, -3) }
  return text + out
}

function isUnread(row) {
  return !!row && String(row.unread || "read") !== "read"
}

function compareRows(a, b) {
  var ap = !!(a && a.pending)
  var bp = !!(b && b.pending)
  if (ap !== bp) return ap ? 1 : -1
  return compareIds(a && a.id, b && b.id)
}

var IMAGE_PREFERENCE = ["image/png", "image/jpeg", "image/webp", "image/gif"]

function bestImageType(types) {
  var list = Array.isArray(types) ? types.map(function(t) { return String(t || "").trim().toLowerCase() }) : []
  for (var i = 0; i < IMAGE_PREFERENCE.length; i++)
    if (list.indexOf(IMAGE_PREFERENCE[i]) >= 0) return IMAGE_PREFERENCE[i]
  for (var j = 0; j < list.length; j++)
    if (list[j].indexOf("image/") === 0 && list[j].indexOf("image/svg") !== 0) return list[j]
  return ""
}

function imageExtension(mime) {
  switch (String(mime || "")) {
    case "image/png": return "png"
    case "image/jpeg": return "jpg"
    case "image/webp": return "webp"
    case "image/gif": return "gif"
    case "image/bmp": return "bmp"
    case "image/tiff": return "tiff"
    case "image/avif": return "avif"
    default: return "img"
  }
}

if (typeof module !== "undefined" && module.exports) {
  module.exports = { visibleChannels: visibleChannels, isOpenableChannel: isOpenableChannel,
    isSelectableChannel: isSelectableChannel, voiceOccupants: voiceOccupants,
    isHiddenChannelType: isHiddenChannelType, LAST_CHANNEL_CAP: LAST_CHANNEL_CAP,
    parseLastChannels: parseLastChannels, lastChannelFor: lastChannelFor,
    bumpLastChannel: bumpLastChannel, guildEntryChannel: guildEntryChannel,
    authorColor: authorColor, contrastRatio: contrastRatio, luminance: luminance,
    filterChannels: filterChannels }
}
