import { execSync } from 'child_process';
import * as fs from 'fs';
import * as path from 'path';
import { CheckResult, QualityGateConfig } from '../config';

export async function checkCoverage(projectDir: string, config: QualityGateConfig): Promise<CheckResult> {
  const start = Date.now();

  // Auto-detect: skip if no jest config or test script
  const pkgPath = path.join(projectDir, 'package.json');
  if (fs.existsSync(pkgPath)) {
    const pkg = JSON.parse(fs.readFileSync(pkgPath, 'utf-8'));
    if (!pkg.scripts?.test && !pkg.devDependencies?.jest && !pkg.dependencies?.jest) {
      return {
        name: 'Coverage',
        status: 'skipped',
        summary: 'No test runner detected',
        duration: Date.now() - start,
      };
    }
  }

  try {
    execSync('npx jest --coverage --coverageReporters=json-summary --passWithNoTests --silent', {
      cwd: projectDir,
      stdio: 'pipe',
      timeout: 300_000,
      env: { ...process.env, CI: 'true', NODE_ENV: 'test' },
    });
  } catch (error: unknown) {
    const err = error as { status?: number; stdout?: Buffer; stderr?: Buffer };
    const output = err.stdout?.toString() || err.stderr?.toString() || '';

    // Check if tests ran but some failed (coverage might still be generated)
    const coveragePath = path.join(projectDir, 'coverage', 'coverage-summary.json');
    if (!fs.existsSync(coveragePath)) {
      const failCount = (output.match(/Tests:\s+(\d+) failed/)?.[1]) || '?';
      return {
        name: 'Coverage',
        status: 'failed',
        summary: `${failCount} test${failCount !== '1' ? 's' : ''} failed`,
        details: output.split('\n').filter(l => l.includes('FAIL') || l.includes('●')).slice(0, 15),
        duration: Date.now() - start,
      };
    }
    // Tests failed but coverage was generated — continue to analyze coverage
  }

  const coveragePath = path.join(projectDir, 'coverage', 'coverage-summary.json');
  if (!fs.existsSync(coveragePath)) {
    return {
      name: 'Coverage',
      status: 'warning',
      summary: 'No coverage data generated',
      duration: Date.now() - start,
    };
  }

  const coverage = JSON.parse(fs.readFileSync(coveragePath, 'utf-8'));
  const total = coverage.total;

  const metrics: Record<string, number> = {
    lines: total.lines.pct,
    branches: total.branches.pct,
    functions: total.functions.pct,
    statements: total.statements.pct,
  };

  const failures: string[] = [];
  if (metrics.lines < config.coverage.lines)
    failures.push(`Lines: ${metrics.lines}% < ${config.coverage.lines}%`);
  if (metrics.branches < config.coverage.branches)
    failures.push(`Branches: ${metrics.branches}% < ${config.coverage.branches}%`);
  if (metrics.functions < config.coverage.functions)
    failures.push(`Functions: ${metrics.functions}% < ${config.coverage.functions}%`);
  if (metrics.statements < config.coverage.statements)
    failures.push(`Statements: ${metrics.statements}% < ${config.coverage.statements}%`);

  const status = failures.length > 0 ? 'failed' : 'passed';

  return {
    name: 'Coverage',
    status,
    summary: `${metrics.lines}% lines · ${metrics.branches}% branches (min: ${config.coverage.lines}%/${config.coverage.branches}%)`,
    details: [
      `Lines:      ${metrics.lines}% (min: ${config.coverage.lines}%)`,
      `Branches:   ${metrics.branches}% (min: ${config.coverage.branches}%)`,
      `Functions:  ${metrics.functions}% (min: ${config.coverage.functions}%)`,
      `Statements: ${metrics.statements}% (min: ${config.coverage.statements}%)`,
      ...(failures.length > 0 ? ['', ...failures.map(f => `FAILED: ${f}`)] : []),
    ],
    metrics,
    duration: Date.now() - start,
  };
}
