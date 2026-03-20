import * as fs from 'fs';
import * as path from 'path';

export function findDependencyBin(name: string): string {
  try {
    const pkgJsonPath = require.resolve(`${name}/package.json`);
    const pkgJson = JSON.parse(fs.readFileSync(pkgJsonPath, 'utf-8'));
    const binEntry = typeof pkgJson.bin === 'string'
      ? pkgJson.bin
      : pkgJson.bin?.[name];

    if (binEntry) {
      return path.resolve(path.dirname(pkgJsonPath), binEntry);
    }
  } catch {
    // fall through
  }
  throw new Error(`Could not find binary for "${name}". Is it installed?`);
}

export function formatDuration(ms: number): string {
  if (ms < 1000) return `${ms}ms`;
  const seconds = (ms / 1000).toFixed(1);
  return `${seconds}s`;
}

export const PACKAGE_ROOT = path.resolve(__dirname, '..');

export const COLORS = {
  reset: '\x1b[0m',
  bold: '\x1b[1m',
  dim: '\x1b[2m',
  red: '\x1b[31m',
  green: '\x1b[32m',
  yellow: '\x1b[33m',
  blue: '\x1b[34m',
  cyan: '\x1b[36m',
  white: '\x1b[37m',
  bgRed: '\x1b[41m',
  bgGreen: '\x1b[42m',
  bgYellow: '\x1b[43m',
};

export function colorize(text: string, ...codes: string[]): string {
  if (process.env.NO_COLOR || process.env.CI) return text;
  return `${codes.join('')}${text}${COLORS.reset}`;
}
