#!/usr/bin/env node
/**
 * CKB Complexity Check for lint-staged
 *
 * Usage in package.json:
 * {
 *   "lint-staged": {
 *     "*.{ts,tsx,js,jsx}": ["node scripts/ckb-check.js"]
 *   }
 * }
 *
 * Installation:
 *   cp examples/hooks/ckb-check.js scripts/ckb-check.js
 *   chmod +x scripts/ckb-check.js
 */

const { execSync } = require('child_process');

// Configuration
const MAX_CYCLOMATIC = parseInt(process.env.CKB_MAX_CYCLOMATIC || '15', 10);
const MAX_COGNITIVE = parseInt(process.env.CKB_MAX_COGNITIVE || '20', 10);

// Get staged files from arguments
const files = process.argv.slice(2);

if (files.length === 0) {
  process.exit(0);
}

// Check if CKB is available
try {
  execSync('ckb --version', { stdio: 'pipe' });
} catch (e) {
  console.log('CKB not installed, skipping complexity check');
  process.exit(0);
}

let hasError = false;
let hasWarning = false;

for (const file of files) {
  try {
    const result = execSync(`ckb complexity "${file}" --format=json`, {
      encoding: 'utf8',
      stdio: ['pipe', 'pipe', 'pipe']
    });

    const data = JSON.parse(result);
    const maxCyclomatic = data.summary?.maxCyclomatic || 0;
    const maxCognitive = data.summary?.maxCognitive || 0;

    if (maxCyclomatic > MAX_CYCLOMATIC) {
      console.error(`❌ ${file}: cyclomatic complexity ${maxCyclomatic} exceeds ${MAX_CYCLOMATIC}`);
      hasError = true;
    } else if (maxCyclomatic > MAX_CYCLOMATIC - 3) {
      console.warn(`⚠️  ${file}: cyclomatic complexity ${maxCyclomatic} approaching limit`);
      hasWarning = true;
    }

    if (maxCognitive > MAX_COGNITIVE) {
      console.error(`❌ ${file}: cognitive complexity ${maxCognitive} exceeds ${MAX_COGNITIVE}`);
      hasError = true;
    } else if (maxCognitive > MAX_COGNITIVE - 5) {
      console.warn(`⚠️  ${file}: cognitive complexity ${maxCognitive} approaching limit`);
      hasWarning = true;
    }
  } catch (e) {
    // File not analyzable or CKB error, skip silently
  }
}

if (hasError) {
  console.error('');
  console.error('Refactor complex functions before committing.');
  console.error('Bypass with: git commit --no-verify');
  process.exit(1);
}

if (hasWarning) {
  console.log('');
  console.log('Consider refactoring functions approaching complexity limits.');
}

process.exit(0);
