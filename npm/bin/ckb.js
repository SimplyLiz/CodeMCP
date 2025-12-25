#!/usr/bin/env node

const { execFileSync } = require('child_process');
const fs = require('fs');
const os = require('os');
const path = require('path');

const PLATFORMS = {
  'darwin-arm64': '@tastehub/ckb-darwin-arm64',
  'darwin-x64': '@tastehub/ckb-darwin-x64',
  'linux-x64': '@tastehub/ckb-linux-x64',
  'linux-arm64': '@tastehub/ckb-linux-arm64',
  'win32-x64': '@tastehub/ckb-win32-x64',
};

/**
 * Find repo root by walking up from the correct starting directory.
 *
 * Key insight: When running via npx, process.cwd() is a temp directory
 * (~/.npm/_npx/...), NOT the user's project. npm sets INIT_CWD to the
 * directory where the user ran npx from.
 *
 * Priority:
 * 1. CKB_REPO env var (explicit override)
 * 2. --repo CLI argument (explicit override)
 * 3. INIT_CWD (where user ran npx from)
 * 4. process.cwd() (fallback)
 */
function findRepoRoot() {
  // If CKB_REPO is already set, respect it
  if (process.env.CKB_REPO) {
    return process.env.CKB_REPO;
  }

  // Check if --repo was passed as argument
  const repoArgIndex = process.argv.indexOf('--repo');
  if (repoArgIndex !== -1 && process.argv[repoArgIndex + 1]) {
    return process.argv[repoArgIndex + 1];
  }

  // INIT_CWD is set by npm to the directory where npx was invoked
  // This is the key fix for the npx sandbox problem
  const startDir = process.env.INIT_CWD || process.cwd();

  let dir = startDir;
  const root = path.parse(dir).root;

  while (dir !== root) {
    // Prefer .ckb/ (explicit CKB project) over .git/ (any git repo)
    if (fs.existsSync(path.join(dir, '.ckb'))) {
      return dir;
    }
    if (fs.existsSync(path.join(dir, '.git'))) {
      return dir;
    }
    dir = path.dirname(dir);
  }

  // No repo found - let the Go binary handle it
  return null;
}

function getBinaryPath() {
  const platform = `${os.platform()}-${os.arch()}`;
  const pkg = PLATFORMS[platform];

  if (!pkg) {
    console.error(`Unsupported platform: ${platform}`);
    console.error('Supported platforms: darwin-arm64, darwin-x64, linux-x64, linux-arm64, win32-x64');
    process.exit(1);
  }

  try {
    // Try to resolve the platform-specific package
    const pkgPath = require.resolve(`${pkg}/package.json`);
    const pkgDir = path.dirname(pkgPath);

    // Binary name varies by platform
    const binName = os.platform() === 'win32' ? 'ckb.exe' : 'ckb';
    return path.join(pkgDir, 'bin', binName);
  } catch (e) {
    console.error(`Failed to find CKB binary for ${platform}`);
    console.error(`Package ${pkg} may not be installed.`);
    console.error('');
    console.error('Try reinstalling:');
    console.error('  npm install -g @tastehub/ckb');
    process.exit(1);
  }
}

// Debug logging when CKB_DEBUG is set
function debug(msg) {
  if (process.env.CKB_DEBUG) {
    console.error(`[ckb-wrapper] ${msg}`);
  }
}

try {
  const binPath = getBinaryPath();
  const repoRoot = findRepoRoot();

  debug(`INIT_CWD: ${process.env.INIT_CWD || '(not set)'}`);
  debug(`process.cwd(): ${process.cwd()}`);
  debug(`Resolved repo root: ${repoRoot || '(not found)'}`);
  debug(`Binary path: ${binPath}`);

  // Pass repo root via environment variable
  const env = { ...process.env };
  if (repoRoot) {
    env.CKB_REPO = repoRoot;
  }

  execFileSync(binPath, process.argv.slice(2), {
    stdio: 'inherit',
    env
  });
} catch (e) {
  if (e.status !== undefined) {
    process.exit(e.status);
  }

  // Provide helpful error message for common issues
  const msg = e.message || '';
  if (msg.includes('ENOENT') || msg.includes('not found')) {
    console.error('CKB Error: Binary not found');
    console.error('');
    console.error('This usually means the platform-specific package failed to install.');
    console.error('Try reinstalling: npm install -g @tastehub/ckb');
    console.error('');
    console.error('For debugging, run with CKB_DEBUG=1');
  } else {
    console.error(`Failed to run CKB: ${msg}`);
  }
  process.exit(1);
}
