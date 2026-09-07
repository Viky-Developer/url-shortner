#!/usr/bin/env bash
set -euo pipefail

node --input-type=module - "$@" <<'EOF'
import { readFileSync } from 'node:fs';

const messageFile = process.argv[2];

if (!messageFile) {
	console.error('Commit message file is required.');
	process.exit(1);
}

const subject = readFileSync(messageFile, 'utf8').split(/\r?\n/, 1)[0].trim();
const type = '(?:feat|fix|docs|style|refactor|perf|test|build|ci|chore|revert)';
const pattern = new RegExp(`^${type}\\(#[1-9]\\d*\\): \\p{Extended_Pictographic}\\uFE0F? .+`, 'u');

if (!pattern.test(subject)) {
	console.error('Invalid commit message.');
	console.error('Use: type(#issue): emoji message');
	console.error('Example: feat(#19): ✨ add link analytics');
	process.exit(1);
}
EOF