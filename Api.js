// Stateless helpers shared by the Discord plugin's QML files. Kept out of
// bindings so heavy or security-relevant logic has one home.

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

function parseJson(text, fallback) {
  try {
    var parsed = JSON.parse(String(text || ""))
    return parsed === null ? fallback : parsed
  } catch (e) {
    return fallback
  }
}

// Every error string that reaches the UI passes through here. Adapted from
// quickshell.spotify's Api.redact; the field list adds Discord's token, QR
// ticket, and encrypted_token.
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

// The backend's media cache only fetches Discord's CDN hosts; anything else
// (embed images off-site) is not even requested.
function isCdnUrl(url) {
  return /^https:\/\/(cdn\.discordapp\.com|media\.discordapp\.net)\//i.test(String(url || ""))
}

// --- theme helpers ---
// Relative luminance of a QColor (WCAG), 0..1.
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

// Secondary-text colour. Themes define `muted` for their own purposes and
// the shell never paints text with it; on several bundled themes (rose-pine,
// catppuccin-latte, flexoki-light, tokyo-night…) it sits at a 1.5–2.5
// contrast ratio against the background, unreadable as text. When it clears
// 3:1 it is used as-is, otherwise the foreground at 60 % alpha stands in —
// the same construction the shell uses for its own placeholder text.
var SECONDARY_MIN_CONTRAST = 3.0
var SECONDARY_ALPHA = 0.6

function secondaryColor(muted, foreground, background) {
  if (muted && background && contrastRatio(muted, background) >= SECONDARY_MIN_CONTRAST) return muted
  if (!foreground || foreground.r === undefined) return muted
  return Qt.rgba(foreground.r, foreground.g, foreground.b, SECONDARY_ALPHA)
}

// Opaque composite of `color` at `alpha` over `background` (for rich text,
// where a translucent colour used as both ink and cover would show through).
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
    case "announcement": return ""
    case "voice": return ""
    case "forum": return ""
    case "thread": return ""
    case "category": return ""
    case "dm": return "@"
    case "group_dm": return "@@"
    default: return "#"
  }
}

// Snowflake ids are decimal strings too wide for a double; compare by length
// then lexically.
function compareIds(a, b) {
  var x = String(a || "")
  var y = String(b || "")
  if (x.length !== y.length) return x.length - y.length
  return x < y ? -1 : (x > y ? 1 : 0)
}

// Guild rail label: first letters of up to three words ("Omarchy Dev" -> "OD").
function initials(name) {
  var words = String(name || "").trim().split(/[\s\-_]+/).filter(function(w) { return w.length > 0 })
  var out = ""
  for (var i = 0; i < words.length && i < 3; i++) out += words[i].charAt(0).toUpperCase()
  return out || "?"
}

// Sidebar filter: voice/stage channels are a non-goal and hidden entirely;
// a category whose visible children are all hidden goes with them. Thread
// rows (list_channels carries every active thread the cache knows — hundreds
// on a busy guild) never sit in the flat list: they are shown under their
// parent on demand (Panel.channelRows / Service.threadsFor).
function isHiddenChannelType(type) {
  var t = String(type || "")
  return t === "voice" || t === "stage" || t === "thread"
}

// Channel types that can carry threads (the `t` affordance).
function hasThreads(type) {
  var t = String(type || "")
  return t === "text" || t === "announcement" || t === "forum"
}

// An archived thread is not an active one: list_threads drops it and the
// flat list must not count it. `archived` is an additive wire field, so a
// channel object without it is live.
function isActiveThread(row) {
  return !!row && String(row.type || "") === "thread" && row.archived !== true
}

// parent id -> number of active threads, from a raw list_channels result.
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

// Thread rows of one parent from a raw list_channels result, newest activity
// first (the list_threads order), used until list_threads answers.
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

// Enter opens these (threads included); a forum only expands its threads.
function isOpenableChannel(row) {
  if (!row) return false
  var t = String(row.type || "")
  return t !== "category" && t !== "forum" && t !== "voice" && t !== "stage"
}

// The sidebar cursor lands on these (thread rows under an expanded parent too).
function isSelectableChannel(row) {
  if (!row) return false
  var t = String(row.type || "")
  return t !== "category" && t !== "voice" && t !== "stage"
}

// Member pane rows from a member_list_update: every group as a header
// (Discord's total count), its served members beneath it, in wire order.
// Members whose group is unknown get a header named after the group id.
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

// Rows in a timeline window: optimistic rows ("pending-N") sort after every
// real snowflake, otherwise by id.
function compareRows(a, b) {
  var ap = !!(a && a.pending)
  var bp = !!(b && b.pending)
  if (ap !== bp) return ap ? 1 : -1
  return compareIds(a && a.id, b && b.id)
}

// Clipboard paste: the image type to stage from `wl-paste --list-types`
// output, preferring lossless; "" when the clipboard holds no image.
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
