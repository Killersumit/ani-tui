#!/usr/bin/env node
'use strict';

const fs = require('node:fs');
const path = require('node:path');
const { spawnSync } = require('node:child_process');
const { releaseTarget } = require('../lib/platform');

let target;
try {
  target = releaseTarget();
} catch (error) {
  console.error(`ani-tui: ${error.message}`);
  process.exit(1);
}

const binary = process.env.ANI_TUI_BINARY || path.join(__dirname, '..', 'native', `ani-tui${target.extension}`);
if (!fs.existsSync(binary)) {
  console.error('ani-tui: the native executable is missing.');
  console.error('Run "npm rebuild -g ani-tui" while online, or reinstall the package.');
  console.error('Release assets: https://github.com/Killersumit/ani-tui/releases');
  process.exit(1);
}

const result = spawnSync(binary, process.argv.slice(2), { stdio: 'inherit' });
if (result.error) {
  console.error(`ani-tui: could not start ${binary}: ${result.error.message}`);
  process.exit(1);
}
if (result.signal) {
  const signalExitCodes = { SIGHUP: 129, SIGINT: 130, SIGQUIT: 131, SIGTERM: 143 };
  process.exit(signalExitCodes[result.signal] || 1);
}
process.exit(result.status === null ? 1 : result.status);
