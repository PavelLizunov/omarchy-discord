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
  assert(!/^import Quickshell/m.test(text),file);
  if (file !== 'ClientView.qml') assert(!/\bImage\s*\{/.test(text),file);
}
console.log('PASS: no fetch_media, no queued media, no cached images, portable text consumers');
const api = {};
vm.createContext(api);
vm.runInContext(fs.readFileSync(path.join(root,'Api.js'),'utf8'),api);
assert.equal(api.userLabel({id:'123',display_name:'Ada'},{}),'Ada');
assert.equal(api.userLabel({id:'123'},{'123':'Cached Ada'}),'Cached Ada');
assert.equal(api.userLabel({id:'123'},{}),'User #123');
assert.equal(api.userLabel({},{}),'Unknown user');
console.log('PASS: missing voice/member names retain a distinct text identity');
