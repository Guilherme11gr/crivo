import { CheckResult, QualityGateResult } from './config';
import { COLORS, colorize, formatDuration } from './utils';

const STATUS_ICONS: Record<string, string> = {
  passed: '✅',
  failed: '❌',
  warning: '⚠️',
  skipped: '⏭️',
};

const STATUS_COLORS: Record<string, string> = {
  passed: COLORS.green,
  failed: COLORS.red,
  warning: COLORS.yellow,
  skipped: COLORS.dim,
};

// ─────────────────────────────────────────────
// Console Report (Terminal)
// ─────────────────────────────────────────────

export function printConsoleReport(result: QualityGateResult, verbose: boolean): void {
  const line = '─'.repeat(56);

  console.log('');
  console.log(colorize(`  ┌${line}┐`, COLORS.dim));

  const gateStatus = result.passed ? 'PASSED' : 'FAILED';
  const gateIcon = result.passed ? '✅' : '❌';
  const gateColor = result.passed ? COLORS.green : COLORS.red;
  const title = `QUALITY GATE: ${gateIcon} ${gateStatus}`;
  const padding = Math.max(0, 56 - title.length - 2);
  const left = Math.floor(padding / 2);
  const right = padding - left;

  console.log(colorize('  │', COLORS.dim) + ' '.repeat(left + 1) + colorize(title, gateColor, COLORS.bold) + ' '.repeat(right + 1) + colorize('│', COLORS.dim));
  console.log(colorize(`  ├${line}┤`, COLORS.dim));

  for (const check of result.checks) {
    printCheckLine(check);
  }

  console.log(colorize(`  ├${line}┤`, COLORS.dim));

  const duration = formatDuration(result.duration);
  const footerText = `Total: ${duration}`;
  const footerPad = 56 - footerText.length - 2;
  console.log(colorize('  │', COLORS.dim) + ' ' + colorize(footerText, COLORS.dim) + ' '.repeat(Math.max(0, footerPad) + 1) + colorize('│', COLORS.dim));
  console.log(colorize(`  └${line}┘`, COLORS.dim));
  console.log('');

  // Print details for failed/warning checks
  if (verbose) {
    for (const check of result.checks) {
      if (check.details && check.details.length > 0) {
        console.log(colorize(`  ${STATUS_ICONS[check.status]} ${check.name} Details:`, STATUS_COLORS[check.status], COLORS.bold));
        for (const detail of check.details) {
          console.log(colorize(`    ${detail}`, COLORS.dim));
        }
        console.log('');
      }
    }
  } else {
    const failedChecks = result.checks.filter(c => c.status === 'failed' || c.status === 'warning');
    if (failedChecks.length > 0) {
      for (const check of failedChecks) {
        if (check.details && check.details.length > 0) {
          console.log(colorize(`  ${STATUS_ICONS[check.status]} ${check.name}:`, STATUS_COLORS[check.status], COLORS.bold));
          for (const detail of check.details.slice(0, 5)) {
            console.log(colorize(`    ${detail}`, COLORS.dim));
          }
          if (check.details.length > 5) {
            console.log(colorize(`    ... and ${check.details.length - 5} more (use --verbose)`, COLORS.dim));
          }
          console.log('');
        }
      }
    }
  }
}

function printCheckLine(check: CheckResult): void {
  const icon = STATUS_ICONS[check.status];
  const color = STATUS_COLORS[check.status];
  const duration = formatDuration(check.duration);

  const name = check.name.padEnd(16);
  const summary = check.summary;
  const durationText = `(${duration})`;
  const content = `${icon} ${name} ${summary}`;
  const pad = Math.max(0, 56 - content.length - durationText.length - 2);

  console.log(
    colorize('  │', COLORS.dim) + ' ' +
    `${icon} ` +
    colorize(check.name.padEnd(16), color) +
    colorize(summary, COLORS.white) +
    ' '.repeat(pad) +
    colorize(durationText, COLORS.dim) + ' ' +
    colorize('│', COLORS.dim)
  );
}

// ─────────────────────────────────────────────
// Markdown Report (PR Comments)
// ─────────────────────────────────────────────

export function generateMarkdownReport(result: QualityGateResult): string {
  const gateStatus = result.passed ? '✅ Passed' : '❌ Failed';
  const lines: string[] = [];

  lines.push(`## 🔍 Quality Gate: ${gateStatus}`);
  lines.push('');
  lines.push('| Check | Status | Details | Time |');
  lines.push('|-------|--------|---------|------|');

  for (const check of result.checks) {
    const icon = STATUS_ICONS[check.status];
    const duration = formatDuration(check.duration);
    lines.push(`| ${check.name} | ${icon} | ${check.summary} | ${duration} |`);
  }

  lines.push('');

  // Details sections
  for (const check of result.checks) {
    if (check.details && check.details.length > 0 && check.status !== 'passed') {
      lines.push(`<details>`);
      lines.push(`<summary>${STATUS_ICONS[check.status]} ${check.name} — ${check.summary}</summary>`);
      lines.push('');
      lines.push('```');
      for (const detail of check.details.slice(0, 30)) {
        lines.push(detail);
      }
      if (check.details.length > 30) {
        lines.push(`... and ${check.details.length - 30} more`);
      }
      lines.push('```');
      lines.push('</details>');
      lines.push('');
    }
  }

  // Coverage breakdown if available
  const coverageCheck = result.checks.find(c => c.name === 'Coverage' && c.metrics);
  if (coverageCheck?.metrics) {
    lines.push('<details>');
    lines.push('<summary>📊 Coverage Breakdown</summary>');
    lines.push('');
    lines.push('| Metric | Value |');
    lines.push('|--------|-------|');
    lines.push(`| Lines | ${coverageCheck.metrics.lines}% |`);
    lines.push(`| Branches | ${coverageCheck.metrics.branches}% |`);
    lines.push(`| Functions | ${coverageCheck.metrics.functions}% |`);
    lines.push(`| Statements | ${coverageCheck.metrics.statements}% |`);
    lines.push('</details>');
    lines.push('');
  }

  // Duplication details
  const dupCheck = result.checks.find(c => c.name === 'Duplication' && c.metrics);
  if (dupCheck?.metrics && dupCheck.metrics.clones > 0) {
    lines.push('<details>');
    lines.push(`<summary>📋 Duplication: ${dupCheck.metrics.percentage.toFixed(1)}% (${dupCheck.metrics.clones} clone${dupCheck.metrics.clones !== 1 ? 's' : ''})</summary>`);
    lines.push('');
    if (dupCheck.details) {
      lines.push('```');
      for (const d of dupCheck.details) {
        lines.push(d);
      }
      lines.push('```');
    }
    lines.push('</details>');
    lines.push('');
  }

  lines.push(`---`);
  lines.push(`*Total time: ${formatDuration(result.duration)} · Generated by [quality-gate](https://github.com/quality-gate)*`);

  return lines.join('\n');
}
