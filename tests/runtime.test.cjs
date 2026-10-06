const test = require('node:test');
const assert = require('node:assert/strict');
const fs = require('node:fs');
const path = require('node:path');
const os = require('node:os');
const cp = require('node:child_process');

function fixture(t) {
  const dir = fs.mkdtempSync(path.join(os.tmpdir(), 'omacord-runtime-'));
  t.after(() => fs.rmSync(dir, {recursive: true, force: true}));
  const source = path.join(dir, 'source');
  const bin = path.join(dir, 'bin');
  fs.mkdirSync(path.join(source, 'scripts'), {recursive: true});
  fs.mkdirSync(bin);
  for (const name of ['backend-runtime.sh', 'remove-runtime.sh'])
    fs.copyFileSync(path.join(__dirname, '../scripts', name), path.join(source, 'scripts', name));
  fs.chmodSync(path.join(source, 'scripts/backend-runtime.sh'), 0o755);
  fs.writeFileSync(path.join(bin, 'systemctl'), `#!/bin/sh
if [ "$TEST_BUS_FAIL" = 1 ]; then exit 1; fi
case "$2" in
 show) case "$3" in
   --property=LoadState) echo "$TEST_LOAD";;
   --property=ActiveState) echo "$TEST_ACTIVE";;
 esac;;
 stop) printf stop >> "$TEST_LOG"; exit "$TEST_STOP_EXIT";;
 daemon-reload) exit 0;;
 *) exit 2;;
esac
`, {mode: 0o755});
  fs.writeFileSync(path.join(bin, 'secret-tool'), '#!/bin/sh\nexit 1\n', {mode: 0o755});
  const config = path.join(dir, 'config');
  const runtime = path.join(dir, 'runtime');
  const cache = path.join(dir, 'cache');
  fs.mkdirSync(path.join(config, 'systemd/user'), {recursive: true});
  fs.mkdirSync(path.join(config, 'omarchy-discord'));
  fs.mkdirSync(runtime);
  fs.mkdirSync(path.join(cache, 'omarchy-discord'), {recursive: true});
  const files = [path.join(config, 'systemd/user/omarchy-discord.service'),
    path.join(config, 'omarchy-discord/preferences'), path.join(runtime, 'omarchy-discord-backend')];
  for (const file of files) fs.writeFileSync(file, 'inert fixture');
  const env = {HOME: dir, XDG_CONFIG_HOME: config, XDG_CACHE_HOME: cache,
    OMARCHY_DISCORD_RUNTIME_DIR: runtime, PATH: bin + ':/usr/bin:/bin',
    TEST_LOAD: 'loaded', TEST_ACTIVE: 'inactive', TEST_STOP_EXIT: '0', TEST_LOG: path.join(dir, 'stop.log')};
  return {files, env, run(script, args = [], extra = {}) {
    return cp.spawnSync('/usr/bin/bash', [path.join(source, 'scripts', script), ...args],
      {env: {...env, ...extra}, encoding: 'utf8', timeout: 5000});
  }};
}

test('stop propagates failure and rejects a still-active backend', t => {
  const f = fixture(t);
  assert.equal(f.run('backend-runtime.sh', ['stop'], {TEST_STOP_EXIT: '7'}).status, 7);
  const active = f.run('backend-runtime.sh', ['stop'], {TEST_ACTIVE: 'active'});
  assert.equal(active.status, 1);
  assert.match(active.stderr, /not stopped/);
  for (const state of ['inactive', 'failed'])
    assert.equal(f.run('backend-runtime.sh', ['stop'], {TEST_ACTIVE: state}).status, 0);
});

test('missing unit is harmless but unavailable systemd is not success', t => {
  const f = fixture(t);
  assert.equal(f.run('backend-runtime.sh', ['stop'], {TEST_LOAD: 'not-found'}).status, 0);
  assert.equal(fs.existsSync(f.env.TEST_LOG), false);
  assert.notEqual(f.run('backend-runtime.sh', ['stop'], {TEST_BUS_FAIL: '1'}).status, 0);
});

test('removal preserves integration and user data when shutdown fails', t => {
  const f = fixture(t);
  assert.notEqual(f.run('remove-runtime.sh', ['--purge'], {TEST_STOP_EXIT: '7'}).status, 0);
  for (const file of f.files) assert.equal(fs.readFileSync(file, 'utf8'), 'inert fixture');
});

test('purge reports an attempt rather than claiming keyring verification', t => {
  const f = fixture(t);
  const result = f.run('remove-runtime.sh', ['--purge']);
  assert.equal(result.status, 0, result.stderr);
  assert.match(result.stdout, /Attempted to clear matching/);
  assert.doesNotMatch(result.stdout, /Cleared matching/);
});
