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
