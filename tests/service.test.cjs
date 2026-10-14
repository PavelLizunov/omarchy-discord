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
test('server entry only browses channels, regardless of remembered channel or loading', () => {
  for (const loading of [false, true]) {
    const calls = [];
    const ctx = load(['enterGuild','resolveGuildEntry'], {
      pendingGuildEntry:'',selectedGuildId:'2',currentChannelId:'old',
      visibleSurfaces:{'full-panel':true},lastChannels:[{guildId:'2',channelId:'general'}],
      channelsFor(){return loading ? [] : [{id:'general',type:'text'}];},
      isLoadingChannels(){return loading;},
      Api:vm.runInNewContext(fs.readFileSync(path.join(root,'Api.js'),'utf8') + '\n({browseGuild})'),
      showChannel(){calls.push('showChannel');},guildEntered(){},
      closeChannel(id){calls.push('close:'+id);},loadChannels(id){calls.push('load:'+id);}
    });
    ctx.root = ctx;
    ctx.enterGuild('2');
    assert.equal(ctx.currentChannelId,'');
    assert.equal(ctx.selectedGuildId,'2');
    assert(!calls.includes('showChannel'));
    ctx.resolveGuildEntry();
    assert.equal(ctx.currentChannelId,'');
    assert(!calls.includes('showChannel'));
  }
});
test('DM activity refresh reorders the list while preserving chat, drafts and selection', () => {
  let reply; let requests=0;
  const ctx=load(['noteChannelUpdate'],{dms:[{id:'old'},{id:'active'}],currentChannelId:'old',selectedGuildId:'dms',drafts:{old:'keep'},send(name,fields,callback){assert.equal(name,'list_dms');requests++;reply=callback;}});
  ctx.root=ctx;
  ctx.noteChannelUpdate({id:'active',type:'dm',guild_id:null,last_message_id:'600000000000000008'});
  reply(true,{channels:[{id:'active',unread:'unread'},{id:'old'}]});
  assert.equal(requests,1);assert.equal(ctx.dms[0].id,'active');assert.equal(ctx.currentChannelId,'old');assert.equal(ctx.drafts.old,'keep');
});
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
test('DM notification eligibility does not depend on an open conversation', () => {
  const ctx=load(['notifySkipReason'],{lifecycle:'ready',notificationMode:'Mentions and DMs',backendState:{presence:'online'},panelActive:false,visibleSurfaces:{},currentChannelId:'other',isOwn(){return false;}});
  const dm={notify:true,guild_id:null,channel_id:'closed',message:{author:{id:'ada'},mentions_self:false}};
  assert.equal(ctx.notifySkipReason(dm),'');
  ctx.notificationMode='Off';assert.equal(ctx.notifySkipReason(dm),'mode-off');ctx.notificationMode='Mentions and DMs';
  ctx.backendState.presence='dnd';assert.equal(ctx.notifySkipReason(dm),'dnd');ctx.backendState.presence='online';
  ctx.panelActive=true;ctx.visibleSurfaces={'full-panel':true};ctx.currentChannelId='closed';assert.equal(ctx.notifySkipReason(dm),'viewing');
  ctx.currentChannelId='other';assert.equal(ctx.notifySkipReason(dm),'');assert.equal(ctx.notifySkipReason({...dm,notify:false}),'not-notify');
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
test('archive opaque persistence preserves scoped entry and survives a config round trip', () => {
  let ctx, updates=0;
  const entry={id:'quickshell.discord',notifications:'Off',unrelated:{retain:true},archivedGuilds:'[]'};
  ctx=load(['configuredEntry','persistOpaque'],{pluginId:'quickshell.discord',Api:{shallowCopy:x=>({...x})},shell:{barConfig:{layout:{right:[entry]}},updateEntryInline(id,next){assert.equal(id,'quickshell.discord');updates++;this.barConfig=JSON.parse(JSON.stringify({layout:{right:[next]}}));return true;}}});
  ctx.persistOpaque('archivedGuilds','["2"]');
  assert.equal(updates,1);assert.equal(ctx.configuredEntry().archivedGuilds,'["2"]');
  assert.equal(ctx.configuredEntry().notifications,'Off');assert.equal(ctx.configuredEntry().unrelated.retain,true);
  assert.equal(entry.archivedGuilds,'[]');
});
test('daemon stop callback preserves active state on failure', () => {
  const daemon = fs.readFileSync(path.join(root, 'DaemonManager.qml'), 'utf8');
  const stop = daemon.slice(daemon.indexOf('    id: stopCommand'));
  const callback = stop.match(/onExited: function\(exitCode\) \{([\s\S]*?)\n    \}/)[1];
  let stopped = 0;
  const model = {busy: true, serviceActive: true, lastError: '', stopped(){stopped++;}};
  const ctx = vm.createContext({root: model});
  vm.runInContext('(function(exitCode) {' + callback + '})(7)', ctx);
  assert.equal(model.serviceActive, true);
  assert.equal(model.busy, false);
  assert.equal(stopped, 0);
  assert.equal(model.lastError, 'Could not stop the Discord backend');
  vm.runInContext('(function(exitCode) {' + callback + '})(0)', ctx);
  assert.equal(model.serviceActive, false);
  assert.equal(stopped, 1);
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
