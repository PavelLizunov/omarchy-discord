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
