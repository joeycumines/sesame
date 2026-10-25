'use strict';

const assert = require('node:assert');
const {execFileSync} = require('node:child_process');
const fs = require('node:fs');
const {builtinModules} = require('node:module');
const os = require('node:os');
const path = require('node:path');

const packageRoot = path.join(__dirname, '..');
const keep = process.argv.includes('--keep');
const bun = process.env.BUN || 'bun';

const {
  extractTarball,
  isForbiddenEntry,
  listEntries,
  requireSpecifiers,
} = require('./pack-helpers');

function run(command, args, options = {}) {
  return execFileSync(command, args, {
    cwd: packageRoot,
    encoding: 'utf8',
    ...options,
  });
}

function listJavaScriptFiles(dir) {
  const files = [];
  for (const entry of fs.readdirSync(dir, {withFileTypes: true})) {
    const fullPath = path.join(dir, entry.name);
    if (entry.isDirectory()) {
      files.push(...listJavaScriptFiles(fullPath));
    } else if (entry.name.endsWith('.js')) {
      files.push(fullPath);
    }
  }
  return files;
}

function packageName(specifier) {
  if (specifier.startsWith('@')) {
    return specifier.split('/').slice(0, 2).join('/');
  }
  return specifier.split('/')[0];
}

function main() {
  const tempDir = fs.mkdtempSync(
    path.join(os.tmpdir(), 'sesame-endpoint-pack-'),
  );
  try {
    run(bun, ['pm', 'pack', '--destination', tempDir]);
    const tarballs = fs
      .readdirSync(tempDir)
      .filter(name => name.endsWith('.tgz'));
    assert.strictEqual(
      tarballs.length,
      1,
      `expected exactly one tarball, found: ${tarballs.join(', ')}`,
    );
    const tarball = path.join(tempDir, tarballs[0]);
    const entries = listEntries(tempDir, tarballs[0]);

    for (const required of [
      'package/package.json',
      'package/LICENSE',
      'package/README.md',
      'package/build/src/cli.js',
    ]) {
      assert.ok(entries.includes(required), `tarball must contain ${required}`);
    }
    assert.ok(
      !entries.includes('package/build/package.json'),
      'tarball must not contain the emitted build/package.json',
    );
    const forbiddenEntries = entries.filter(entry =>
      isForbiddenEntry(entry, [
        'package/build/test/',
        'package/node_modules/',
        'package/src/',
        'package/test/',
      ]),
    );
    assert.ok(
      forbiddenEntries.length === 0,
      `tarball must not contain: ${forbiddenEntries.join(', ')}`,
    );

    const extractDir = path.join(tempDir, 'extract');
    fs.mkdirSync(extractDir);
    extractTarball(tempDir, tarballs[0], 'extract');
    const extractedRoot = path.join(extractDir, 'package');
    const cliPath = path.join(extractedRoot, 'build', 'src', 'cli.js');
    const manifest = JSON.parse(
      fs.readFileSync(path.join(extractedRoot, 'package.json'), 'utf8'),
    );
    const runtimeDependencies = new Set(
      Object.keys(manifest.dependencies || {}),
    );
    for (const dependency of [
      '@bufbuild/protobuf',
      '@connectrpc/connect',
      '@connectrpc/connect-node',
    ]) {
      assert.ok(
        runtimeDependencies.has(dependency),
        `package.json must declare runtime dependency ${dependency}`,
      );
    }

    assert.strictEqual(
      manifest.bin && manifest.bin['sesame-endpoint'],
      'build/src/cli.js',
      'package.json must expose the sesame-endpoint bin target',
    );
    for (const field of ['main', 'types']) {
      assert.strictEqual(
        typeof manifest[field],
        'string',
        `package.json must declare ${field}`,
      );
      assert.ok(
        fs.existsSync(path.join(extractedRoot, manifest[field])),
        `package.json ${field} target must exist: ${manifest[field]}`,
      );
    }

    // Every bare require in the build must be a declared runtime dependency;
    // NODE_PATH below must not be able to hide an omitted dependency.
    // requireSpecifiers strips comments itself, so require mentions in
    // comments/docs cannot false-positive.
    const builtins = new Set(builtinModules);
    for (const file of listJavaScriptFiles(
      path.join(extractedRoot, 'build', 'src'),
    )) {
      const source = fs.readFileSync(file, 'utf8');
      for (const specifier of requireSpecifiers(source)) {
        if (
          specifier.startsWith('.') ||
          specifier.startsWith('node:') ||
          specifier === 'bun' ||
          specifier.startsWith('bun:') ||
          builtins.has(specifier)
        ) {
          continue;
        }
        const dependency = packageName(specifier);
        assert.ok(
          runtimeDependencies.has(dependency),
          `${path.relative(extractedRoot, file)} requires undeclared dependency ${dependency}`,
        );
      }
    }

    // The tarball never contains node_modules; an installed package resolves
    // dependencies from the surrounding install tree. Supply the workspace's
    // installed tree through NODE_PATH (honoured by both Node.js and Bun) so
    // the extracted CLI executes exactly as it would after installation.
    const runtimeEnv = {
      ...Object.fromEntries(
        Object.entries(process.env).filter(
          ([key]) => !key.startsWith('SESAME_ENDPOINT_'),
        ),
      ),
      NODE_PATH: [path.join(packageRoot, 'node_modules'), process.env.NODE_PATH]
        .filter(Boolean)
        .join(path.delimiter),
    };
    const expectedVersion = `sesame-endpoint v${manifest.version}\n`;

    const mainPath = path.join(extractedRoot, manifest.main);
    for (const runtime of [process.execPath, bun]) {
      run(runtime, ['-e', `require(${JSON.stringify(mainPath)})`], {
        cwd: extractedRoot,
        env: runtimeEnv,
      });
    }

    assert.strictEqual(
      run(process.execPath, [cliPath, '--version'], {
        cwd: extractedRoot,
        env: runtimeEnv,
      }),
      expectedVersion,
    );
    assert.ok(
      run(process.execPath, [cliPath, '--help'], {
        cwd: extractedRoot,
        env: runtimeEnv,
      }).includes('Usage: sesame-endpoint'),
      'extracted CLI --help must include usage',
    );
    assert.strictEqual(
      run(bun, [cliPath, '--version'], {cwd: extractedRoot, env: runtimeEnv}),
      expectedVersion,
    );

    if (keep) {
      for (const name of fs.readdirSync(packageRoot)) {
        if (name.endsWith('.tgz')) {
          fs.rmSync(path.join(packageRoot, name), {force: true});
        }
      }
      const destination = path.join(packageRoot, tarballs[0]);
      fs.copyFileSync(tarball, destination);
      process.stdout.write(`verified packed artifact: ${destination}\n`);
    } else {
      process.stdout.write(
        `verified packed artifact: ${tarballs[0]} (${entries.length} entries)\n`,
      );
    }
  } finally {
    fs.rmSync(tempDir, {recursive: true, force: true});
  }
}

main();
