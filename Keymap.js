// The keymap, once. The panel footer hints and the Ctrl+/ cheatsheet both
// render from ENTRIES, so they cannot drift: a footer state is a list of
// entry ids (FOOTER), and the cheatsheet is ENTRIES grouped by ZONES.
//
// entry: { id, zone, keys, action, hint?, hintKeys? }
//   keys     - as shown in the cheatsheet ("Alt+↑ / Alt+↓")
//   action   - full sentence for the cheatsheet
//   hint     - short verb phrase for the footer (defaults to `action`)
//   hintKeys - shorter key text for the footer (defaults to `keys`)
//   cheatsheet: false - footer-only variant of a sibling entry

var ZONES = [
  { id: "global", title: "Anywhere in the panel" },
  { id: "sidebar", title: "Sidebar — servers and channels" },
  { id: "timeline", title: "Timeline — cursor on a message" },
  { id: "composer", title: "Composer" },
  { id: "members", title: "Member list" },
  { id: "switcher", title: "Quick switcher" },
  { id: "picker", title: "Emoji picker" },
  { id: "login", title: "Login screen" }
]

var ENTRIES = [
  // --- global ---
  { id: "global.switcher", zone: "global", keys: "Ctrl+K · /", hintKeys: "Ctrl+K", hint: "switcher",
    action: "Quick switcher: unread channels and DMs first, then everything (/ outside text inputs; from any app via the Hyprland bind)" },
  { id: "global.cheatsheet", zone: "global", keys: "Ctrl+/ · ?", hintKeys: "?", hint: "keys",
    action: "This cheatsheet (? outside text inputs); Esc closes it" },
  { id: "global.zone", zone: "global", keys: "Alt+h / Alt+l", hintKeys: "Alt+h/l", hint: "zone",
    action: "Move the focus zone: sidebar ↔ timeline ↔ composer (↔ member list while it is shown)" },
  { id: "global.members", zone: "global", keys: "m · Alt+m", hintKeys: "m", hint: "members",
    action: "Show / hide the member list (m outside text inputs, Alt+m in the composer too)" },
  { id: "global.membersAlt", zone: "global", cheatsheet: false, keys: "Alt+m", hint: "members",
    action: "Show / hide the member list" },
  { id: "global.zoneLeft", zone: "global", cheatsheet: false, keys: "Alt+h", hint: "timeline",
    action: "Focus zone to the left" },
  { id: "global.zoneLeftComposer", zone: "global", cheatsheet: false, keys: "Alt+h", hint: "composer",
    action: "Focus zone to the left" },
  { id: "global.zoneRight", zone: "global", cheatsheet: false, keys: "Alt+l", hint: "timeline",
    action: "Focus zone to the right" },
  { id: "global.zoneComposer", zone: "global", cheatsheet: false, keys: "Alt+l", hint: "composer",
    action: "Focus zone to the right" },
  { id: "global.zoneMembers", zone: "global", cheatsheet: false, keys: "Alt+l", hint: "members",
    action: "Focus zone to the right" },
  { id: "global.channelStep", zone: "global", keys: "Alt+↑ / Alt+↓", hintKeys: "Alt+↑/↓", hint: "channel (Shift: unread)",
    action: "Previous / next channel in the list; add Shift to jump between unread channels only" },
  { id: "global.tab", zone: "global", keys: "Tab / Shift+Tab", hintKeys: "Tab", hint: "reaches buttons",
    action: "Cycle rail → channels → timeline → composer (and its attachments) → member list → Members → Log out → Close" },
  { id: "global.tabCycle", zone: "global", cheatsheet: false, keys: "Tab / Shift+Tab", hintKeys: "Tab/Shift+Tab", hint: "cycle",
    action: "Cycle the focus stops" },
  { id: "global.reload", zone: "global", keys: "r", hint: "reloads",
    action: "Reload state, the channel list and the open channel (outside text inputs)" },
  { id: "global.retry", zone: "global", keys: "r", hint: "retries",
    action: "While the backend is down: start it, or re-pull state" },
  { id: "global.esc", zone: "global", keys: "Esc", hint: "closes",
    action: "Walk back out: composer → timeline (marking read) → channel list → server rail → close the panel" },
  { id: "global.activate", zone: "global", cheatsheet: false, keys: "Enter", hint: "activates",
    action: "Activate the focused button" },
  { id: "global.escBack", zone: "global", keys: "Esc", hint: "back to the last zone",
    action: "From a header button: back to the last zone" },

  // --- sidebar ---
  { id: "rail.move", zone: "sidebar", keys: "j / k · ↑ / ↓", hintKeys: "j/k", hint: "move",
    action: "Move the cursor (servers in the rail, channels in the list)" },
  { id: "rail.ends", zone: "sidebar", keys: "g / G · Home / End", hintKeys: "g/G", hint: "first/last",
    action: "First / last row" },
  { id: "rail.enter", zone: "sidebar", keys: "Enter · l · →", hintKeys: "Enter/l", hint: "opens channels",
    action: "Rail: open the server's channel list" },
  { id: "channels.open", zone: "sidebar", keys: "Enter", hint: "opens channel",
    action: "Channel list: open the channel or thread (the composer takes focus); on a forum: show its threads" },
  { id: "channels.threads", zone: "sidebar", keys: "t", hint: "threads",
    action: "Channel list: show / hide the channel's active threads beneath it (from the timeline: the open channel's)" },
  { id: "timeline.threads", zone: "timeline", cheatsheet: false, keys: "t", hint: "threads",
    action: "Channel list: show / hide the channel's active threads beneath it (from the timeline: the open channel's)" },
  { id: "channels.back", zone: "sidebar", keys: "h · ← · Esc", hintKeys: "h/Esc", hint: "servers",
    action: "Channel list: back to the server rail" },
  { id: "channels.timeline", zone: "sidebar", keys: "l · →", hintKeys: "l", hint: "timeline",
    action: "Channel list: focus the open channel's timeline" },

  // --- timeline ---
  { id: "timeline.move", zone: "timeline", keys: "j / k · ↑ / ↓", hintKeys: "j/k", hint: "move",
    action: "Move the message cursor (k past the top loads history)" },
  { id: "timeline.ends", zone: "timeline", keys: "gg / G · Home / End", hintKeys: "gg/G", hint: "top/newest",
    action: "Oldest loaded message (paging history) / newest message" },
  { id: "timeline.page", zone: "timeline", keys: "PgUp / PgDn", hint: "page",
    action: "Scroll a page (PgUp at the top loads history)" },
  { id: "timeline.reply", zone: "timeline", keys: "R", hint: "reply",
    action: "Reply to the message (the composer enters reply mode)" },
  { id: "timeline.react", zone: "timeline", keys: "E", hint: "react",
    action: "React: opens the emoji picker; the message's existing reactions come first and Enter on one toggles it" },
  { id: "timeline.delete", zone: "timeline", keys: "D D", hint: "delete yours",
    action: "Delete your own message (press D twice within 3 s)" },
  { id: "timeline.copy", zone: "timeline", keys: "Y", hint: "copy",
    action: "Copy the message text (plus attachment URLs) to the clipboard" },
  { id: "timeline.open", zone: "timeline", keys: "O", hint: "open link",
    action: "Open the first link, attachment (from the cache when present) or embed" },
  { id: "timeline.enter", zone: "timeline", keys: "Enter", hint: "reveals spoilers",
    action: "Reveal the message's spoiler images" },
  { id: "timeline.esc", zone: "timeline", keys: "Esc", hint: "marks read, back to sidebar",
    action: "Mark the channel read and go back to the channel list (cancels an armed delete first)" },

  // --- composer ---
  { id: "composer.send", zone: "composer", keys: "Enter", hint: "sends",
    action: "Send (uploads the staged attachments when there are any)" },
  { id: "composer.newline", zone: "composer", keys: "Shift+Enter", hint: "newline",
    action: "Insert a newline" },
  { id: "composer.editLast", zone: "composer", keys: "↑", hint: "edits your last message",
    action: "In an empty input: edit your newest message" },
  { id: "composer.paste", zone: "composer", keys: "Ctrl+V", hint: "pastes an image",
    action: "Paste: an image on the clipboard is staged as an attachment, text pastes normally" },
  { id: "composer.chips", zone: "composer", keys: "Tab · ← / →", hintKeys: "Tab", hint: "reaches attachments",
    action: "Move from the input onto the staged attachments (← at the start / → at the end too)" },
  { id: "composer.esc", zone: "composer", keys: "Esc", hint: "marks read, back to timeline",
    action: "Cancel edit or reply mode first, else mark the channel read and focus the timeline" },
  { id: "composer.save", zone: "composer", keys: "Enter", hint: "saves the edit",
    action: "Edit mode: save" },
  { id: "composer.cancel", zone: "composer", keys: "Esc", hint: "cancels",
    action: "Edit / reply mode: cancel (a stashed draft comes back)" },
  { id: "chips.move", zone: "composer", keys: "← / → · h / l", hintKeys: "←/→", hint: "move between attachments",
    action: "On an attachment: move between the chips" },
  { id: "chips.remove", zone: "composer", keys: "x · Delete", hintKeys: "x", hint: "removes",
    action: "On an attachment: remove it (and its staged file)" },
  { id: "chips.esc", zone: "composer", keys: "Esc", hint: "back to the input",
    action: "On an attachment: back to the text input" },

  // --- member list ---
  { id: "members.move", zone: "members", keys: "j / k · ↑ / ↓", hintKeys: "j/k", hint: "move",
    action: "Move the cursor over the members (g / G: first / last)" },
  { id: "members.copy", zone: "members", keys: "Y", hint: "copies @username",
    action: "Copy the member's @username to the clipboard" },
  { id: "members.esc", zone: "members", keys: "Esc", hint: "back to the composer",
    action: "Back to the composer (Alt+h too)" },

  // --- quick switcher ---
  { id: "switcher.type", zone: "switcher", keys: "type", hint: "to search",
    action: "Type to search channels and DMs (empty: unread first, then recent)" },
  { id: "switcher.move", zone: "switcher", keys: "↑ / ↓ · Tab · Ctrl+j / Ctrl+k · Ctrl+n / Ctrl+p", hintKeys: "↑/↓", hint: "move",
    action: "Move the cursor (plain j/k type into the search)" },
  { id: "switcher.open", zone: "switcher", keys: "Enter", hint: "opens",
    action: "Open the channel in the panel" },
  { id: "switcher.esc", zone: "switcher", keys: "Esc", hint: "closes",
    action: "Clear the search, then close" },

  // --- emoji picker ---
  { id: "picker.type", zone: "picker", keys: "type", hint: "filters",
    action: "Type to filter by name" },
  { id: "picker.move", zone: "picker", keys: "arrows · Ctrl+h/j/k/l", hintKeys: "arrows", hint: "move",
    action: "Move across the grid: the message's reactions first, then frequently used, then everything (plain hjkl type into the filter)" },
  { id: "picker.pick", zone: "picker", keys: "Enter", hint: "reacts / toggles",
    action: "React with the emoji; on one of the message's existing reactions: add or remove yours" },
  { id: "picker.esc", zone: "picker", keys: "Esc", hint: "closes",
    action: "Clear the filter, then close" },

  // --- login ---
  { id: "login.activate", zone: "login", keys: "Enter", hint: "activates",
    action: "Activate the focused button / submit the token" },
  { id: "login.tab", zone: "login", keys: "Tab", hint: "cycles Scan QR, token, Log in, Close",
    action: "Cycle Scan QR → token field → Log in → Close" },
  { id: "login.esc", zone: "login", keys: "Esc", hint: "closes",
    action: "Close the panel" },
  { id: "qr.cancel", zone: "login", keys: "Esc", hint: "cancels",
    action: "QR view: cancel the running flow" },
  { id: "qr.dismiss", zone: "login", keys: "Esc", hint: "dismisses",
    action: "QR view: dismiss a finished flow" },
  { id: "qr.again", zone: "login", keys: "Enter", hint: "tries again",
    action: "QR view: start a new code" },
  { id: "qr.tab", zone: "login", keys: "Tab", hint: "reaches Close",
    action: "QR view: Tab reaches Close" }
]

