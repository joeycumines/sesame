'use strict';

const {afterAll, beforeAll, describe, expect, it} = require('bun:test');
const {execFileSync} = require('node:child_process');
const fs = require('node:fs');
const os = require('node:os');
const path = require('node:path');

const {
  extractTarball,
  isForbiddenEntry,
  listEntries,
  normalizeEntry,
  requireSpecifiers,
  stripComments,
} = require('./pack-helpers');

describe('pack helpers: normalizeEntry', () => {
  it('trims, strips ./ prefixes and trailing slashes', () => {
    expect(normalizeEntry('./package/foo/')).toBe('package/foo');
    expect(normalizeEntry('  package/foo  ')).toBe('package/foo');
    expect(normalizeEntry('package/foo.js')).toBe('package/foo.js');
  });
});

describe('pack helpers: comment stripping', () => {
  it('finds requires outside comments', () => {
    expect(requireSpecifiers("require('a');")).toEqual(['a']);
    expect(requireSpecifiers('require("b");')).toEqual(['b']);
  });

  it('ignores requires inside comments', () => {
    expect(requireSpecifiers("// require('ghost');")).toEqual([]);
    expect(requireSpecifiers("/* require('ghost'); */")).toEqual([]);
    expect(
      requireSpecifiers("let a = 1; // require('ghost')\nrequire('b');"),
    ).toEqual(['b']);
  });

  it('keeps requires that follow strings containing //', () => {
    const source = "const url = 'https://example.com/a'; require('undici');";
    expect(requireSpecifiers(source)).toEqual(['undici']);
  });

  it('keeps requires that follow strings containing /* */ markers', () => {
    const source = "const a = '/*'; require('undici'); const b = '*/';";
    expect(requireSpecifiers(source)).toEqual(['undici']);
  });

  it('keeps requires after strings with escaped quotes', () => {
    const source = "const s = 'it\\'s // fine'; require('undici');";
    expect(requireSpecifiers(source)).toEqual(['undici']);
  });

  it('keeps requires after backtick templates containing quotes and //', () => {
    const source = 'const t = `say "hi" // now`; require(\'undici\');';
    expect(requireSpecifiers(source)).toEqual(['undici']);
  });

  it('strips real comments but preserves string literals', () => {
    expect(stripComments('const a = 1; // gone')).not.toContain('gone');
    expect(stripComments('const a = /* gone */ 1;')).not.toContain('gone');
    expect(stripComments("const s = '//';")).toContain("'//'");
  });
});

describe('pack helpers: forbidden entries', () => {
  it('matches entries inside a forbidden prefix', () => {
    expect(isForbiddenEntry('package/test/foo.js', ['package/test/'])).toBe(
      true,
    );
    expect(isForbiddenEntry('package/test', ['package/test/'])).toBe(true);
  });

  it('does not match sibling paths sharing a name stem', () => {
    expect(isForbiddenEntry('package/testing/foo.js', ['package/test/'])).toBe(
      false,
    );
    expect(isForbiddenEntry('package/test.js', ['package/test/'])).toBe(false);
    expect(isForbiddenEntry('package/testing', ['package/test'])).toBe(false);
  });

  it('normalizes prefixes without trailing slashes', () => {
    expect(isForbiddenEntry('package/test', ['package/test'])).toBe(true);
    expect(isForbiddenEntry('package/test/foo.js', ['package/test'])).toBe(
      true,
    );
    expect(isForbiddenEntry('package/other', ['package/test'])).toBe(false);
  });
});

describe('pack helpers: tar round-trip', () => {
  let tarDir;

  beforeAll(() => {
    // A space in the directory name guards against word-splitting and
    // path-handling regressions in the tar invocation.
    tarDir = fs.mkdtempSync(path.join(os.tmpdir(), 'sesame pack helpers-'));
    fs.mkdirSync(path.join(tarDir, 'package', 'dir'), {recursive: true});
    fs.mkdirSync(path.join(tarDir, 'package', 'empty'), {recursive: true});
    fs.writeFileSync(path.join(tarDir, 'package', 'one.txt'), 'one');
    fs.writeFileSync(path.join(tarDir, 'package', 'dir', 'two.txt'), 'two');
    execFileSync('tar', ['-czf', 'fixture.tgz', 'package'], {
      cwd: tarDir,
      encoding: 'utf8',
    });
  });

  afterAll(() => {
    fs.rmSync(tarDir, {recursive: true, force: true});
  });

  it('lists normalized entries including a bare directory name', () => {
    const entries = listEntries(tarDir, 'fixture.tgz');
    expect(entries).toContain('package/one.txt');
    expect(entries).toContain('package/dir/two.txt');
    expect(entries).toContain('package/empty');
    expect(entries.every(entry => !entry.endsWith('/'))).toBe(true);
  });

  it('extracts relative to the tarball directory', () => {
    fs.mkdirSync(path.join(tarDir, 'out'));
    extractTarball(tarDir, 'fixture.tgz', 'out');
    const root = path.join(tarDir, 'out', 'package');
    expect(fs.readFileSync(path.join(root, 'one.txt'), 'utf8')).toBe('one');
    expect(fs.readFileSync(path.join(root, 'dir', 'two.txt'), 'utf8')).toBe(
      'two',
    );
    expect(fs.statSync(path.join(root, 'empty')).isDirectory()).toBe(true);
  });
});
