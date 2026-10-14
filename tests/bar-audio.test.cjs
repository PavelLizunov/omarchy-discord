const fs=require('node:fs');
const vm=require('node:vm');
const test=require('node:test');
const assert=require('node:assert/strict');
const source=fs.readFileSync(require('node:path').join(__dirname,'../components/BarAudioControl.qml'),'utf8');
function fixture(muted=false){
 const pending=[],calls=[],errors=[];
 const c={busy:false,pendingFields:null,replyReceived:false,savedMuted:false,hasSavedMute:false,revision:0,previousDeafened:false,available:true,room:'room',voice:{muted,deafened:false},timeout:{start(){},stop(){}},service:{backend:{sendCommand(name,fields,cb){assert.equal(name,'voice_set');calls.push(JSON.parse(JSON.stringify(fields)));pending.push(cb);}},fail(e){errors.push(e);}}};c.root=c;
 vm.createContext(c);
 for(const name of ['reset','sync','request','toggleMicrophone','toggleSilence']){
  const start=source.indexOf('  function '+name+'('),end=source.indexOf('\n  }',start);
  assert(start>=0);vm.runInContext(source.slice(start,end+4),c);
 }
 c.timeoutStop=()=>{};
 return {c,calls,errors,finish(ok=true,eventFirst=true){const fields=calls[calls.length-1],cb=pending.shift();if(ok&&eventFirst){c.voice={...c.voice,...fields};c.sync();}cb(ok,{},ok?'':'denied');if(ok&&!eventFirst){assert(c.busy,'reply alone must not unlock a stale-state action');c.voice={...c.voice,...fields};c.sync();}}};
}
test('combined silence restores prior mute with both backend reply/event orders',()=>{
 for(const muted of [false,true])for(const first of [false,true]){
  const f=fixture(muted),c=f.c;assert(c.toggleSilence());assert(!c.toggleSilence());assert.deepEqual(f.calls[0],{muted:true,deafened:true});f.finish(true,first);assert(c.toggleSilence());assert.deepEqual(f.calls[1],{muted,deafened:false});f.finish(true,first);assert.equal(c.voice.muted,muted);assert.equal(c.voice.deafened,false);
 }
});
test('left click exits combined silence into normal mode regardless of prior mute',()=>{
 for(const muted of [false,true])for(const first of [false,true]){
  const f=fixture(muted),c=f.c;c.toggleSilence();f.finish(true,first);assert(c.toggleMicrophone());assert.deepEqual(f.calls[1],{muted:false,deafened:false});assert(!c.toggleSilence());f.finish(true,first);assert(!c.voice.muted);assert(!c.voice.deafened);assert(!c.hasSavedMute);
 }
 const f=fixture(true);f.c.voice={muted:true,deafened:true};f.c.sync();assert(f.c.toggleMicrophone());assert.deepEqual(f.calls[0],{muted:false,deafened:false});f.finish();
});
test('microphone-only, idle gate and external deafen do not turn microphone on unexpectedly',()=>{
 const f=fixture(true),c=f.c;assert(c.toggleMicrophone());assert.deepEqual(f.calls[0],{muted:false});f.finish();c.available=false;assert(!c.toggleSilence());assert(!c.toggleMicrophone());c.available=true;c.voice={muted:true,deafened:true};c.sync();assert(c.toggleSilence());assert.deepEqual(f.calls[1],{muted:true,deafened:false});f.finish();
});
test('room loss invalidates late callbacks and failed silence cannot invent restoration state',()=>{
 const f=fixture(),c=f.c;c.toggleSilence();c.reset();assert(!c.hasSavedMute);f.finish(false);assert.deepEqual(f.errors,[]);
 c.toggleSilence();f.finish(false);assert(!c.busy);assert(!c.hasSavedMute);assert.deepEqual(f.errors,['denied']);
 c.toggleSilence();f.finish();c.voice={muted:true,deafened:false};c.sync();assert(!c.hasSavedMute);
});
