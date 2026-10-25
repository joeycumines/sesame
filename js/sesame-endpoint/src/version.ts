import {readFileSync} from 'node:fs';
import {dirname, join, resolve} from 'node:path';

const PACKAGE_NAME = 'sesame-endpoint';
const MAX_PARENT_DEPTH = 8;

interface PackageManifest {
  name?: unknown;
  version?: unknown;
}

function readManifest(file: string): PackageManifest | undefined {
  let raw: string;
  try {
    raw = readFileSync(file, 'utf8');
  } catch {
    // If the file does not exist (ENOENT), is a directory (EISDIR), or cannot
    // be read due to permissions (EACCES/EPERM/ENOTDIR), this candidate file
    // cannot provide a valid manifest; return undefined so the upward walk
    // continues towards the package root.
    return undefined;
  }
  try {
    return JSON.parse(raw) as PackageManifest;
  } catch {
    // Not a manifest we can use; keep walking towards the package root.
    return undefined;
  }
}

/**
 * Resolves the version from the nearest `sesame-endpoint` package manifest.
 *
 * Source modules live in `src/` and compiled modules in `build/src/`, so a
 * fixed `../package.json` path would point at `build/package.json` after
 * compilation. That emitted copy is not part of the published package; this
 * walk finds the one authoritative manifest at the package root in both
 * layouts.
 *
 * @param startDir Optional starting directory (defaults to `__dirname`).
 */
export function getVersion(startDir: string = __dirname): string {
  // Anchor the walk to an absolute directory up front: a relative startDir
  // would otherwise be re-joined against process.cwd() at every step, so the
  // walk could never climb above the caller's working directory.
  let dir = resolve(startDir);
  for (let depth = 0; depth < MAX_PARENT_DEPTH; depth++) {
    const manifest = readManifest(join(dir, 'package.json'));
    if (
      manifest?.name === PACKAGE_NAME &&
      typeof manifest.version === 'string' &&
      manifest.version.length > 0
    ) {
      return manifest.version;
    }
    const parent = dirname(dir);
    if (parent === dir) {
      break;
    }
    dir = parent;
  }
  throw new Error(
    `unable to resolve ${PACKAGE_NAME} version: no package.json with that name above ${startDir}`,
  );
}
