// Emoji picker helpers. The catalogue normally comes from the shell's own
// data file ($OMARCHY_PATH/shell/plugins/emojis/emojis.json, entries
// {e: emoji, k: "keywords"}); FALLBACK is a compact set in the same shape
// for when that file is unavailable.

var FALLBACK = [
  { e: "👍", k: "thumbs up +1 yes ok" }, { e: "👎", k: "thumbs down -1 no" },
  { e: "❤️", k: "red heart love" }, { e: "😂", k: "joy tears laugh lol" },
  { e: "😀", k: "grinning smile happy" }, { e: "😄", k: "smile happy laugh" },
  { e: "😅", k: "sweat smile" }, { e: "🤣", k: "rofl rolling laughing" },
  { e: "🙂", k: "slightly smiling" }, { e: "😉", k: "wink" },
  { e: "😍", k: "heart eyes love" }, { e: "😎", k: "sunglasses cool" },
  { e: "🤔", k: "thinking hmm" }, { e: "😐", k: "neutral meh" },
  { e: "🙄", k: "eye roll" }, { e: "😬", k: "grimace awkward" },
  { e: "😢", k: "cry sad tear" }, { e: "😭", k: "sob crying loudly" },
  { e: "😡", k: "angry pouting mad" }, { e: "🤯", k: "mind blown exploding head" },
  { e: "🥳", k: "party celebrate" }, { e: "🤗", k: "hug hugging" },
  { e: "🫡", k: "salute" }, { e: "🙏", k: "pray please thanks folded hands" },
  { e: "👀", k: "eyes looking" }, { e: "👋", k: "wave hello bye" },
  { e: "👏", k: "clap applause" }, { e: "🙌", k: "raised hands hooray" },
  { e: "💪", k: "flex muscle strong" }, { e: "🤝", k: "handshake deal" },
  { e: "✅", k: "check done yes" }, { e: "❌", k: "cross no wrong" },
  { e: "⭐", k: "star" }, { e: "🔥", k: "fire lit hot" },
  { e: "🎉", k: "tada party popper celebrate" }, { e: "💯", k: "hundred 100" },
  { e: "🚀", k: "rocket ship launch" }, { e: "💀", k: "skull dead" },
  { e: "🫠", k: "melting" }, { e: "😴", k: "sleeping zzz" },
  { e: "☕", k: "coffee hot beverage" }, { e: "🍕", k: "pizza" },
  { e: "🐛", k: "bug" }, { e: "🐧", k: "penguin linux" },
  { e: "💡", k: "light bulb idea" }, { e: "📌", k: "pushpin pin" },
  { e: "🔧", k: "wrench fix" }, { e: "⚠️", k: "warning" },
  { e: "❓", k: "question mark" }, { e: "❗", k: "exclamation" },
  { e: "➕", k: "plus add" }, { e: "➖", k: "minus remove" },
  { e: "🟢", k: "green circle" }, { e: "🔴", k: "red circle" },
  { e: "1️⃣", k: "one keycap 1" }, { e: "2️⃣", k: "two keycap 2" },
  { e: "3️⃣", k: "three keycap 3" }
]

var FREQUENT_CAP = 16

function parseCatalog(raw) {
  try {
    var data = JSON.parse(String(raw || ""))
    if (!Array.isArray(data)) return []
    var out = []
    for (var i = 0; i < data.length; i++)
      if (data[i] && data[i].e) out.push({ e: String(data[i].e), k: String(data[i].k || "") })
    return out
  } catch (e) {
    return []
  }
}

// Keyword match, ranked: the canonical shortcode (first keyword) equal to
// the query first, then any whole keyword equal to it, then a keyword
// starting with it, then any substring ("fire" puts 🔥 before 🚒 and
// ❤️‍🔥). Catalogue order is kept within a rank; an empty query keeps
// everything. `limit` caps the result (0 = unlimited).
function rank(item, needle) {
  if (item.e === needle) return 0
  var words = String(item.k || "").toLowerCase().split(" ")
  if (words[0] === needle) return 0
  var best = -1
  for (var i = 0; i < words.length; i++) {
    if (words[i] === needle) return 1
    if (words[i].indexOf(needle) === 0) best = best < 0 || best > 2 ? 2 : best
    else if (best < 0 && words[i].indexOf(needle) >= 0) best = 3
  }
  return best
}

