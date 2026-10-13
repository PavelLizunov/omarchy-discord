const test = require('node:test');
const assert = require('node:assert/strict');
const fs = require('node:fs');
const path = require('node:path');
const os = require('node:os');
const {spawnSync} = require('node:child_process');
const root = path.resolve(__dirname, '..');

test('repository guard rejects changed upstream bytes, hook growth and stale package stamps', () => {
  const tmp = fs.mkdtempSync(path.join(os.tmpdir(), 'omacord-guard-'));
  try {
    fs.mkdirSync(path.join(tmp, 'scripts'));
    fs.mkdirSync(path.join(tmp, 'docs'));
    const marker = path.join(tmp, 'marker');
    fs.writeFileSync(marker, 'baseline');
    const program = fs.readFileSync(path.join(root, 'scripts/check-repository.py'), 'utf8');
    const runner = path.join(tmp, 'probe.py');
    const adapter = '\ndef git(*args):\n    b = json.loads((ROOT / "scripts/fork-boundary.json").read_text())\n    if args[0] == "ls-tree": return "\\n".join(sorted(set(b["preserved"]) | set(b["hooks"]) | set(b["removed"]))).encode()\n    if args[0] == "diff": return b"114\\t37\\tBarWidget.qml\\n"\n    return b""\n\n';
    const source = program.replace('ROOT = Path(__file__).resolve().parent.parent', `ROOT = Path(${JSON.stringify(root)})`).replace('def main():', adapter + 'def main():');
    fs.writeFileSync(runner, source);
    const r = spawnSync('python3', ['-B', runner], {encoding: 'utf8'});
    assert.equal(r.status, 0, r.stdout + r.stderr);
    assert.equal(fs.readFileSync(marker, 'utf8'), 'baseline');
    // Negative fixtures replace checker inputs in memory, never edit the real checkout.
    const cases = [
      ['upstream bytes changed', 'boundary["preserved"]["LICENSE"] = "0" * 64'],
      ['upstream hook grew', 'boundary["hooks"]["BarWidget.qml"]["added"] = 0'],
      ['bundled backend source fingerprint is stale', 'expected = "0" * 64']
    ];
    for (const [message, mutation] of cases) {
      const anchor = mutation.startsWith('expected')
        ? "expected = (backend / 'dist/x86_64/source.sha256').read_text().strip()"
        : "errors = []";
      const negative = fs.readFileSync(runner, 'utf8').replace(anchor, anchor + '\n    ' + mutation);
      const filename = path.join(tmp, 'negative.py');
      fs.writeFileSync(filename, negative);
      const result = spawnSync('python3', ['-B', filename], {encoding: 'utf8'});
      assert.equal(result.status, 1, result.stdout + result.stderr);
      assert(result.stdout.includes(message), result.stdout);
    }
  } finally {
    fs.rmSync(tmp, {recursive: true, force: true});
  }
});
