#!/usr/bin/env node
'use strict';

const crypto = require('node:crypto');
const fs = require('node:fs');
const fsp = require('node:fs/promises');
const http = require('node:http');
const https = require('node:https');
const path = require('node:path');
const { Transform } = require('node:stream');
const { pipeline } = require('node:stream/promises');
const { releaseTarget } = require('../lib/platform');
const packageJSON = require('../package.json');

const MAX_REDIRECTS = 8;
const MAX_CHECKSUM_BYTES = 2 * 1024 * 1024;
const RETRY_DELAYS_MS = [0, 500, 1500];

function request(url, redirects = 0) {
  return new Promise((resolve, reject) => {
    const client = url.startsWith('https:') ? https : http;
    const req = client.get(url, {
      headers: {
        'User-Agent': `ani-tui-npm/${packageJSON.version}`,
        Accept: 'application/octet-stream',
      },
    }, (response) => {
      const status = response.statusCode || 0;
      if ([301, 302, 303, 307, 308].includes(status) && response.headers.location) {
        response.resume();
        if (redirects >= MAX_REDIRECTS) {
          reject(new Error(`too many redirects while downloading ${url}`));
          return;
        }
        const nextURL = new URL(response.headers.location, url).toString();
        resolve(request(nextURL, redirects + 1));
        return;
      }
      if (status < 200 || status >= 300) {
        response.resume();
        const error = new Error(`HTTP ${status} while downloading ${url}`);
        error.statusCode = status;
        reject(error);
        return;
      }
      resolve(response);
    });
    req.setTimeout(30_000, () => req.destroy(new Error(`download timed out: ${url}`)));
    req.on('error', reject);
  });
}

function isTransient(error) {
  if (error && [408, 429].includes(error.statusCode)) return true;
  if (error && error.statusCode >= 500) return true;
  return Boolean(error && [
    'ECONNRESET', 'ECONNREFUSED', 'EAI_AGAIN', 'ENETDOWN', 'ENETUNREACH',
    'EHOSTUNREACH', 'ETIMEDOUT',
  ].includes(error.code));
}

async function withRetries(operation) {
  let lastError;
  for (let attempt = 0; attempt < RETRY_DELAYS_MS.length; attempt += 1) {
    if (RETRY_DELAYS_MS[attempt]) {
      await new Promise((resolve) => setTimeout(resolve, RETRY_DELAYS_MS[attempt]));
    }
    try {
      return await operation();
    } catch (error) {
      lastError = error;
      if (!isTransient(error) || attempt === RETRY_DELAYS_MS.length - 1) throw error;
      console.warn(`ani-tui: transient download failure; retrying (${attempt + 2}/${RETRY_DELAYS_MS.length})…`);
    }
  }
  throw lastError;
}

async function downloadText(url) {
  return withRetries(async () => {
    const response = await request(url);
    const chunks = [];
    let length = 0;
    for await (const chunk of response) {
      length += chunk.length;
      if (length > MAX_CHECKSUM_BYTES) throw new Error('checksums.txt is unexpectedly large');
      chunks.push(chunk);
    }
    return Buffer.concat(chunks).toString('utf8');
  });
}

async function downloadBinary(url, destination) {
  return withRetries(async () => {
    const response = await request(url);
    const expectedBytes = Number(response.headers['content-length']) || 0;
    const hash = crypto.createHash('sha256');
    let downloadedBytes = 0;
    let lastPercent = -1;

    const observe = new Transform({
      transform(chunk, _encoding, callback) {
        hash.update(chunk);
        downloadedBytes += chunk.length;
        if (process.stderr.isTTY && expectedBytes > 0) {
          const percent = Math.floor((downloadedBytes / expectedBytes) * 100);
          if (percent !== lastPercent) {
            process.stderr.write(`\r  Downloading ${percent.toString().padStart(3)}%`);
            lastPercent = percent;
          }
        }
        callback(null, chunk);
      },
    });

    try {
      await pipeline(response, observe, fs.createWriteStream(destination, { mode: 0o700 }));
      if (process.stderr.isTTY && expectedBytes > 0) process.stderr.write('\r  Downloading 100%\n');
      return hash.digest('hex');
    } catch (error) {
      await fsp.rm(destination, { force: true });
      throw error;
    }
  });
}

function checksumFor(checksums, asset) {
  for (const line of checksums.split(/\r?\n/)) {
    const match = line.trim().match(/^([a-fA-F0-9]{64})\s+\*?(.+)$/);
    if (match && match[2] === asset) return match[1].toLowerCase();
  }
  throw new Error(`${asset} is missing from checksums.txt`);
}

async function install() {
  if (process.env.ANI_TUI_SKIP_DOWNLOAD === '1') {
    console.log('ani-tui: native download skipped by ANI_TUI_SKIP_DOWNLOAD');
    return;
  }

  const target = releaseTarget();
  const versionTag = packageJSON.version.startsWith('v') ? packageJSON.version : `v${packageJSON.version}`;
  const defaultBase = `https://github.com/Killersumit/ani-tui/releases/download/${versionTag}`;
  const base = (process.env.ANI_TUI_DOWNLOAD_BASE || defaultBase).replace(/\/$/, '');
  const nativeDir = process.env.ANI_TUI_NATIVE_DIR || path.join(__dirname, '..', 'native');
  const executable = path.join(nativeDir, `ani-tui${target.extension}`);
  const temporary = path.join(nativeDir, `.ani-tui-download-${process.pid}`);

  console.log(`ani-tui: installing ${target.os}/${target.arch} binary for ${versionTag}`);
  await fsp.mkdir(nativeDir, { recursive: true });
  await fsp.rm(temporary, { force: true });

  try {
    const checksums = await downloadText(`${base}/checksums.txt`);
    const expected = checksumFor(checksums, target.asset);
    const actual = await downloadBinary(`${base}/${target.asset}`, temporary);
    if (actual !== expected) {
      throw new Error(`SHA-256 mismatch for ${target.asset}; expected ${expected}, received ${actual}`);
    }
    await fsp.chmod(temporary, 0o755);
    await fsp.rm(executable, { force: true });
    await fsp.rename(temporary, executable);
    console.log(`ani-tui: verified and installed ${target.asset}`);
  } catch (error) {
    await fsp.rm(temporary, { force: true });
    throw error;
  }
}

if (require.main === module) {
  install().catch((error) => {
    console.error(`ani-tui: native installation failed: ${error.message}`);
    console.error('Check https://github.com/Killersumit/ani-tui/releases or retry with "npm rebuild -g ani-tui".');
    process.exit(1);
  });
}

module.exports = { checksumFor, install, isTransient, request };