function filter(catalog, query, limit) {
  var list = Array.isArray(catalog) ? catalog : []
  var needle = String(query || "").trim().toLowerCase()
  var max = Math.max(0, Number(limit) || 0)
  var out = []
  if (!needle) {
    for (var j = 0; j < list.length; j++) {
      if (!list[j] || !list[j].e) continue
      out.push(list[j])
      if (max && out.length >= max) break
    }
    return out
  }
  var buckets = [[], [], [], []]
  for (var i = 0; i < list.length; i++) {
    var item = list[i]
    if (!item || !item.e) continue
    var r = rank(item, needle)
    if (r >= 0) buckets[r].push(item)
  }
  out = buckets[0].concat(buckets[1], buckets[2], buckets[3])
  return max && out.length > max ? out.slice(0, max) : out
}

// Frequently used: [{ e, n }] sorted by use count, most recent first on
// ties, capped. Persisted as a JSON string on the plugin's shell.json entry.
function parseFrequent(raw) {
  try {
    var data = JSON.parse(String(raw || ""))
    if (!Array.isArray(data)) return []
    var out = []
    for (var i = 0; i < data.length && out.length < FREQUENT_CAP; i++) {
      var item = data[i]
      if (!item || !item.e) continue
      out.push({ e: String(item.e), n: Math.max(1, Math.floor(Number(item.n)) || 1) })
    }
    return out
  } catch (e) {
    return []
  }
}

function bumpFrequent(list, emoji) {
  var value = String(emoji || "")
  if (!value) return Array.isArray(list) ? list : []
  var out = []
  var count = 1
  var source = Array.isArray(list) ? list : []
  for (var i = 0; i < source.length; i++) {
    if (!source[i] || source[i].e === value) { if (source[i]) count = (Number(source[i].n) || 0) + 1; continue }
    out.push({ e: source[i].e, n: Number(source[i].n) || 1 })
  }
  // Most recent first among equals: insert before the first entry with a
  // lower-or-equal count.
  var at = out.length
  for (var j = 0; j < out.length; j++) if (out[j].n <= count) { at = j; break }
  out.splice(at, 0, { e: value, n: count })
  return out.slice(0, FREQUENT_CAP)
}

// A reaction's emoji on the wire is unicode or "name:id" (custom); the
// picker shows custom ones as :name: and sends them back unchanged.
function customEmoji(emoji) {
  var m = /^([^:]+):(\d+)$/.exec(String(emoji || ""))
  return m ? { name: m[1], id: m[2] } : null
}

function displayName(emoji) {
  var custom = customEmoji(emoji)
  return custom ? ":" + custom.name + ":" : String(emoji || "")
}

// Wire form of a catalogue item or custom emoji for react/unreact.
function wire(item) {
  if (!item) return ""
  if (item.id && item.name) return String(item.name) + ":" + String(item.id)
  return String(item.e || item.emoji || "")
}

// emoji -> keywords for the catalogue, so the reaction / frequent sections
// can be filtered by the same names as the grid.
function keywordIndex(catalog) {
  var map = {}
  var list = Array.isArray(catalog) ? catalog : []
  for (var i = 0; i < list.length; i++) if (list[i] && list[i].e) map[list[i].e] = String(list[i].k || "")
  return map
}

function matches(cell, needle, keywords) {
  if (!needle) return true
  var name = String(cell.label || "").toLowerCase()
  if (name.indexOf(needle) >= 0 || cell.emoji === needle) return true
  var k = keywords && keywords[cell.emoji] ? keywords[cell.emoji].toLowerCase() : ""
  return k.indexOf(needle) >= 0
}

