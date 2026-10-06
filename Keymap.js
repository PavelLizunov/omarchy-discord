
var ZONES = [
  { id: "global", title: "Anywhere in the panel" },
  { id: "sidebar", title: "Sidebar — servers and channels" },
  { id: "timeline", title: "Timeline — cursor on a message" },
  { id: "composer", title: "Composer" },
  { id: "members", title: "Member list" },
  { id: "voice", title: "Voice call" },
  { id: "switcher", title: "Quick switcher" },
  { id: "picker", title: "Emoji picker" },
  { id: "login", title: "Login screen" }
]

var ENTRIES = [
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
    action: "Cycle rail → channels → timeline and message actions → composer, attachments and Send → member list → Search → Help → Members → Log out → Close" },
  { id: "global.tabCycle", zone: "global", cheatsheet: false, keys: "Tab / Shift+Tab", hintKeys: "Tab/Shift+Tab", hint: "cycle",
    action: "Cycle the focus stops" },
  { id: "global.reload", zone: "global", keys: "r", hint: "reloads",
    action: "Reload state, the channel list and the open channel (outside text inputs)" },
  { id: "global.retry", zone: "global", keys: "r", hint: "retries",
    action: "While the backend is down: start it, or re-pull state" },
  { id: "global.esc", zone: "global", keys: "Esc", hint: "back out",
    action: "Walk back out: composer → timeline (marking read) → channel list → server rail, where it stops — Esc never closes the panel" },
  { id: "global.escClose", zone: "global", cheatsheet: false, keys: "Esc", hint: "closes",
    action: "Close the panel" },
  { id: "global.activate", zone: "global", cheatsheet: false, keys: "Enter", hint: "activates",
    action: "Activate the focused button" },
  { id: "global.escBack", zone: "global", keys: "Esc", hint: "back to the last zone",
    action: "From a panel button: back to the last zone" },

  { id: "rail.move", zone: "sidebar", keys: "j / k · ↑ / ↓", hintKeys: "j/k", hint: "move",
    action: "Move the cursor (servers in the rail, channels in the list)" },
  { id: "rail.ends", zone: "sidebar", keys: "g / G · Home / End", hintKeys: "g/G", hint: "first/last",
    action: "First / last row" },
  { id: "rail.enter", zone: "sidebar", keys: "Enter · l · →", hintKeys: "Enter/l", hint: "opens channels",
    action: "Rail: open the server's channel list and the channel it was last left on (a server also falls back to #general, then its first channel; Direct messages restore only what you left open)" },
  { id: "channels.open", zone: "sidebar", keys: "Enter", hint: "opens channel",
    action: "Channel list: open the channel or thread (the composer takes focus); on a forum: show its threads" },
  { id: "sidebar.joinVoice", zone: "sidebar", keys: "Enter", hint: "joins voice",
    action: "Channel list: on a voice channel, join it — on the one you are already in, focus the call bar" },
  { id: "channels.threads", zone: "sidebar", keys: "t", hint: "threads",
    action: "Channel list: show / hide the channel's active threads beneath it (from the timeline: the open channel's)" },
  { id: "timeline.threads", zone: "timeline", cheatsheet: false, keys: "t", hint: "threads",
    action: "Channel list: show / hide the channel's active threads beneath it (from the timeline: the open channel's)" },
  { id: "channels.back", zone: "sidebar", keys: "h · ← · Esc", hintKeys: "h/Esc", hint: "servers",
    action: "Channel list: back to the server rail" },
  { id: "channels.timeline", zone: "sidebar", keys: "l · →", hintKeys: "l", hint: "timeline",
    action: "Channel list: focus the open channel's timeline" },

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
  { id: "timeline.copyLink", zone: "timeline", keys: "L", hint: "copy link",
    action: "Copy the first link, attachment or embed URL to the clipboard (a link hovered with the mouse offers a Copy link chip)" },
  { id: "timeline.links", zone: "timeline", cheatsheet: false, keys: "O / L", hint: "open/copy link",
    action: "Open or copy the first link, attachment or embed" },
  { id: "timeline.copySelection", zone: "timeline", keys: "Ctrl+C", hint: "copies selection",
    action: "Copy the highlighted text (drag the mouse over a message to highlight it); Y still copies the whole message" },
  { id: "timeline.escSelection", zone: "timeline", cheatsheet: false, keys: "Esc", hint: "clears the selection",
    action: "Clear the text selection" },
  { id: "timeline.enter", zone: "timeline", keys: "Enter", hint: "reveals spoilers",
    action: "Reveal the message's spoiler images" },
  { id: "timeline.esc", zone: "timeline", keys: "Esc", hint: "marks read, back to sidebar",
    action: "Mark the channel read and go back to the channel list (cancels an armed delete, then clears a text selection, first)" },

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

  { id: "members.move", zone: "members", keys: "j / k · ↑ / ↓", hintKeys: "j/k", hint: "move",
    action: "Move the cursor over the members (g / G: first / last)" },
  { id: "members.copy", zone: "members", keys: "Y", hint: "copies @username",
    action: "Copy the member's @username to the clipboard" },
  { id: "members.esc", zone: "members", keys: "Esc", hint: "back to the composer",
    action: "Back to the composer (Alt+h too)" },

  { id: "voice.mute", zone: "voice", keys: "Ctrl+Shift+M", hint: "mute",
    action: "Mute / unmute your microphone (from any app via the Hyprland bind)" },
  { id: "voice.deafen", zone: "voice", keys: "Ctrl+Shift+D", hint: "deafen",
    action: "Deafen / undeafen: stop playing what the others say" },
  { id: "voice.leave", zone: "voice", keys: "Ctrl+Shift+H", hint: "hangs up",
    action: "Leave the voice channel" },

  { id: "switcher.type", zone: "switcher", keys: "type", hint: "to search",
    action: "Type to search channels and DMs (empty: unread first, then recent)" },
  { id: "switcher.move", zone: "switcher", keys: "↑ / ↓ · Tab · Ctrl+j / Ctrl+k · Ctrl+n / Ctrl+p", hintKeys: "↑/↓", hint: "move",
    action: "Move the cursor (plain j/k type into the search)" },
  { id: "switcher.open", zone: "switcher", keys: "Enter", hint: "opens",
    action: "Open the channel in the panel" },
  { id: "switcher.esc", zone: "switcher", keys: "Esc", hint: "closes",
    action: "Clear the search, then close" },

  { id: "picker.type", zone: "picker", keys: "type", hint: "filters",
    action: "Type to filter by name" },
  { id: "picker.move", zone: "picker", keys: "arrows · Ctrl+h/j/k/l", hintKeys: "arrows", hint: "move",
    action: "Move across the grid: the message's reactions first, then frequently used, then everything (plain hjkl type into the filter)" },
  { id: "picker.pick", zone: "picker", keys: "Enter", hint: "reacts / toggles",
    action: "React with the emoji; on one of the message's existing reactions: add or remove yours" },
  { id: "picker.esc", zone: "picker", keys: "Esc", hint: "closes",
    action: "Clear the filter, then close" },

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

