const test = require('node:test');
const assert = require('node:assert/strict');
const fs = require('node:fs');
const path = require('node:path');
const os = require('node:os');
const cp = require('node:child_process');
const root = path.resolve(__dirname, '..');

function fixture(t) {
  const dir = fs.mkdtempSync(path.join(os.tmpdir(), 'omacord-build-'));
  t.after(() => fs.rmSync(dir, {recursive: true, force: true}));
  const source = path.join(dir, 'source');
  fs.mkdirSync(path.join(source, 'scripts'), {recursive: true});
  fs.copyFileSync(path.join(root, 'scripts/build-backend.sh'), path.join(source, 'scripts/build-backend.sh'));
  fs.cpSync(path.join(root, 'backend'), path.join(source, 'backend'), {recursive: true});
  const bin = path.join(dir, 'bin');
  fs.mkdirSync(bin);
  fs.writeFileSync(path.join(bin, 'uname'), '#!/bin/sh\necho x86_64\n', {mode: 0o755});
  fs.writeFileSync(path.join(bin, 'ldd'), '#!/bin/sh\nexit 0\n', {mode: 0o755});
  fs.writeFileSync(path.join(bin, 'go'), `#!/bin/sh
printf build >> "$TEST_LOG"
while [ "$#" -gt 0 ]; do
  if [ "$1" = -o ]; then shift; printf 'inert freshly built backend' > "$1"; exit 0; fi
  shift
done
exit 1
`, {mode: 0o755});
  const env = {HOME: dir, XDG_CACHE_HOME: path.join(dir, 'cache'),
    OMARCHY_DISCORD_RUNTIME_DIR: path.join(dir, 'runtime'), PATH: bin + ':/usr/bin:/bin',
    TEST_LOG: path.join(dir, 'build.log')};
  return {source, env, run() {
    return cp.spawnSync('/usr/bin/bash', [path.join(source, 'scripts/build-backend.sh')],
      {env, encoding: 'utf8', timeout: 10000});
  }};
}

test('bundled backend matches current production-source fingerprint', t => {
  const f = fixture(t);
  const result = f.run();
  assert.equal(result.status, 0, result.stderr);
  assert.match(result.stdout, /Installed bundled/);
  assert.equal(fs.existsSync(f.env.TEST_LOG), false);
  assert.deepEqual(fs.readFileSync(path.join(f.env.OMARCHY_DISCORD_RUNTIME_DIR, 'omarchy-discord-backend')),
    fs.readFileSync(path.join(root, 'backend/dist/x86_64/omarchy-discord-backend')));
});

for (const mode of ['changed source', 'missing stamp']) test('build rejects bundled backend with ' + mode, t => {
  const f = fixture(t);
  if (mode === 'changed source') fs.appendFileSync(path.join(f.source, 'backend/internal/remoteauth/remoteauth.go'), '\n// Inert changed-source fixture.\n');
  else fs.unlinkSync(path.join(f.source, 'backend/dist/x86_64/source.sha256'));
  const result = f.run();
  assert.equal(result.status, 0, result.stderr);
  assert.match(result.stdout, /Built and installed/);
  assert.equal(fs.readFileSync(f.env.TEST_LOG, 'utf8'), 'build');
  assert.equal(fs.readFileSync(path.join(f.env.OMARCHY_DISCORD_RUNTIME_DIR, 'omarchy-discord-backend'), 'utf8'),
    'inert freshly built backend');
});
