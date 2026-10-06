const fs = require('node:fs');
const path = require('node:path');
const vm = require('node:vm');
const test = require('node:test');
const assert = require('node:assert/strict');
const root = path.resolve(__dirname, '..');
const source = fs.readFileSync(path.join(root, 'Service.qml'), 'utf8');
function load(names, context) {
  vm.createContext(context);
  for (const name of names) {
    const start = source.indexOf('  function ' + name + '(');
    assert(start >= 0, name);
    const end = source.indexOf('\n  }', start);
    assert(end > start, name);
    vm.runInContext(source.slice(start, end + 4), context);
  }
  return context;
}
test('upload completion preserves files staged while it was in flight', () => {
  let finish;
  const ctx = load(['stagedFor','setStaged','patchStaged','upload'], {
    staged:{channel:[{path:'/inert/first.png',filename:'first.png'}]},
    ready:true,uploadChannels:{},Api:{shallowCopy:value => Object.assign({},value),assign:Object.assign},
    isOpen(){return true;},noteActivity(){},draftFor(){return '';},
    send(name, fields, callback){finish = callback; return 7;},
    Quickshell:{execDetached(){}},fail(message){throw new Error(message);}
  });
  ctx.root = ctx;
  assert.equal(ctx.upload('channel',''),true);
  ctx.setStaged('channel',ctx.stagedFor('channel').concat([{path:'/inert/second.png',filename:'second.png'}]));
  finish(true);
  assert.deepEqual(Array.from(ctx.stagedFor('channel'),file => file.path),['/inert/second.png']);
});
test('scoped bar settings use the signal of their owning property', () => {
  assert.match(source, /function onBarConfigChanged\(\) \{ root\.syncSettings\(\) \}/);
  assert.doesNotMatch(source, /function onShellConfigChanged/);
  const ctx = load(['configuredEntry', 'syncSettings'], {
    shell:{barConfig:{layout:{left:[{id:'quickshell.discord',window:'On demand'}]}}},
    pluginId:'quickshell.discord', frequentEmojiKey:'frequentEmoji',lastChannelKey:'lastChannels',
    frequentEmoji:[],lastChannels:[],Emoji:{parseFrequent(){return [];}},Api:{parseLastChannels(){return [];}},
    applySettings(entry){ctx.observed = entry.window;}
  });
  ctx.syncSettings();
  assert.equal(ctx.observed, 'On demand');
  ctx.shell.barConfig = {layout:{right:[{id:'quickshell.discord',window:'Persistent'}]}};
  ctx.syncSettings();
  assert.equal(ctx.observed, 'Persistent');
});
test('disabled plugin teardown distinguishes a hot reload', () => {
  let stops = 0;
  const ctx = load(['stopBackendIfDisabled'], {
    pluginRegistry:{isEnabled(){return false;}},shell:{pluginReloading:true},
    pluginId:'quickshell.discord',stopBackend(){stops++;}
  });
  ctx.stopBackendIfDisabled();
  assert.equal(stops,0);
  ctx.shell.pluginReloading = false;
  ctx.stopBackendIfDisabled();
  assert.equal(stops,1);
  ctx.pluginRegistry.isEnabled = () => true;
  ctx.stopBackendIfDisabled();
  assert.equal(stops,1);
});
