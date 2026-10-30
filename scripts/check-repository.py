#!/usr/bin/env python3
"""Check the pinned fork boundary, documentation and package source identity."""
import argparse
import hashlib
import json
from pathlib import Path
import re
import subprocess
import sys

ROOT = Path(__file__).resolve().parent.parent

def git(*args):
    return subprocess.check_output(['git', '-C', str(ROOT), '-c', 'diff.algorithm=histogram', *args], stderr=subprocess.PIPE)

def main():
    parser = argparse.ArgumentParser()
    parser.add_argument('--upstream-ref')
    args = parser.parse_args()
    boundary = json.loads((ROOT / 'scripts/fork-boundary.json').read_text())
    errors = []
    for name, expected in boundary['preserved'].items():
        path = ROOT / name
        if not path.is_file() or path.is_symlink() or hashlib.sha256(path.read_bytes()).hexdigest() != expected:
            errors.append('upstream bytes changed: ' + name)
    for name in boundary['removed']:
        if (ROOT / name).exists():
            errors.append('upstream deletion reintroduced without review: ' + name)
    upstream = args.upstream_ref or boundary['upstream']
    try:
        git('cat-file', '-e', upstream + '^{commit}')
        upstream_paths = set(git('ls-tree', '-r', '--name-only', upstream).decode().splitlines())
        allowed = set(boundary['hooks']) | set(boundary['removed']) | set(boundary['preserved'])
        if upstream_paths != allowed:
            errors.append('upstream file list differs from the pinned boundary')
        changes = git('diff', '--numstat', '--no-renames', upstream, '--').decode().splitlines()
        for row in changes:
            add, remove, name = row.split('\t', 2)
            if name not in upstream_paths:
                continue
            if name in boundary['removed']:
                continue
            limit = boundary['hooks'].get(name)
            if limit is not None and limit.get('sha256'):
                if hashlib.sha256((ROOT / name).read_bytes()).hexdigest() != limit['sha256']:
                    errors.append('pinned binary hook changed: ' + name)
            if limit is None:
                errors.append('unexpected upstream delta: ' + name)
            elif limit['added'] is not None and (not add.isdigit() or int(add) > limit['added'] or int(remove) > limit['removed']):
                errors.append('upstream hook grew: ' + name)
    except subprocess.CalledProcessError:
        errors.append('pinned upstream Git revision unavailable; use a verified --upstream-ref')
    for name, expected in boundary.get('derived_preserved', {}).items():
        path = ROOT / name
        if not path.is_file() or hashlib.sha256(path.read_bytes()).hexdigest() != expected:
            errors.append('vendored transport changed during hygiene: ' + name)
    instructions = ['AGENTS.md', 'docs/development.md']
    lines = (ROOT / instructions[0]).read_text().splitlines()
    if not 8 <= len(lines) <= 25:
        errors.append('root instruction file must stay between 8 and 25 lines')
    extra_instructions = [str(p.relative_to(ROOT)) for p in ROOT.rglob('*')
                          if p.name in ('AGENTS.md', 'CLAUDE.md', 'GEMINI.md', 'SKILL.md')
                          and '.git' not in p.parts and str(p.relative_to(ROOT)) not in instructions]
    if extra_instructions:
        errors.append('unlisted instruction files: ' + ', '.join(extra_instructions))
    for name in instructions:
        if not (ROOT / name).is_file():
            errors.append('instruction missing: ' + name)
    for path in [ROOT / 'README.md', ROOT / 'AGENTS.md', *sorted((ROOT / 'docs').rglob('*.md'))]:
        for target in re.findall(r'(?<!!)\[[^\]]*\]\(([^\s)]+)\)', path.read_text()):
            target = target.split('#', 1)[0]
            if not target or '://' in target or target.startswith('mailto:'):
                continue
            if not (path.parent / target).exists():
                errors.append('broken link in ' + str(path.relative_to(ROOT)) + ': ' + target)
    if any(p.is_symlink() for p in ROOT.rglob('*') if '.git' not in p.parts):
        errors.append('repository symlink found')
    backend = ROOT / 'backend'
    source = [backend / 'go.mod', backend / 'go.sum', backend / '../scripts/capture-screen.py']
    source += sorted(p for folder in ('cmd', 'internal', 'native') for p in (backend / folder).rglob('*') if p.is_file() and not p.name.endswith('_test.go'))
    manifest = ''.join(hashlib.sha256(p.read_bytes()).hexdigest() + '  ' + str(p.relative_to(backend)) + '\n' for p in source)
    expected = (backend / 'dist/x86_64/source.sha256').read_text().strip()
    if hashlib.sha256(manifest.encode()).hexdigest() != expected:
        errors.append('bundled backend source fingerprint is stale')
    for error in errors:
        print('FAIL:', error)
    print('Repository checks:', 'FAIL' if errors else 'PASS', '- upstream bytes, hook limits, instructions, links, source stamp')
    return bool(errors)

if __name__ == '__main__':
    sys.exit(main())