var FOOTER = {
  login: ["login.activate", "login.tab", "login.esc"],
  qrRunning: ["qr.cancel", "qr.tab"],
  qrDone: ["qr.again", "qr.dismiss", "qr.tab"],
  qrMissing: ["qr.again", "qr.cancel", "qr.tab"],
  down: ["global.retry", "global.tab", "global.escClose"],
  buttons: ["global.activate", "global.tabCycle", "global.escBack"],
  composer: ["composer.send", "composer.newline", "composer.editLast", "composer.paste"],
  composerTail: ["global.zoneLeft", "composer.esc"],
  composerMembers: ["global.zoneMembers", "global.membersAlt"],
  members: ["members.move", "members.copy", "global.zoneLeftComposer", "global.members", "members.esc"],
  voice: ["voice.mute", "voice.deafen", "voice.leave", "global.tabCycle", "global.escBack"],
  composerChips: ["composer.chips"],
  composerEdit: ["composer.save", "composer.newline", "composer.cancel"],
  chips: ["chips.move", "chips.remove", "composer.send", "chips.esc"],
  timeline: ["timeline.move", "timeline.ends", "timeline.reply", "timeline.react", "timeline.delete",
    "timeline.copy", "timeline.links"],
  timelineThreads: ["timeline.threads"],
  timelineSelection: ["timeline.copySelection", "timeline.escSelection"],
  timelineTail: ["global.members", "global.channelStep", "global.zoneComposer", "timeline.esc"],
  rail: ["rail.move", "rail.enter", "global.switcher", "global.cheatsheet", "global.reload", "global.tab"],
  channels: ["rail.move", "channels.open", "channels.threads", "channels.back", "global.channelStep"],
  channelsTimeline: ["channels.timeline"],
  channelsMembers: ["global.members"],
  channelsTail: ["global.switcher", "global.cheatsheet", "global.reload", "global.tab"],
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

function footer(state) {
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

function missingFooterIds() {
  var missing = []
  for (var state in FOOTER) {
    var ids = FOOTER[state]
    for (var i = 0; i < ids.length; i++) if (!entry(ids[i])) missing.push(state + ":" + ids[i])
  }
  return missing
}

function sections() {
  var out = []
  for (var z = 0; z < ZONES.length; z++) {
    var rows = []
    for (var i = 0; i < ENTRIES.length; i++) {
      var e = ENTRIES[i]
      if (e.zone !== ZONES[z].id || e.cheatsheet === false) continue
      var dup = false
      for (var r = 0; r < rows.length; r++) if (rows[r].keys === e.keys && rows[r].action === e.action) dup = true
      if (!dup) rows.push({ keys: e.keys, action: e.action })
    }
    out.push({ id: ZONES[z].id, title: ZONES[z].title, rows: rows })
  }
  return out
}
