'use strict';

// Shared helpers for the packed-artifact verification runner and its unit
// tests. CommonJS so the plain-JS runner can require it directly, and plain
// JS (not TS) so tests can require it without a declaration file.

const {execFileSync} = require('node:child_process');

// Trims tar listing output, drops any leading './' and any trailing '/'
// (tar prints directory entries with a trailing slash).
function normalizeEntry(entry) {
  return entry.trim().replace(/^\.\//, '').replace(/\/$/, '');
}

// Reports whether an entry lies inside one of the given forbidden prefixes.
// Trims trailing slashes from the prefix so a bare directory entry (tar
// prints directories with a trailing slash, which normalizeEntry removes)
// still matches: 'package/test' is forbidden for prefix 'package/test/'.
function isForbiddenEntry(entry, prefixes) {
  return prefixes.some(prefix => {
    const base = prefix.replace(/\/+$/, '');
    return entry === base || entry.startsWith(`${base}/`);
  });
}

// Removes JavaScript comments so require mentions inside comments cannot
// false-positive a dependency scan. String literals (including template
// literals and escaped quotes) are matched first and preserved verbatim, so
// comment markers inside strings — 'https://...', '/*', '*/' — neither
// swallow real code nor corrupt the source.
function stripComments(source) {
  return source.replace(
    /("(?:\\[\s\S]|[^"\\])*"|'(?:\\[\s\S]|[^'\\])*'|`(?:\\[\s\S]|[^`\\])*`)|\/\*[\s\S]*?\*\/|\/\/.*/g,
    (match, quoted) => quoted ?? '',
  );
}

// Returns every require(...) specifier in the source, in source order,
// after comments have been stripped.
function requireSpecifiers(source) {
  const specifiers = [];
  const pattern = /require\((['"])([^'"]+)\1\)/g;
  for (const match of stripComments(source).matchAll(pattern)) {
    specifiers.push(match[2]);
  }
  return specifiers;
}

// Lists the normalized entry names of a tarball, invoking tar from the
// directory that holds it so only relative names cross the process
// boundary.
function listEntries(tarDir, tarballName) {
  return execFileSync('tar', ['-tzf', tarballName], {
    cwd: tarDir,
    encoding: 'utf8',
  })
    .split('\n')
    .map(normalizeEntry)
    .filter(entry => entry.length > 0);
}

// Extracts a tarball into destinationName, a directory relative to tarDir
// that the caller has already created.
function extractTarball(tarDir, tarballName, destinationName) {
  execFileSync('tar', ['-xzf', tarballName, '-C', destinationName], {
    cwd: tarDir,
    encoding: 'utf8',
  });
}

module.exports = {
  extractTarball,
  isForbiddenEntry,
  listEntries,
  normalizeEntry,
  requireSpecifiers,
  stripComments,
};
