const test=require('node:test');
const assert=require('node:assert/strict');
const fs=require('node:fs');
const vm=require('node:vm');
const src=fs.readFileSync(require('node:path').join(__dirname,'../Service.qml'),'utf8');
test('voice refresh consumes the current cached roster without joining or changing presence',()=>{
 const start=src.indexOf('  function refreshVoiceMembers('),end=src.indexOf('\n  }',start);
 let callback,calls=0,applied;
 const c={ready:true,backendClient:{sendCommand(name,fields,cb){assert.equal(name,'voice_members');assert.equal(fields.guild_id,'1');callback=cb;calls++;}},applyVoiceMembers(x){applied=x;}};c.root=c;vm.createContext(c);vm.runInContext(src.slice(start,end+4),c);
 c.refreshVoiceMembers('1');assert.equal(calls,1);const roster={guild_id:'1',channels:[{channel_id:'2',users:[{id:'3'}]}]};callback(true,roster);assert.equal(applied,roster);
 c.ready=false;c.refreshVoiceMembers('1');assert.equal(calls,1);applied=null;callback(true,roster);assert.equal(applied,null);
 assert.match(src,/running: root.ready && \(root.uiVisible \|\| root.inCall\) && !root.restartBusy/);
});