// Footer states → entry ids. Panel.qml picks a state (and appends optional
// ids for situational hints); every id must resolve in ENTRIES.
var FOOTER = {
  login: ["login.activate", "login.tab", "login.esc"],
  qrRunning: ["qr.cancel", "qr.tab"],
  qrDone: ["qr.again", "qr.dismiss", "qr.tab"],
  qrMissing: ["qr.again", "qr.cancel", "qr.tab"],
  down: ["global.retry", "global.tab", "global.esc"],
  buttons: ["global.activate", "global.tabCycle", "global.escBack"],
  composer: ["composer.send", "composer.newline", "composer.editLast", "composer.paste"],
  composerTail: ["global.zoneLeft", "composer.esc"],
  composerMembers: ["global.zoneMembers", "global.membersAlt"],
  members: ["members.move", "members.copy", "global.zoneLeftComposer", "global.members", "members.esc"],
  composerChips: ["composer.chips"],
  composerEdit: ["composer.save", "composer.newline", "composer.cancel"],
  chips: ["chips.move", "chips.remove", "composer.send", "chips.esc"],
  timeline: ["timeline.move", "timeline.ends", "timeline.reply", "timeline.react", "timeline.delete",
    "timeline.copy", "timeline.open", "timeline.threads", "global.members", "global.channelStep", "global.zoneComposer", "timeline.esc"],
  rail: ["rail.move", "rail.enter", "global.switcher", "global.cheatsheet", "global.reload", "global.tab", "global.esc"],
  channels: ["rail.move", "channels.open", "channels.threads", "channels.back", "global.channelStep"],
  channelsTimeline: ["channels.timeline"],
  channelsTail: ["global.members", "global.switcher", "global.cheatsheet", "global.reload", "global.tab"],
  switcher: ["switcher.type", "switcher.move", "switcher.open", "switcher.esc"],
  picker: ["picker.type", "picker.move", "picker.pick", "picker.esc"]
}

