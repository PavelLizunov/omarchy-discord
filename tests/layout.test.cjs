const test = require('node:test');
const assert = require('node:assert/strict');
const fs = require('node:fs');
const vm = require('node:vm');

test('Full and Compact explicitly float and resize, never restore a cramped tile', () => {
  const source = fs.readFileSync(new URL('../Panel.qml', `file://${__filename}`), 'utf8');
  const start = source.indexOf('function applyLayoutMode(compact) {');
  const end = source.indexOf('\n  readonly property var toplevel:', start);
  const commands = [];
  const context = {Hyprland:{dispatch:command => commands.push(command)}, Style:{space:value => value}, Math};
  vm.createContext(context);
  vm.runInContext(source.slice(start, end), context);
  context.applyLayoutMode(true);
  context.applyLayoutMode(false);
  assert.equal(commands.length, 4);
  assert(commands[0].includes('action = "on"'));
  assert(commands[1].includes('x = 520, y = 560, relative = false'));
  assert(commands[2].includes('action = "on"'));
  assert(commands[3].includes('x = 1040, y = 680, relative = false'));
  assert(commands.every(command => command.includes('title:^(Omarchy Discord)$')));
});
