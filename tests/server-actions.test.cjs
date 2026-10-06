const fs=require('node:fs'),vm=require('node:vm'),test=require('node:test'),assert=require('node:assert/strict');
const source=fs.readFileSync('components/ServerActions.qml','utf8');
function load(names,ctx){vm.createContext(ctx);for(const name of names){const s=source.indexOf('  function '+name+'('),e=source.indexOf('\n  }',s);vm.runInContext(source.slice(s,e+4),ctx)}ctx.root=ctx;return ctx}
test('online sweep is serial, finite, failure clears stale values and zero survives',()=>{
 const pending=[],sent=[];
 const ctx=load(['fetchGuildStats'],{ready:true,guildStatsBusy:false,guildStats:{old:{online_count:99}},guilds:[{id:'1'},{id:'2'}],Date,Qt:{callLater(fn){fn()}},service:{backend:{sendCommand(name,fields,cb){sent.push(fields.guild_id);pending.push(cb)}}}});
 ctx.fetchGuildStats();assert.deepEqual(sent,['1']);assert.equal(ctx.guildStatsBusy,true);
 pending.shift()(true,{online_count:0});assert.deepEqual(sent,['1','2']);pending.shift()(false,null);
 assert.equal(ctx.guildStatsBusy,false);assert.equal(ctx.guildStats['1'].online_count,0);assert.equal(ctx.guildStats['2'],undefined);assert.equal(ctx.guildStats.old,undefined);
 ctx.guilds=Array.from({length:250},(_,i)=>({id:String(i)}));ctx.fetchGuildStats();for(let i=0;i<200;i++)pending.shift()(false,null);
 assert.equal(ctx.guildStatsBusy,false);assert.equal(sent.length,202);assert.equal(pending.length,0);
});
test('server writes are single-flight and keep errors visible',()=>{
 let finish;const ctx=load(['serverAction'],{ready:true,guildActionBusy:false,actionError:'',guildMute:{},Api:{assign:Object.assign,shallowCopy:x=>({...x})},service:{send(name,fields,cb){finish=cb;return 7}}});
 assert.equal(ctx.serverAction('set_guild_mute','1',{muted:true}),7);
 assert.equal(ctx.serverAction('leave_guild','1',{confirmed:true}),false);
 finish(false,null,'denied');assert.equal(ctx.guildActionBusy,false);assert.equal(ctx.actionError,'denied');assert.equal(ctx.guildMute['1'],undefined);
 ctx.serverAction('set_guild_mute','1',{muted:true});finish(true,{});assert.equal(ctx.guildMute['1'],true);
});
