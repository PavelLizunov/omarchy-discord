.pragma library

// Original 24-unit action drawings; no external assets or runtime requests.
var paths = {
  settings: '<circle cx="12" cy="12" r="3"/><path d="M19.4 15a1.65 1.65 0 0 0 .33 1.82l.06.06a2 2 0 0 1 0 2.83 2 2 0 0 1-2.83 0l-.06-.06a1.65 1.65 0 0 0-1.82-.33 1.65 1.65 0 0 0-1 1.51V21a2 2 0 0 1-2 2 2 2 0 0 1-2-2v-.09A1.65 1.65 0 0 0 9 19.4a1.65 1.65 0 0 0-1.82.33l-.06.06a2 2 0 0 1-2.83 0 2 2 0 0 1 0-2.83l.06-.06a1.65 1.65 0 0 0 .33-1.82 1.65 1.65 0 0 0-1.51-1H3a2 2 0 0 1-2-2 2 2 0 0 1 2-2h.09A1.65 1.65 0 0 0 4.6 9a1.65 1.65 0 0 0-.33-1.82l-.06-.06a2 2 0 0 1 0-2.83 2 2 0 0 1 2.83 0l.06.06a1.65 1.65 0 0 0 1.82.33H9a1.65 1.65 0 0 0 1-1.51V3a2 2 0 0 1 2-2 2 2 0 0 1 2 2v.09a1.65 1.65 0 0 0 1 1.51 1.65 1.65 0 0 0 1.82-.33l.06-.06a2 2 0 0 1 2.83 0 2 2 0 0 1 0 2.83l-.06.06a1.65 1.65 0 0 0-.33 1.82V9a1.65 1.65 0 0 0 1.51 1H21a2 2 0 0 1 2 2 2 2 0 0 1-2 2h-.09a1.65 1.65 0 0 0-1.51 1Z"/>',
  back: '<path d="m15 18-6-6 6-6"/>',
  all: '<rect x="3" y="3" width="7" height="7" rx="1"/><rect x="14" y="3" width="7" height="7" rx="1"/><rect x="14" y="14" width="7" height="7" rx="1"/><rect x="3" y="14" width="7" height="7" rx="1"/>',
  mention: '<circle cx="12" cy="12" r="4"/><path d="M16 8v5a3 3 0 0 0 6 0v-1a10 10 0 1 0-4 8"/>',
  unread: '<path d="M6 8a6 6 0 0 1 12 0c0 7 3 9 3 9H3s3-2 3-9m7.7 13a2 2 0 0 1-3.4 0"/>',
  textChannel: '<path d="M4 9h16M4 15h16M10 3 8 21m8-18-2 18"/>',
  sliders: '<path d="M3 6h18M3 12h18M3 18h18"/><circle cx="8" cy="6" r="2"/><circle cx="16" cy="12" r="2"/><circle cx="10" cy="18" r="2"/>',
  user: '<circle cx="12" cy="7" r="4"/><path d="M6 21v-2a6 6 0 0 1 12 0v2"/>',
  archive: '<rect x="3" y="4" width="18" height="4" rx="1"/><path d="M5 8v10a2 2 0 0 0 2 2h10a2 2 0 0 0 2-2V8m-9 4h4"/>',
  reconnect: '<path d="M20 9a8 8 0 1 0 0 6M20 3v6h-6"/>',
  navigation: '<path d="M3 5h18M3 12h18M3 19h18"/>',
  compact: '<rect x="4" y="5" width="16" height="14" rx="1"/><path d="M14 5v14"/>',
  expand: '<path d="M3 9V3h6m6 0h6v6M3 15v6h6m6 0h6v-6"/>',
  search: '<circle cx="10.5" cy="10.5" r="6.5"/><path d="m16 16 5 5"/>',
  help: '<circle cx="12" cy="12" r="9"/><path d="M9.5 9a2.5 2.5 0 0 1 5 0c0 2-2.5 2-2.5 4m0 3h.01"/>',
  members: '<circle cx="9" cy="8" r="3"/><path d="M3 20v-2a6 6 0 0 1 12 0v2m1-15a3 3 0 0 1 0 6m2 3a5 5 0 0 1 3 4v2"/>',
  logout: '<path d="M10 4H4v16h6m-1-8h12m-4-4 4 4-4 4"/>',
  close: '<path d="m6 6 12 12M18 6 6 18"/>',
  reply: '<path d="m10 5-7 7 7 7m-7-7h10a8 8 0 0 1 8 8"/>',
  react: '<circle cx="12" cy="12" r="9"/><path d="M8 14a4.5 4.5 0 0 0 8 0M8 9h.01M16 9h.01"/>',
  copy: '<rect x="8" y="8" width="12" height="13" rx="2"/><path d="M15 5V3H3v13h2"/>',
  edit: '<path d="m15 4 5 5m-16 11 2-6L17 3l4 4-11 11-6 2Z"/>',
  delete: '<path d="M3 6h18M9 6V3h6v3M5 6l1 15h12l1-15M10 10v7m4-7v7"/>',
  send: '<path d="m3 3 18 9-18 9 3-9-3-9Zm3 9h15"/>',
  save: '<path d="M4 3h13l4 4v14H3V3h1Zm3 0v6h10V3M7 21v-8h10v8"/>',
  microphone: '<rect x="9" y="3" width="6" height="12" rx="3"/><path d="M5 11v1a7 7 0 0 0 14 0v-1m-7 8v3m-4 0h8"/>',
  headphones: '<path d="M3 14v-2a9 9 0 0 1 18 0v2M3 13h4v8H3v-8Zm14 0h4v8h-4v-8Z"/>',
  leave: '<path d="M3 13c5-5 13-5 18 0l-2 5-5-2v-3h-4v3l-5 2-2-5Z"/>'
}

function svg(name, color) {
  if (!Object.prototype.hasOwnProperty.call(paths, name)) return ""
  if (!color) color = "#ffffff"
  var strokeStr = ""
  if (typeof color === "object" && typeof color.r === "number") {
    var red = Math.round(color.r * 255)
    var green = Math.round(color.g * 255)
    var blue = Math.round(color.b * 255)
    var opacity = typeof color.a === "number" ? color.a : 1
    strokeStr = 'stroke="rgb(' + red + ',' + green + ',' + blue + ')" stroke-opacity="' + opacity + '"'
  } else {
    strokeStr = 'stroke="' + String(color) + '"'
  }
  return '<svg xmlns="http://www.w3.org/2000/svg" viewBox="0 0 24 24" width="24" height="24"'
    + ' fill="none" ' + strokeStr + ' stroke-width="1.5" stroke-linecap="round" stroke-linejoin="round">'
    + paths[name] + '</svg>'
}

function source(name, color) {
  var value = svg(name, color)
  return value ? "data:image/svg+xml;charset=utf-8," + encodeURIComponent(value) : ""
}
