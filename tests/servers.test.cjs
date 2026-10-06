const fs=require('node:fs');
const vm=require('node:vm');
const test=require('node:test');
const assert=require('node:assert/strict');
const ctx=vm.createContext({});
vm.runInContext(fs.readFileSync('ServerList.js','utf8').replace(/^\.pragma library\n/,''),ctx);
const rows=[{id:'1',name:'Zulu',position:0,unread:'unread'},{id:'2',name:'Alpha',position:1,mention_count:3,unread:'mentioned'},{id:'3',name:'Beta',position:2,unread:'read'}];
const ids=r=>Array.from(r,x=>x.id);
test('server filters and sort are stable, nonmutating and Unicode-friendly',()=>{
 assert.deepEqual(ids(ctx.rows(rows,'','all','name',{})),['2','3','1']);
 assert.deepEqual(ids(ctx.rows(rows,'  ALP ','all','name',{})),['2']);
 assert.deepEqual(ids(ctx.rows(rows,'','unread','position',{})),['1','2']);
 assert.deepEqual(ids(ctx.rows(rows,'','mentions','mentions',{})),['2']);
 assert.deepEqual(ids(ctx.rows(rows,'missing','all','name',{})),[]);
 assert.deepEqual(ids(rows),['1','2','3']);
 assert.equal(ctx.rows([{id:'4',name:'Сообщество'}],'СООБ','all','name',{}).length,1);
});
test('online sort puts unknown after genuine zero and handles ties',()=>{
 assert.deepEqual(ids(ctx.rows(rows,'','all','online',{'2':{online_count:0},'3':{online_count:9}})),['3','2','1']);
 assert.equal(ctx.knownCount({'1':{online_count:null}},'1'),null);
 assert.equal(ctx.knownCount({'1':{online_count:0}},'1'),0);
 assert.equal(ctx.knownCount({'1':{online_count:-1}},'1'),null);
 assert.deepEqual(ids(ctx.rows(rows,'','all','online',{'1':{online_count:9},'2':{online_count:9}})),['1','2','3']);
});