var _byId = null

function entry(id) {
  if (!_byId) {
    _byId = {}
    for (var i = 0; i < ENTRIES.length; i++) _byId[ENTRIES[i].id] = ENTRIES[i]
  }
  return _byId[String(id || "")] || null
}

// "keys hint · keys hint · …" for a list of ids. An element may also be
// { id, hint } to override the hint text of that entry for a situational
// footer ("Esc back to the composer"). Unknown ids render as "?id" so a
// typo is visible in the footer instead of silently dropped.
function hints(ids) {
  var list = Array.isArray(ids) ? ids : []
  var out = []
  for (var i = 0; i < list.length; i++) {
    var item = list[i]
    var id = item && typeof item === "object" ? item.id : item
    var e = entry(id)
    if (!e) { out.push("?" + id); continue }
    var hint = item && typeof item === "object" && item.hint ? item.hint : (e.hint || e.action)
    out.push((e.hintKeys || e.keys) + " " + hint)
  }
  return out.join(" · ")
}

// Footer text for a state, plus any extra states / ids / { id, hint }
// overrides appended in order. Empty arguments are skipped.
function footer(state /*, ...more states, ids or overrides */) {
  var ids = []
  for (var a = 0; a < arguments.length; a++) {
    var item = arguments[a]
    if (!item) continue
    if (typeof item === "object") { ids.push(item); continue }
    var name = String(item)
    if (FOOTER[name]) ids = ids.concat(FOOTER[name])
    else ids.push(name)
  }
  return hints(ids)
}

// Every footer id that has no entry (harness assertion: must be empty).
function missingFooterIds() {
  var missing = []
  for (var state in FOOTER) {
    var ids = FOOTER[state]
    for (var i = 0; i < ids.length; i++) if (!entry(ids[i])) missing.push(state + ":" + ids[i])
  }
  return missing
}

// Cheatsheet model: [{ id, title, rows: [{ keys, action }] }], zone order.
function sections() {
  var out = []
  for (var z = 0; z < ZONES.length; z++) {
    var rows = []
    for (var i = 0; i < ENTRIES.length; i++) {
      var e = ENTRIES[i]
      if (e.zone !== ZONES[z].id || e.cheatsheet === false) continue
      // Footer-only variants share keys+action with a sibling; skip repeats.
      var dup = false
      for (var r = 0; r < rows.length; r++) if (rows[r].keys === e.keys && rows[r].action === e.action) dup = true
      if (!dup) rows.push({ keys: e.keys, action: e.action })
    }
    out.push({ id: ZONES[z].id, title: ZONES[z].title, rows: rows })
  }
  return out
}
