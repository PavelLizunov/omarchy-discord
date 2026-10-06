const test = require('node:test');
const assert = require('node:assert/strict');
const fs = require('node:fs');
const os = require('node:os');
const path = require('node:path');
const cp = require('node:child_process');
const installer = fs.readFileSync(path.join(__dirname,'../scripts/install-local.sh'));
function fixture(t) {
  const dir = fs.mkdtempSync(path.join(os.tmpdir(),'omacord-install-'));
  t.after(() => fs.rmSync(dir,{recursive:true,force:true}));
  const scripts = path.join(dir,'source/scripts');
  const bin = path.join(dir,'bin');
  fs.mkdirSync(scripts,{recursive:true});
  fs.mkdirSync(bin);
  fs.writeFileSync(path.join(scripts,'install-local.sh'),installer,{mode:0o755});
  const log = path.join(dir,'side-effects');
  fs.writeFileSync(path.join(scripts,'setup.sh'),'#!/bin/sh\nprintf setup >> "$TEST_LOG"\n',{mode:0o755});
  for (const name of ['omarchy','omarchy-shell','rsync','jq']) fs.writeFileSync(path.join(bin,name),'#!/bin/sh\nexit 0\n',{mode:0o755});
  fs.writeFileSync(path.join(bin,'systemctl'),'#!/bin/sh\nprintf systemctl >> "$TEST_LOG"\n',{mode:0o755});
  const config = path.join(dir,'config');
  const target = path.join(config,'omarchy/plugins/quickshell.discord');
  fs.mkdirSync(path.dirname(target),{recursive:true});
  return {dir,scripts,log,target,env:{...process.env,HOME:dir,XDG_CONFIG_HOME:config,PATH:bin+':/usr/bin:/bin',TEST_LOG:log}};
}
for (const kind of ['symlink','non-plugin directory']) test('installer rejects '+kind+' before runtime changes',t => {
  const f = fixture(t);
  if (kind === 'symlink') fs.symlinkSync(f.dir,f.target);
  else fs.mkdirSync(f.target);
  const result = cp.spawnSync('bash',[path.join(f.scripts,'install-local.sh')],{env:f.env,encoding:'utf8',timeout:5000});
  assert.equal(result.status,1,result.stderr);
  assert.equal(fs.existsSync(f.log),false,'setup or systemctl ran before target validation');
});
