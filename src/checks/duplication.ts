import { execSync } from 'child_process';
import * as fs from 'fs';
import * as path from 'path';
import { CheckResult, QualityGateConfig } from '../config';
import { findDependencyBin } from '../utils';

export async function checkDuplication(projectDir: string, config: QualityGateConfig): Promise<CheckResult> {
  const start = Date.now();
  const reportDir = path.join(projectDir, '.qualitygate-temp');

  try {
    fs.mkdirSync(reportDir, { recursive: true });

    const srcDir = path.join(projectDir, config.srcPattern);
    if (!fs.existsSync(srcDir)) {
      return {
        name: 'Duplication',
        status: 'skipped',
        summary: `Source directory "${config.srcPattern}" not found`,
        duration: Date.now() - start,
      };
    }

    let jscpdBin: string;
    try {
      jscpdBin = findDependencyBin('jscpd');
    } catch {
      return {
        name: 'Duplication',
        status: 'skipped',
        summary: 'jscpd not found',
        duration: Date.now() - start,
      };
    }

    const ignoreArgs = config.exclude
      .map(e => `--ignore "${e}"`)
      .join(' ');

    const cmd = [
      `"${process.execPath}"`,
      `"${jscpdBin}"`,
      `"${srcDir}"`,
      `--min-lines ${config.duplication.minLines}`,
      `--min-tokens ${config.duplication.minTokens}`,
      `--reporters json`,
      `--output "${reportDir}"`,
      ignoreArgs,
      '--silent',
    ].join(' ');

    try {
      execSync(cmd, {
        cwd: projectDir,
        stdio: 'pipe',
        timeout: 120_000,
      });
    } catch {
      // jscpd exits non-zero when duplicates are found — that's expected
    }

    const reportPath = path.join(reportDir, 'jscpd-report.json');
    if (!fs.existsSync(reportPath)) {
      return {
        name: 'Duplication',
        status: 'passed',
        summary: '0% duplication',
        metrics: { percentage: 0, clones: 0 },
        duration: Date.now() - start,
      };
    }

    const report = JSON.parse(fs.readFileSync(reportPath, 'utf-8'));
    const pct = report.statistics?.total?.percentage ?? 0;
    const clones = report.duplicates ?? [];

    const status = pct > config.duplication.threshold
      ? 'failed'
      : pct > config.duplication.threshold * 0.8
        ? 'warning'
        : 'passed';

    const details = clones.slice(0, 10).map((d: Record<string, unknown>) => {
      const first = (d.firstFile as Record<string, string>)?.name ?? '';
      const second = (d.secondFile as Record<string, string>)?.name ?? '';
      const lines = d.lines ?? '?';
      return `${path.relative(projectDir, first)} <-> ${path.relative(projectDir, second)} (${lines} lines)`;
    });

    return {
      name: 'Duplication',
      status,
      summary: `${pct.toFixed(1)}% (max: ${config.duplication.threshold}%)`,
      details: details.length > 0 ? details : undefined,
      metrics: { percentage: pct, clones: clones.length },
      duration: Date.now() - start,
    };
  } finally {
    if (fs.existsSync(reportDir)) {
      fs.rmSync(reportDir, { recursive: true, force: true });
    }
  }
}
