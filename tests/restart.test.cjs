const test = require('node:test');
const assert = require('node:assert/strict');
const fs = require('node:fs');
const vm = require('node:vm');
const source = fs.readFileSync(require('node:path').join(__dirname,'../Service.qml'),'utf8');
const start=source.indexOf('  function restartBackend('),end=source.indexOf('\n  }',start);
const body=source.slice(start,end+4);
function fixture(blocked='') {
 let starts=0;const ctx={restartBusy:false,restartBlockedReason:blocked,backendClient:{wanted:true},daemonManager:{busy:false,setupBusy:false,runtimeAvailable:true,running:true,restart(){starts++;return true;}},noteActivity(){},fail(x){ctx.error=x;},statusMessage:''};
 ctx.root=ctx;vm.createContext(ctx);vm.runInContext(body,ctx);return {ctx,starts:()=>starts};
}
test('full restart reconnects UI transport once and blocks destructive in-flight states',()=>{
 for(const reason of ['Unsent draft','Pending send','Upload in progress','Sign-in in progress']) {
  const f=fixture(reason);assert.equal(f.ctx.restartBackend(),false);assert.equal(f.starts(),0);assert.equal(f.ctx.backendClient.wanted,true);assert.equal(f.ctx.error,reason);
 }
 const f=fixture();assert.equal(f.ctx.restartBackend(),true);assert.equal(f.ctx.restartBusy,true);assert.equal(f.ctx.backendClient.wanted,false);assert.equal(f.starts(),1);assert.equal(f.ctx.restartBackend(),false);assert.equal(f.starts(),1);
 const failed=fixture();failed.ctx.daemonManager.restart=()=>false;assert.equal(failed.ctx.restartBackend(),false);assert.equal(failed.ctx.restartBusy,false);assert.equal(failed.ctx.backendClient.wanted,true);
});
