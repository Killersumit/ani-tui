'use strict';

const test = require('node:test');
const assert = require('node:assert/strict');
const crypto = require('node:crypto');
const fsp = require('node:fs/promises');
const http = require('node:http');
const os = require('node:os');
const path = require('node:path');
const { execFile } = require('node:child_process');
const { promisify } = require('node:util');
const { checksumFor, isTransient } = require('../scripts/install');
const { releaseTarget } = require('../lib/platform');

const execFileAsync = promisify(execFile);

test('selects exact checksum entries', () => {
  const hash = 'a'.repeat(64);
  assert.equal(checksumFor(`${hash}  ani-tui-linux-x64\n`, 'ani-tui-linux-x64'), hash);
  assert.throws(() => checksumFor(`${hash}  other-file\n`, 'ani-tui-linux-x64'), /missing/);
});

test('retries only explicit transient failures', () => {
  assert.equal(isTransient({ statusCode: 429 }), true);
  assert.equal(isTransient({ statusCode: 503 }), true);
  assert.equal(isTransient({ statusCode: 404 }), false);
  assert.equal(isTransient({ code: 'ETIMEDOUT' }), true);
  assert.equal(isTransient(new Error('failed to download')), false);
});

test('global launcher forwards arguments to the native executable', async () => {
  const launcher = path.join(__dirname, '..', 'bin', 'ani-tui.js');
  const expression = 'process.stdout.write(process.argv[1])';
  const result = await execFileAsync(process.execPath, [launcher, '-e', expression, 'forwarded argument'], {
    env: { ...process.env, ANI_TUI_BINARY: process.execPath },
  });
  assert.equal(result.stdout, 'forwarded argument');
});

test('postinstall downloads and verifies the native artifact', async (t) => {
  const target = releaseTarget();
  const binary = Buffer.from('native ani-tui fixture\n');
  const digest = crypto.createHash('sha256').update(binary).digest('hex');
  const server = http.createServer((request, response) => {
    if (request.url === '/checksums.txt') {
      response.end(`${digest}  ${target.asset}\n`);
      return;
    }
    if (request.url === `/${target.asset}`) {
      response.setHeader('content-length', binary.length);
      response.end(binary);
      return;
    }
    response.statusCode = 404;
    response.end('not found');
  });
  await new Promise((resolve) => server.listen(0, '127.0.0.1', resolve));
  t.after(() => server.close());

  const temporary = await fsp.mkdtemp(path.join(os.tmpdir(), 'ani-tui-npm-test-'));
  t.after(() => fsp.rm(temporary, { recursive: true, force: true }));
  const port = server.address().port;
  const script = path.join(__dirname, '..', 'scripts', 'install.js');
  const result = await execFileAsync(process.execPath, [script], {
    env: {
      ...process.env,
      ANI_TUI_DOWNLOAD_BASE: `http://127.0.0.1:${port}`,
      ANI_TUI_NATIVE_DIR: temporary,
    },
  });

  assert.match(result.stdout, /verified and installed/);
  const installed = await fsp.readFile(path.join(temporary, `ani-tui${target.extension}`));
  assert.deepEqual(installed, binary);
});

test('postinstall refuses a checksum mismatch without leaving a binary', async (t) => {
  const target = releaseTarget();
  const binary = Buffer.from('tampered fixture\n');
  const server = http.createServer((request, response) => {
    if (request.url === '/checksums.txt') {
      response.end(`${'0'.repeat(64)}  ${target.asset}\n`);
    } else if (request.url === `/${target.asset}`) {
      response.end(binary);
    } else {
      response.statusCode = 404;
      response.end('not found');
    }
  });
  await new Promise((resolve) => server.listen(0, '127.0.0.1', resolve));
  t.after(() => server.close());

  const temporary = await fsp.mkdtemp(path.join(os.tmpdir(), 'ani-tui-npm-bad-test-'));
  t.after(() => fsp.rm(temporary, { recursive: true, force: true }));
  const script = path.join(__dirname, '..', 'scripts', 'install.js');
  await assert.rejects(
    execFileAsync(process.execPath, [script], {
      env: {
        ...process.env,
        ANI_TUI_DOWNLOAD_BASE: `http://127.0.0.1:${server.address().port}`,
        ANI_TUI_NATIVE_DIR: temporary,
      },
    }),
    (error) => {
      assert.match(error.stderr, /SHA-256 mismatch/);
      return true;
    },
  );
  await assert.rejects(
    fsp.stat(path.join(temporary, `ani-tui${target.extension}`)),
    { code: 'ENOENT' },
  );
});
