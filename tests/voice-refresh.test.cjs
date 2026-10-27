const test=require('node:test');
const assert=require('node:assert/strict');
const fs=require('node:fs');
const vm=require('node:vm');
const src=fs.readFileSync(require('node:path').join(__dirname,'../Service.qml'),'utf8');
test('voice refresh consumes the current cached roster without joining or changing presence',()=>{
 const start=src.indexOf('  function refreshVoiceMembers('),end=src.indexOf('\n  }',start);
 let callback,calls=0,applied;
 const c={ready:true,voiceMemberRevisions:{},backendClient:{sendCommand(name,fields,cb){assert.equal(name,'voice_members');assert.equal(fields.guild_id,'1');callback=cb;calls++;}},applyVoiceMembers(x){applied=x;}};c.root=c;vm.createContext(c);vm.runInContext(src.slice(start,end+4),c);
 c.refreshVoiceMembers('1');assert.equal(calls,1);const roster={guild_id:'1',channels:[{channel_id:'2',users:[{id:'3'}]}]};callback(true,roster);assert.equal(applied,roster);
 c.ready=false;c.refreshVoiceMembers('1');assert.equal(calls,1);applied=null;callback(true,roster);assert.equal(applied,null);
 assert.match(src,/running: root.ready && \(root.uiVisible \|\| root.inCall\) && !root.restartBusy/);
});

function voiceContext() {
 const callbacks=[];
 const c={ready:true,voiceMembers:{},voiceMemberRevisions:{},Api:{shallowCopy:x=>({...x})},backendClient:{sendCommand(name,fields,callback){callbacks.push(callback);}}};
 c.root=c;vm.createContext(c);
 for(const name of ['refreshVoiceMembers','applyVoiceMembers']) {
  const start=src.indexOf('  function '+name+'('),end=src.indexOf('\n  }',start);
  vm.runInContext(src.slice(start,end+4),c);
 }
 return {c,callbacks};
}
test('late cached refresh cannot erase a newer live roster',()=>{
 const {c,callbacks}=voiceContext();
 c.refreshVoiceMembers('1');
 const live={guild_id:'1',channels:[{channel_id:'2',users:[{id:'3'},{id:'4'}]}]};
 c.applyVoiceMembers(live);
 callbacks[0](true,{guild_id:'1',channels:[]});
 assert.equal(c.voiceMembers['1'],live.channels);
 c.applyVoiceMembers({guild_id:'1',channels:[]});
 assert.equal(c.voiceMembers['1'].length,0,'a genuine leave must still clear the room');
});
test('refresh from a prior socket lifetime and malformed rosters are ignored',()=>{
 const {c,callbacks}=voiceContext();
 c.refreshVoiceMembers('1');c.voiceMemberRevisions={};
 callbacks[0](true,{guild_id:'1',channels:[]});assert.equal(c.voiceMembers['1'],undefined);
 c.applyVoiceMembers({guild_id:'1'});assert.equal(c.voiceMembers['1'],undefined);
 c.refreshVoiceMembers('1');callbacks[1](true,{guild_id:'2',channels:[]});assert.equal(c.voiceMembers['2'],undefined);
});
