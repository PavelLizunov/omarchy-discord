.pragma library

// Original 24-unit action drawings; no external assets or runtime requests.
var paths = {
  omacord: '<path fill-rule="evenodd" d="M4 2H12V3H14V5H15V12H13V14H10V12H6V14H3V12H1V5H2V3H4Z M4 6V9H7V6Z M9 6V9H12V6Z M4 10V11H12V10Z"/>',
  barMicrophone: '<path d="M6 1H10V2H11V8H10V9H6V8H5V2H6Z M3 6H4V10H6V11H10V10H12V6H13V11H11V12H9V14H12V15H4V14H7V12H5V11H3Z"/>',
  barMicrophoneOff: '<path d="M6 1H10V2H11V7H9V5H7V3H5V2H6Z M5 6H7V8H9V9H6V8H5Z M3 6H4V10H6V11H9V12H9V14H12V15H4V14H7V12H5V11H3Z M12 6H13V10H12Z M1 1H3V3H5V5H7V7H9V9H11V11H13V13H15V15H13V13H11V11H9V9H7V7H5V5H3V3H1Z"/>',
  barHeadphonesOff: '<path d="M4 2H12V3H14V5H15V14H11V9H13V5H11V4H5V5H3V9H5V14H1V5H2V3H4Z M1 1H3V3H5V5H7V7H9V9H11V11H13V13H15V15H13V13H11V11H9V9H7V7H5V5H3V3H1Z"/>',
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
  attach: '<path d="m21 11-9 9a6 6 0 0 1-8.5-8.5L13 2a4 4 0 0 1 5.7 5.7l-9.5 9.5a2 2 0 0 1-2.8-2.8L15 5.8"/>',
  send: '<path d="m3 3 18 9-18 9 3-9-3-9Zm3 9h15"/>',
  save: '<path d="M4 3h13l4 4v14H3V3h1Zm3 0v6h10V3M7 21v-8h10v8"/>',
  microphone: '<rect x="9" y="3" width="6" height="12" rx="3"/><path d="M5 11v1a7 7 0 0 0 14 0v-1m-7 8v3m-4 0h8"/>',
  microphoneOff: '<path d="m3 3 18 18M9 9v3a3 3 0 0 0 5 2M9 5a3 3 0 0 1 6 0v4M5 11v1a7 7 0 0 0 12 5m2-5v-1m-7 8v3m-4 0h8"/>',
  camera: '<rect x="3" y="5" width="12" height="14" rx="2"/><path d="m15 10 6-4v12l-6-4"/>',
  headphones: '<path d="M3 14v-2a9 9 0 0 1 18 0v2M3 13h4v8H3v-8Zm14 0h4v8h-4v-8Z"/>',
  headphonesOff: '<path d="m3 3 18 18M7 4.5A9 9 0 0 1 21 12v2M3 9v5M3 13h4v8H3v-8m14 4v4h4"/>',
  speaker: '<path d="M11 5 6 9H3v6h3l5 4V5m4 3a6 6 0 0 1 0 8m3-11a10 10 0 0 1 0 14"/>',
  speakerOff: '<path d="M11 5 6 9H3v6h3l5 4V5m5 4 5 6m0-6-5 6"/>',
  leave: '<path d="M3 13c5-5 13-5 18 0l-2 5-5-2v-3h-4v3l-5 2-2-5Z"/>'
}

function svg(name, color, pixelSize) {
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
  if(name === "omacord" || name === "barMicrophone" || name === "barMicrophoneOff" || name === "barHeadphonesOff") {
    var size = Math.max(1, Math.min(512, Math.ceil(Number(pixelSize) || 16)))
    return '<svg xmlns="http://www.w3.org/2000/svg" viewBox="0 0 16 16" width="'+size+'" height="'+size+'" '
      + strokeStr.replace('stroke=', 'fill=').replace('stroke-opacity=', 'fill-opacity=') + '>' + paths[name] + '</svg>'
  }
  return '<svg xmlns="http://www.w3.org/2000/svg" viewBox="0 0 24 24" width="24" height="24"'
    + ' fill="none" ' + strokeStr + ' stroke-width="1.5" stroke-linecap="round" stroke-linejoin="round">'
    + paths[name] + '</svg>'
}

function source(name, color, pixelSize) {
  var value = svg(name, color, pixelSize)
  return value ? "data:image/svg+xml;charset=utf-8," + encodeURIComponent(value) : ""
}
