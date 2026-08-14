'use strict';

const test = require('node:test');
const assert = require('node:assert/strict');
const { releaseTarget } = require('../lib/platform');

test('maps all published release targets', () => {
  assert.deepEqual(releaseTarget('linux', 'x64'), {
    os: 'linux', arch: 'x64', extension: '', asset: 'ani-tui-linux-x64',
  });
  assert.equal(releaseTarget('linux', 'arm').asset, 'ani-tui-linux-armv7');
  assert.equal(releaseTarget('darwin', 'arm64').asset, 'ani-tui-darwin-arm64');
  assert.equal(releaseTarget('win32', 'x64').asset, 'ani-tui-windows-x64.exe');
});

test('rejects unsupported platform and architecture combinations', () => {
  assert.throws(() => releaseTarget('freebsd', 'x64'), /does not publish/);
  assert.throws(() => releaseTarget('darwin', 'arm'), /does not publish/);
  assert.throws(() => releaseTarget('linux', 'ia32'), /does not publish/);
});
