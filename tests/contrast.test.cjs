const test = require('node:test');
const assert = require('node:assert/strict');
const fs = require('node:fs');
const path = require('node:path');
const vm = require('node:vm');
const root = path.resolve(__dirname, '..');
const color = hex => ({r: parseInt(hex.slice(1, 3), 16)/255,
  g: parseInt(hex.slice(3, 5), 16)/255, b: parseInt(hex.slice(5, 7), 16)/255, a: 1});
const api = {Qt: {rgba: (r, g, b, a) => ({r, g, b, a})}};
vm.createContext(api);
vm.runInContext(fs.readFileSync(path.join(root, 'Api.js'), 'utf8'), api);

test('default normal-text colors meet AA', () => {
  const source = fs.readFileSync(path.join(root, 'ui/Color.qml'), 'utf8');
  const role = name => color(source.match(new RegExp('property color '+name+': "(#[0-9a-f]+)"'))[1]);
  for (const name of ['foreground', 'muted', 'urgent'])
    assert(api.contrastRatio(role(name), role('background')) >= 4.5, name);
});

test('secondary text fallback checks composited contrast', () => {
  for (const [foreground, background] of [['#cacccc', '#101315'], ['#242424', '#f5f4f0']]) {
    const bg = color(background);
    const result = api.secondaryColor(color('#707880'), color(foreground), bg);
    assert(api.contrastRatio(api.blend(result, bg, result.a), bg) >= 4.5);
  }
});

test('theme muted role is preserved when already readable', () => {
  const muted = color('#565656');
  assert.equal(api.secondaryColor(muted, color('#242424'), color('#f5f4f0')), muted);
});

test('semantic text resolves against painted hover, selection and badge surfaces', () => {
  for (const [background, foreground, accent, muted] of [
    ['#101315','#cacccc','#cacccc','#78818a'],
    ['#f5f4f0','#242424','#235a81','#565656'],
    ['#111c18','#c1c497','#509475','#53685b']]) {
    const bg=color(background), fg=color(foreground), ac=color(accent), mu=color(muted);
    for (const alpha of [0, .08, .18]) {
      const surface=api.blend(fg,bg,alpha);
      for (const painted of [surface,api.blend(ac,surface,.2),api.blend(fg,surface,.08)]) {
        const text=api.textColor(ac,fg,painted);
        assert(api.contrastRatio(api.blend(text,painted,text.a),painted)>=4.5);
        const secondary=api.secondaryColor(mu,fg,painted);
        assert(api.contrastRatio(api.blend(secondary,painted,secondary.a),painted)>=4.5);
      }
    }
  }
});

test('authorColor contrast meets AA across dark and light themes', () => {
  const testIds = ['100', '200', '300', 'alice', 'bob', 'carol', 'user123', ''];
  const darkBg = color('#101315');
  const lightBg = color('#f5f4f0');
  for (const id of testIds) {
    const cDarkHex = api.authorColor(id, darkBg, '#cacccc');
    const cLightHex = api.authorColor(id, lightBg, '#242424');
    const cDark = color(cDarkHex);
    const cLight = color(cLightHex);
    assert(api.contrastRatio(cDark, darkBg) >= 4.5, `authorColor dark for id ${id}: ${cDarkHex}`);
    assert(api.contrastRatio(cLight, lightBg) >= 4.5, `authorColor light for id ${id}: ${cLightHex}`);
  }
});
