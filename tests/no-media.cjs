const fs = require('node:fs');
const vm = require('node:vm');
const assert = require('node:assert/strict');
const path = require('node:path');
const root = path.resolve(__dirname, '..');
const source = fs.readFileSync(path.join(root, 'Service.qml'), 'utf8');
function body(name) {
  const start = source.indexOf('function ' + name + '(');
  assert(start >= 0, name);
  const brace = source.indexOf('{', start);
  let depth = 1, i = brace + 1;
  while (depth && i < source.length) { if (source[i] === '{') depth++; else if (source[i] === '}') depth--; i++; }
  return source.slice(start, i);
}
let calls = 0;
const ctx = {textOnly:true,connected:true,mediaAllowed:false,mediaPaths:{'cached|64':'/old.png'},
  mediaWanted:{},mediaPending:{},backendClient:{sendCommand(){calls++;}},Qt:{callLater(){calls++;}}};
vm.createContext(ctx);
for (const name of ['mediaPath','requestMedia','flushMediaRequests','fetchMedia','applyMediaReady']) vm.runInContext(body(name),ctx);
assert.equal(ctx.mediaPath('cached',64),'');
ctx.requestMedia('https://cdn.discordapp.com/a.png',0);
ctx.flushMediaRequests();
ctx.fetchMedia('key','https://cdn.discordapp.com/a.png',0);
ctx.applyMediaReady({url:'https://cdn.discordapp.com/a.png',path:'/old.png',ok:true});
assert.equal(calls,0);
assert.equal(Object.keys(ctx.mediaWanted).length,0);
for (const file of ['ClientView.qml',...fs.readdirSync(path.join(root,'components')).filter(f=>f.endsWith('.qml')).map(f=>'components/'+f)]) {
  const text=fs.readFileSync(path.join(root,file),'utf8');
  assert(!text.includes('MultiEffect {'),file);
  if (file === 'components/DroppedFileStager.qml') {
    assert(!text.includes('fetch_media') && !text.includes('mediaPath('), 'File staging must not fetch remote media');
    assert(text.includes('stage-dropped-files.py') && text.includes('root.service.stageFile('), 'Local copies must use the staging-only helper');
    assert(!text.includes('.upload(') && !text.includes('.sendMessage('), 'Dropping must never send');
  } else if (file === 'components/ExternalLink.qml') {
    assert(text.includes('"/usr/bin/xdg-open"') && text.includes('onExited:'), 'External link handoff must report process failure');
    assert(!text.includes('fetch_media') && !text.includes('.upload('), 'Link handoff must not fetch or upload attachments');
  } else assert(!/^import Quickshell/m.test(text),file);
  if (file !== 'ClientView.qml' && file !== 'components/CameraView.qml' && file !== 'components/CallBar.qml') assert(!/\bImage\s*\{/.test(text),file);
  if (file === 'components/CallBar.qml') {
    assert.equal((text.match(/\bImage\s*\{/g) || []).length, 1, 'Only the local voice-room SVG is allowed');
    assert(text.includes('source: Icons.source("headphones",'), 'Voice-room image must be a local SVG, not downloaded media');
    assert(!text.includes('mediaPath(') && !text.includes('fetch_media'), 'Voice-room icon must not fetch attachment media');
  }
  if (file === 'components/CameraView.qml') {
    assert(!text.includes('fetch_media'), 'Camera frames must not enable attachment fetching');
    assert(text.includes('"data:image/jpeg;base64,"'), 'Camera display must use backend frame bytes');
    assert(text.includes('video_watch') && text.includes('Component.onDestruction'), 'Camera reception must have an explicit stop path');
  }
}
const inlineVideo = fs.readFileSync(path.join(root,'components/InlineVideo.qml'),'utf8');
assert(inlineVideo.includes('MediaPlayer {') && inlineVideo.includes('Component.onDestruction: player.stop()'));
const row = fs.readFileSync(path.join(root,'components/MessageRow.qml'),'utf8');
assert(row.includes('active: attachment.playing && root.mediaActive && attachment.safeUrl'));
assert(row.includes('else attachment.playing = true'));
console.log('PASS: no automatic attachment image fetching; video player is explicit, lazy and lifecycle-owned');
const api = {};
vm.createContext(api);
vm.runInContext(fs.readFileSync(path.join(root,'Api.js'),'utf8'),api);
assert.equal(api.userLabel({id:'123',display_name:'Ada'},{}),'Ada');
assert.equal(api.userLabel({id:'123'},{'123':'Cached Ada'}),'Cached Ada');
assert.equal(api.userLabel({id:'123'},{}),'User #123');
assert.equal(api.userLabel({},{}),'Unknown user');
console.log('PASS: missing voice/member names retain a distinct text identity');
