const test = require('node:test');
const assert = require('node:assert/strict');
const fs = require('node:fs');
const vm = require('node:vm');
const source = fs.readFileSync(new URL('../BarWidget.qml', `file://${__filename}`), 'utf8');

test('bar left click opens only compact popup; right/middle keep explicit window actions', () => {
  const handler = source.slice(source.lastIndexOf('onPressed: function(mouseButton) {'));
  const body = handler.slice(handler.indexOf('{') + 1, handler.indexOf('\n    }'));
  const calls=[];
  const context={mouseButton:1,Qt:{LeftButton:1,RightButton:2,MiddleButton:4},root:{toggle:()=>calls.push('compact'),middleClick:()=>calls.push('middle'),openFullPanel:()=>calls.push('full')}};
  vm.createContext(context);
  vm.runInContext(body,context);
  context.mouseButton=2;vm.runInContext(body,context);
  context.mouseButton=4;vm.runInContext(body,context);
  assert.deepEqual(calls,['compact','full','middle']);
});

test('persistent close does not depend on focus, and quick mute does not deafen or leave', () => {
  const panel=fs.readFileSync(new URL('../Panel.qml', `file://${__filename}`),'utf8');
  const close=panel.slice(panel.indexOf('function close() {'),panel.indexOf('Component.onCompleted:'));
  assert(close.includes('if (persistent)'));
  assert(!close.includes('persistent && opened'));
  const mute=source.slice(source.indexOf('id: quickMute'),source.indexOf('onSettingsChanged:'));
  assert(mute.includes('toggleMute()'));
  assert(!mute.includes('toggleDeafen')&&!mute.includes('voiceLeave'));
});
