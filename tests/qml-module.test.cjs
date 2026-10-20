const test = require('node:test');
const assert = require('node:assert/strict');
const fs = require('node:fs');
const path = require('node:path');

test('the local UI module exports components as well as singletons', () => {
  const ui = path.join(__dirname, '../ui');
  const declarations = fs.readFileSync(path.join(ui, 'qmldir'), 'utf8').trim().split('\n');
  for (const file of fs.readdirSync(ui).filter(name => name.endsWith('.qml'))) {
    const singleton = /^pragma Singleton$/m.test(fs.readFileSync(path.join(ui, file), 'utf8'));
    const declaration = `${singleton ? 'singleton ' : ''}${file.slice(0, -4)} 1.0 ${file}`;
    assert.ok(declarations.includes(declaration), `Missing export: ${declaration}`);
  }
});