// Picker model. Sections in display order, empty ones dropped:
//   reactions — the message's existing reactions ("Toggle": me => unreact)
//   frequent  — persisted frequently-used list
//   server    — custom emoji of the guild (list_emoji, next wave)
//   all       — the unicode catalogue
// Each cell: { emoji (wire form), label, custom (id or ""), toggle, me }.
// `query` filters every section by name / keyword.
function sections(reactions, frequent, server, catalog, query, limit) {
  var needle = String(query || "").trim().toLowerCase()
  var keywords = keywordIndex(catalog)
  var out = []
  var cells

  cells = []
  var rx = Array.isArray(reactions) ? reactions : []
  for (var r = 0; r < rx.length; r++) {
    var value = String(rx[r] && rx[r].emoji || "")
    if (!value) continue
    var custom = customEmoji(value)
    var cell = { emoji: value, label: displayName(value), custom: custom ? custom.id : "", toggle: true, me: !!rx[r].me,
      count: Math.max(0, Number(rx[r].count) || 0) }
    if (matches(cell, needle, keywords)) cells.push(cell)
  }
  if (cells.length) out.push({ id: "reactions", title: "Toggle", cells: cells })

  cells = []
  var fq = Array.isArray(frequent) ? frequent : []
  for (var f = 0; f < fq.length; f++) {
    var fe = String(fq[f] && fq[f].e || "")
    if (!fe) continue
    var fc = customEmoji(fe)
    var fcell = { emoji: fe, label: displayName(fe), custom: fc ? fc.id : "", toggle: false, me: false, count: 0 }
    if (matches(fcell, needle, keywords)) cells.push(fcell)
  }
  if (cells.length) out.push({ id: "frequent", title: "Frequently used", cells: cells })

  cells = []
  var sv = Array.isArray(server) ? server : []
  for (var s = 0; s < sv.length; s++) {
    if (!sv[s] || !sv[s].id || !sv[s].name) continue
    var scell = { emoji: wire(sv[s]), label: ":" + String(sv[s].name) + ":", custom: String(sv[s].id), toggle: false, me: false, count: 0 }
    if (matches(scell, needle, keywords)) cells.push(scell)
  }
  if (cells.length) out.push({ id: "server", title: "Server emoji", cells: cells })

  cells = []
  var all = filter(catalog, needle, limit)
  for (var a = 0; a < all.length; a++)
    cells.push({ emoji: all[a].e, label: String(all[a].k || "").split(" ")[0] || all[a].e, custom: "", toggle: false, me: false, count: 0 })
  if (cells.length) out.push({ id: "all", title: needle ? "Matches" : "Emoji", cells: cells })
  return out
}

// Flat cell list over sections: [{ section, index, cell }].
function flatten(sections) {
  var out = []
  var list = Array.isArray(sections) ? sections : []
  for (var s = 0; s < list.length; s++)
    for (var i = 0; i < list[s].cells.length; i++) out.push({ section: s, index: i, cell: list[s].cells[i] })
  return out
}

// Grid navigation over the flat list. dx moves through the flat order
// (wrapping), dy moves a row within the section and crosses into the
// neighbouring section at its edge, keeping the column when it can.
function move(sections, flat, from, dx, dy, columns) {
  var count = flat.length
  if (!count) return -1
  var cols = Math.max(1, Number(columns) || 1)
  var at = from < 0 || from >= count ? 0 : from
  if (dx) return ((at + dx) % count + count) % count
  if (!dy) return at
  var pos = flat[at]
  var base = at - pos.index
  var col = pos.index % cols
  var size = sections[pos.section].cells.length
  var lastRow = Math.floor((size - 1) / cols)
  var target = pos.index + dy * cols
  if (target >= 0 && target < size) return base + target
  // Down onto a short last row: land on its last cell.
  if (dy > 0 && Math.floor(target / cols) === lastRow) return base + size - 1
  var section = pos.section + (dy > 0 ? 1 : -1)
  if (section < 0 || section >= sections.length)
    return base + (dy > 0 ? Math.min(size - 1, lastRow * cols + col) : col)
  var cells = sections[section].cells.length
  var nextBase = dy > 0 ? base + size : base - cells
  var row = dy > 0 ? 0 : Math.floor((cells - 1) / cols)
  return nextBase + Math.min(cells - 1, row * cols + col)
}
