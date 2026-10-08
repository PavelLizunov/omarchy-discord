var ZONES = [
  { id: "global", title: "General shortcuts" },
  { id: "timeline", title: "Messages and timeline" },
  { id: "composer", title: "Message composer" },
  { id: "voice", title: "Voice call" },
  { id: "sidebar", title: "Servers and channels" }
]

var ENTRIES = [
  { id: "global.switcher", zone: "global", keys: "Ctrl+K · /", hintKeys: "Ctrl+K", hint: "find channel",
    action: "Quick switcher: find any channel or direct message" },
  { id: "global.cheatsheet", zone: "global", keys: "Ctrl+/ · ?", hintKeys: "?", hint: "shortcuts",
    action: "This keyboard shortcuts reference" },
  { id: "global.members", zone: "global", keys: "m", hintKeys: "m", hint: "members",
    action: "Show or hide the member list" },
  { id: "global.tab", zone: "global", keys: "Tab · Shift+Tab", hintKeys: "Tab", hint: "cycle focus",
    action: "Cycle focus between panels and toolbar controls" },
  { id: "global.esc", zone: "global", keys: "Esc", hint: "back",
    action: "Close dialogs or move focus back: composer → timeline → channels → servers" },

  { id: "timeline.move", zone: "timeline", keys: "↑ / ↓ · j / k", hintKeys: "↑/↓", hint: "move",
    action: "Navigate between messages in the timeline" },
  { id: "timeline.reply", zone: "timeline", keys: "R / r", hint: "reply",
    action: "Reply to the selected message" },
  { id: "timeline.react", zone: "timeline", keys: "E / e", hint: "react",
    action: "Add emoji reaction to the selected message" },
  { id: "timeline.copy", zone: "timeline", keys: "Y · Ctrl+C", hint: "copy",
    action: "Copy message text to the clipboard" },
  { id: "timeline.delete", zone: "timeline", keys: "D D", hint: "delete",
    action: "Delete your message (press D twice within 3 seconds)" },
  { id: "timeline.enter", zone: "timeline", keys: "Enter", hint: "open link",
    action: "Open message link or reveal spoiler image" },

  { id: "composer.send", zone: "composer", keys: "Enter", hint: "send",
    action: "Send message (or save edit)" },
  { id: "composer.newline", zone: "composer", keys: "Shift+Enter", hint: "newline",
    action: "Insert a newline in composer" },
  { id: "composer.editLast", zone: "composer", keys: "↑", hint: "edit last",
    action: "In an empty composer: edit your most recent message" },
  { id: "composer.paste", zone: "composer", keys: "Ctrl+V", hint: "paste",
    action: "Paste text or attach image from clipboard" },
  { id: "composer.cancel", zone: "composer", keys: "Esc", hint: "cancel",
    action: "Cancel edit or reply mode" },

  { id: "voice.mute", zone: "voice", keys: "Ctrl+Shift+M", hint: "mute",
    action: "Mute or unmute your microphone" },
  { id: "voice.deafen", zone: "voice", keys: "Ctrl+Shift+D", hint: "deafen",
    action: "Deafen or undeafen audio" },
  { id: "voice.leave", zone: "voice", keys: "Ctrl+Shift+H", hint: "leave",
    action: "Disconnect from voice call" },

  { id: "sidebar.move", zone: "sidebar", keys: "↑ / ↓ · j / k", hintKeys: "↑/↓", hint: "move",
    action: "Move cursor between servers or channels" },
  { id: "sidebar.open", zone: "sidebar", keys: "Enter · →", hint: "open",
    action: "Open selected channel (or view voice channel text chat)" },
  { id: "sidebar.back", zone: "sidebar", keys: "← · h · Esc", hint: "back",
    action: "Back from channel list to servers rail" },
  { id: "sidebar.menu", zone: "sidebar", keys: "Shift+F10 · Menu", hint: "menu",
    action: "Server context menu: settings, mark read, archive, leave" },

  { id: "members.move", zone: "members", cheatsheet: false, keys: "↑ / ↓ · j / k", hintKeys: "↑/↓", hint: "move",
    action: "Navigate member list" },
  { id: "members.copy", zone: "members", cheatsheet: false, keys: "Y", hint: "copy",
    action: "Copy member @username" },

  { id: "switcher.type", zone: "switcher", cheatsheet: false, keys: "type", hint: "search",
    action: "Search channels and DMs" },
  { id: "switcher.move", zone: "switcher", cheatsheet: false, keys: "↑ / ↓", hintKeys: "↑/↓", hint: "move",
    action: "Select channel" },
  { id: "switcher.open", zone: "switcher", cheatsheet: false, keys: "Enter", hint: "open",
    action: "Open selected channel" },
  { id: "switcher.esc", zone: "switcher", cheatsheet: false, keys: "Esc", hint: "close",
    action: "Close switcher" },

  { id: "picker.type", zone: "picker", cheatsheet: false, keys: "type", hint: "filter",
    action: "Filter emojis" },
  { id: "picker.move", zone: "picker", cheatsheet: false, keys: "arrows", hintKeys: "arrows", hint: "move",
    action: "Navigate emoji picker" },
  { id: "picker.pick", zone: "picker", cheatsheet: false, keys: "Enter", hint: "select",
    action: "Select emoji" },
  { id: "picker.esc", zone: "picker", cheatsheet: false, keys: "Esc", hint: "close",
    action: "Close emoji picker" },

  { id: "login.activate", zone: "login", cheatsheet: false, keys: "Enter", hint: "activate",
    action: "Submit login / activate" },
  { id: "login.tab", zone: "login", cheatsheet: false, keys: "Tab", hint: "cycle",
    action: "Cycle login fields" },
  { id: "login.esc", zone: "login", cheatsheet: false, keys: "Esc", hint: "close",
    action: "Close login window" }
]

var FOOTER = {
  login: ["login.activate", "login.tab", "login.esc"],
  qrRunning: ["login.esc"],
  qrDone: ["login.activate", "login.esc"],
  qrMissing: ["login.esc"],
  down: ["global.tab", "global.esc"],
  buttons: ["global.tab", "global.esc"],
  composer: ["composer.send", "composer.newline", "composer.editLast", "composer.paste"],
  composerTail: ["composer.cancel"],
  composerMembers: ["global.members"],
  members: ["members.move", "members.copy", "global.members", "global.esc"],
  voice: ["voice.mute", "voice.deafen", "voice.leave"],
  composerChips: ["composer.send"],
  composerEdit: ["composer.send", "composer.newline", "composer.cancel"],
  chips: ["composer.send", "composer.cancel"],
  timeline: ["timeline.move", "timeline.reply", "timeline.react", "timeline.delete", "timeline.copy", "timeline.enter"],
  timelineThreads: ["timeline.reply"],
  timelineSelection: ["timeline.copy"],
  timelineTail: ["global.members", "global.esc"],
  rail: ["sidebar.move", "sidebar.open", "global.switcher", "global.cheatsheet"],
  channels: ["sidebar.move", "sidebar.open", "sidebar.back"],
  channelsTimeline: ["sidebar.open"],
  channelsMembers: ["global.members"],
  channelsTail: ["global.switcher", "global.cheatsheet"],
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
    if (!e) continue
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
      rows.push({ keys: e.keys, action: e.action })
    }
    if (rows.length) out.push({ id: ZONES[z].id, title: ZONES[z].title, rows: rows })
  }
  return out
}

if (typeof module !== "undefined" && module.exports) {
  module.exports = { sections: sections, footer: footer, hints: hints, missingFooterIds: missingFooterIds }
}
