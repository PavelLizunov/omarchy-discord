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
// a category whose visible children are all hidden goes with them.
function isHiddenChannelType(type) {
  var t = String(type || "")
  return t === "voice" || t === "stage"
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
  return t !== "category" && t !== "thread" && t !== "forum" && !isHiddenChannelType(t)
}

function isSelectableChannel(row) {
  return !!row && String(row.type || "") !== "category" && !isHiddenChannelType(row.type)
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
