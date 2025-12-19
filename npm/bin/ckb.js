#!/usr/bin/env node

const { execFileSync } = require('child_process');
const os = require('os');
const path = require('path');

const PLATFORMS = {
  'darwin-arm64': '@tastehub/ckb-darwin-arm64',
  'darwin-x64': '@tastehub/ckb-darwin-x64',
  'linux-x64': '@tastehub/ckb-linux-x64',
  'linux-arm64': '@tastehub/ckb-linux-arm64',
  'win32-x64': '@tastehub/ckb-win32-x64',
};

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

try {
  const binPath = getBinaryPath();
  execFileSync(binPath, process.argv.slice(2), { stdio: 'inherit' });
} catch (e) {
  if (e.status !== undefined) {
    process.exit(e.status);
  }
  console.error(`Failed to run CKB: ${e.message}`);
  process.exit(1);
}
