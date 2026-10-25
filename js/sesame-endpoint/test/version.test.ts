import {describe, expect, it} from 'bun:test';
import {spawnSync} from 'node:child_process';
import * as fs from 'node:fs';
import * as os from 'node:os';
import * as path from 'node:path';
import {getVersion} from '../src/version';

describe('version resolution', () => {
  it('reports the root package manifest version', () => {
    const manifest = JSON.parse(
      fs.readFileSync(path.join(__dirname, '..', 'package.json'), 'utf8'),
    ) as {version?: unknown};
    const version = manifest.version;
    if (typeof version !== 'string') {
      throw new Error('package.json version must be a string');
    }
    expect(getVersion()).toBe(version);
  });

  it('resolves version when starting from deeply nested subdirectories', () => {
    const tmp = fs.mkdtempSync(path.join(os.tmpdir(), 'version-nested-'));
    try {
      const rootDir = path.join(tmp, 'pkg');
      const deepDir = path.join(rootDir, 'a', 'b', 'c');
      fs.mkdirSync(deepDir, {recursive: true});
      fs.writeFileSync(
        path.join(rootDir, 'package.json'),
        JSON.stringify({name: 'sesame-endpoint', version: '1.2.3'}),
      );
      expect(getVersion(deepDir)).toBe('1.2.3');
    } finally {
      fs.rmSync(tmp, {recursive: true, force: true});
    }
  });

  it('skips a directory named package.json (EISDIR) and finds valid parent manifest', () => {
    const tmp = fs.mkdtempSync(path.join(os.tmpdir(), 'version-isdir-'));
    try {
      const rootDir = path.join(tmp, 'pkg');
      const childDir = path.join(rootDir, 'child');
      fs.mkdirSync(childDir, {recursive: true});
      // Create a directory named package.json in childDir
      fs.mkdirSync(path.join(childDir, 'package.json'));
      // Create a valid manifest in parent
      fs.writeFileSync(
        path.join(rootDir, 'package.json'),
        JSON.stringify({name: 'sesame-endpoint', version: '2.3.4'}),
      );
      expect(getVersion(childDir)).toBe('2.3.4');
    } finally {
      fs.rmSync(tmp, {recursive: true, force: true});
    }
  });

  it('skips malformed JSON or unreadable package.json files and continues upward', () => {
    const tmp = fs.mkdtempSync(path.join(os.tmpdir(), 'version-malformed-'));
    try {
      const rootDir = path.join(tmp, 'pkg');
      const childDir = path.join(rootDir, 'child');
      fs.mkdirSync(childDir, {recursive: true});
      // Malformed JSON in child
      fs.writeFileSync(path.join(childDir, 'package.json'), '{invalid json');
      // Valid in parent
      fs.writeFileSync(
        path.join(rootDir, 'package.json'),
        JSON.stringify({name: 'sesame-endpoint', version: '3.4.5'}),
      );
      expect(getVersion(childDir)).toBe('3.4.5');
    } finally {
      fs.rmSync(tmp, {recursive: true, force: true});
    }
  });

  it('skips package.json with mismatched name or invalid version', () => {
    const tmp = fs.mkdtempSync(path.join(os.tmpdir(), 'version-mismatch-'));
    try {
      const rootDir = path.join(tmp, 'pkg');
      const childDir = path.join(rootDir, 'child');
      const grandChild = path.join(childDir, 'grand');
      fs.mkdirSync(grandChild, {recursive: true});

      // Different package name in grandchild
      fs.writeFileSync(
        path.join(grandChild, 'package.json'),
        JSON.stringify({name: 'other-package', version: '9.9.9'}),
      );
      // Empty version string in child
      fs.writeFileSync(
        path.join(childDir, 'package.json'),
        JSON.stringify({name: 'sesame-endpoint', version: ''}),
      );
      // Valid target in parent
      fs.writeFileSync(
        path.join(rootDir, 'package.json'),
        JSON.stringify({name: 'sesame-endpoint', version: '4.5.6'}),
      );

      expect(getVersion(grandChild)).toBe('4.5.6');
    } finally {
      fs.rmSync(tmp, {recursive: true, force: true});
    }
  });

  it('throws descriptive error when no sesame-endpoint manifest exists in parent tree', () => {
    const tmp = fs.mkdtempSync(path.join(os.tmpdir(), 'version-missing-'));
    try {
      const sub = path.join(tmp, 'sub');
      fs.mkdirSync(sub);
      expect(() => getVersion(sub)).toThrow(
        /unable to resolve sesame-endpoint version: no package\.json with that name above/,
      );
    } finally {
      fs.rmSync(tmp, {recursive: true, force: true});
    }
  });

  it('resolves a relative startDir against process.cwd() of the caller', () => {
    // A relative startDir must be resolve()d once against process.cwd(),
    // then walked as absolute ancestors. Proven in a subprocess: bun test
    // runs from the package root, where even a relative walk would find
    // the real package.json, masking the bug.
    const tmp = fs.mkdtempSync(path.join(os.tmpdir(), 'version-relative-'));
    try {
      const rootDir = path.join(tmp, 'pkg');
      const cwdDir = path.join(rootDir, 'a', 'b');
      fs.mkdirSync(cwdDir, {recursive: true});
      fs.writeFileSync(
        path.join(rootDir, 'package.json'),
        JSON.stringify({name: 'sesame-endpoint', version: '5.6.7'}),
      );
      const runner = path.join(tmp, 'runner.ts');
      fs.writeFileSync(
        runner,
        [
          'import {getVersion} from ' +
            JSON.stringify(path.join(__dirname, '..', 'src', 'version.ts')) +
            ';',
          "process.stdout.write(getVersion('sub/dir') + '\\n');",
          '',
        ].join('\n'),
      );
      const res = spawnSync(process.execPath, [runner], {
        cwd: cwdDir,
        encoding: 'utf8',
        timeout: 30000,
      });
      expect(res.status).toBe(0);
      expect(res.stderr).toBe('');
      expect(res.stdout).toBe('5.6.7\n');
    } finally {
      fs.rmSync(tmp, {recursive: true, force: true});
    }
  });
});
