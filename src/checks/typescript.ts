import { execSync } from 'child_process';
import * as fs from 'fs';
import * as path from 'path';
import { CheckResult, QualityGateConfig } from '../config';

export async function checkTypescript(projectDir: string, _config: QualityGateConfig): Promise<CheckResult> {
  const start = Date.now();

  if (!fs.existsSync(path.join(projectDir, 'tsconfig.json'))) {
    return {
      name: 'Type Safety',
      status: 'skipped',
      summary: 'No tsconfig.json found',
      duration: Date.now() - start,
    };
  }

  try {
    execSync('npx tsc --noEmit', {
      cwd: projectDir,
      stdio: 'pipe',
      timeout: 120_000,
    });

    return {
      name: 'Type Safety',
      status: 'passed',
      summary: '0 errors',
      duration: Date.now() - start,
    };
  } catch (error: unknown) {
    const err = error as { stdout?: Buffer; stderr?: Buffer };
    const output = err.stdout?.toString() || err.stderr?.toString() || '';
    const errorLines = output.split('\n').filter((l: string) => l.includes('error TS'));
    const errorCount = errorLines.length || 1;

    return {
      name: 'Type Safety',
      status: 'failed',
      summary: `${errorCount} error${errorCount !== 1 ? 's' : ''}`,
      details: errorLines.slice(0, 20),
      duration: Date.now() - start,
    };
  }
}
