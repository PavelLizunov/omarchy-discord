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

test('bar exposes one smaller SVG microphone without shrinking its click target', () => {
 assert(!source.includes('id: callMark'));assert(!source.includes('callGlyph'));
 const mute=source.slice(source.indexOf('id: quickMute'),source.indexOf('onSettingsChanged:'));
 assert(mute.includes('width: Math.round(Style.bar.iconCanvas * 0.75)'));assert(mute.includes('height: width'));assert(mute.includes('fixedWidth: root.vertical ? root.barSize : Style.bar.statusSlot'));assert(mute.includes('fixedHeight: root.vertical ? Style.bar.statusSlot : -1'));assert(mute.includes('Icons.source('));assert(mute.includes('labelVisible: false'));
});

test('bar mark uses compact padding without overlapping the microphone target', () => {
 assert(source.includes('+ Style.space(5))'));assert(!source.includes('+ Style.space(17))'));
 assert(source.includes('x: root.vertical ? 0 : button.width'));
 assert(source.includes('root.vertical || root.badgeText ? 0 : -Style.space(5)'));
 assert(source.includes('root.vertical || root.badgeText ? 0 : Style.space(2)'));
});

test('bar microphone remains discoverable but idle clicks cannot change the call', () => {
 const mute=source.slice(source.indexOf('id: quickMute'),source.indexOf('onSettingsChanged:'));
 assert(mute.includes('visible: true'));assert(mute.includes('enabled: !!(root.discord && root.discord.ready && root.inCall)'));
 assert(mute.includes('Keys.onReturnPressed')&&mute.includes('Keys.onSpacePressed'));
 assert(mute.includes('enabled && root.discord'));
 assert(mute.includes('barMicrophoneOff')&&mute.includes('barMicrophone'));
});

test('compact popup uses native keyboard ownership and the real search focus target', () => {
  const popup=fs.readFileSync(new URL('../QuickPopup.qml', `file://${__filename}`),'utf8');
  const view=fs.readFileSync(new URL('../components/QuickView.qml', `file://${__filename}`),'utf8');
  assert(popup.includes('KeyboardPanel {'));
  assert(popup.includes('focusTarget: view.focusItem'));
  assert(!popup.includes('grabFocus:'));
  assert(view.includes('readonly property var focusItem: callView ? call.roster : search'));
  assert(view.includes('roster.singleClickAudio: true'));
  assert(view.includes('showDiagnostics: false'));
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
