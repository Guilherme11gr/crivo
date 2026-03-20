import * as fs from 'fs';
import * as path from 'path';
import { loadConfig, QualityGateResult, CheckResult, RunOptions } from './config';
import { checkTypescript } from './checks/typescript';
import { checkCodeQuality, checkSecurity } from './checks/lint';
import { checkCoverage } from './checks/coverage';
import { checkDuplication } from './checks/duplication';
import { printConsoleReport, generateMarkdownReport } from './report';
import { colorize, COLORS } from './utils';

export async function runQualityGate(projectDir: string, options: RunOptions): Promise<QualityGateResult> {
  const config = loadConfig(projectDir);
  const checks: CheckResult[] = [];
  const start = Date.now();

  const configPath = path.join(projectDir, '.qualitygate.json');
  const configSource = fs.existsSync(configPath) ? configPath : 'defaults';

  if (!options.ci) {
    console.log('');
    console.log(colorize('  🔍 Quality Gate', COLORS.bold, COLORS.cyan));
    console.log(colorize(`  Config: ${configSource}`, COLORS.dim));
    console.log('');
  }

  // 1. Type Safety
  if (config.checks.typescript) {
    if (!options.ci) process.stdout.write(colorize('  ⏳ Type Safety...', COLORS.dim));
    const result = await checkTypescript(projectDir, config);
    checks.push(result);
    if (!options.ci) clearAndPrint(result);
  }

  // 2. Code Quality (SonarJS + Structural)
  if (config.checks.lint) {
    if (!options.ci) process.stdout.write(colorize('  ⏳ Code Quality...', COLORS.dim));
    const result = await checkCodeQuality(projectDir, config);
    checks.push(result);
    if (!options.ci) clearAndPrint(result);

    // 3. Security (separate category)
    if (!options.ci) process.stdout.write(colorize('  ⏳ Security...', COLORS.dim));
    const secResult = await checkSecurity(projectDir, config);
    checks.push(secResult);
    if (!options.ci) clearAndPrint(secResult);
  }

  // 4. Coverage
  if (config.checks.coverage) {
    if (!options.ci) process.stdout.write(colorize('  ⏳ Tests + Coverage...', COLORS.dim));
    const result = await checkCoverage(projectDir, config);
    checks.push(result);
    if (!options.ci) clearAndPrint(result);
  }

  // 5. Duplication
  if (config.checks.duplication) {
    if (!options.ci) process.stdout.write(colorize('  ⏳ Duplication...', COLORS.dim));
    const result = await checkDuplication(projectDir, config);
    checks.push(result);
    if (!options.ci) clearAndPrint(result);
  }

  const passed = checks.every(c => c.status !== 'failed');
  const result: QualityGateResult = {
    passed,
    checks,
    duration: Date.now() - start,
  };

  // Print report
  if (!options.ci) {
    printConsoleReport(result, options.verbose);
  }

  // Generate markdown if requested
  if (options.output) {
    const markdown = generateMarkdownReport(result);
    const outputPath = path.isAbsolute(options.output)
      ? options.output
      : path.join(projectDir, options.output);
    fs.writeFileSync(outputPath, markdown, 'utf-8');

    if (!options.ci) {
      console.log(colorize(`  📄 Report saved to ${options.output}`, COLORS.dim));
      console.log('');
    }
  }

  // In CI mode, always output markdown to stdout if no file specified
  if (options.ci && !options.output) {
    console.log(generateMarkdownReport(result));
  }

  return result;
}

function clearAndPrint(check: CheckResult): void {
  // Clear the "running" line and print result
  process.stdout.write('\r\x1b[K');
  const icons: Record<string, string> = {
    passed: '✅',
    failed: '❌',
    warning: '⚠️',
    skipped: '⏭️',
  };
  console.log(`  ${icons[check.status]} ${check.name}: ${check.summary}`);
}
