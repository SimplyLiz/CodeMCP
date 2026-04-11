#!/usr/bin/env node
/**
 * CCE Bridge — thin stdin/stdout wrapper around context-compression-engine.
 *
 * Input  (stdin):  JSON { messages: Message[], tokenBudget?: number }
 * Output (stdout): JSON { messages: Message[], verbatim: VerbatimMap,
 *                         tokenCount?: number, withinBudget?: boolean }
 *
 * CCE dist location (in priority order):
 *   1. --cce-dist <path>  CLI flag
 *   2. CCE_DIST           environment variable
 */

import { readFileSync } from 'fs';
import { pathToFileURL } from 'url';
import { resolve } from 'path';

// ---------------------------------------------------------------------------
// Resolve CCE dist path
// ---------------------------------------------------------------------------

const args = process.argv.slice(2);
let cceDist = process.env.CCE_DIST ?? '';

for (let i = 0; i < args.length; i++) {
  if (args[i] === '--cce-dist' && args[i + 1]) {
    cceDist = args[++i];
  }
}

if (!cceDist) {
  process.stderr.write(
    'cce_bridge: CCE dist path required.\n' +
      'Set CCE_DIST env var or pass --cce-dist <path>\n',
  );
  process.exit(1);
}

// ---------------------------------------------------------------------------
// Read stdin
// ---------------------------------------------------------------------------

let input;
try {
  const raw = readFileSync(0, 'utf-8'); // fd 0 = stdin
  input = JSON.parse(raw);
} catch (e) {
  process.stderr.write(`cce_bridge: failed to read/parse stdin JSON: ${e.message}\n`);
  process.exit(1);
}

const { messages, tokenBudget } = input;
if (!Array.isArray(messages)) {
  process.stderr.write('cce_bridge: input.messages must be an array\n');
  process.exit(1);
}

// ---------------------------------------------------------------------------
// Dynamic import of CCE
// ---------------------------------------------------------------------------

const indexPath = resolve(cceDist, 'index.js');
const indexUrl = pathToFileURL(indexPath).href;

let compress;
try {
  ({ compress } = await import(indexUrl));
} catch (e) {
  process.stderr.write(`cce_bridge: failed to import CCE from ${indexUrl}: ${e.message}\n`);
  process.exit(1);
}

// ---------------------------------------------------------------------------
// Normalise messages — CCE requires id (string) and index (number)
// ---------------------------------------------------------------------------

const normalized = messages.map((m, i) => ({
  id: m.id ?? `msg_${i}`,
  index: m.index ?? i,
  ...m,
}));

// ---------------------------------------------------------------------------
// Compress
// ---------------------------------------------------------------------------

const opts = {};
if (typeof tokenBudget === 'number' && tokenBudget > 0) {
  opts.tokenBudget = tokenBudget;
}

try {
  const result = compress(normalized, opts);
  process.stdout.write(
    JSON.stringify({
      messages: result.messages,
      verbatim: result.verbatim,
      tokenCount: result.tokenCount ?? null,
      withinBudget: result.withinBudget ?? null,
    }) + '\n',
  );
} catch (e) {
  process.stderr.write(`cce_bridge: compression failed: ${e.message}\n`);
  process.exit(1);
}
