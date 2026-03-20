import * as path from 'path';
import { CheckResult, QualityGateConfig } from '../config';
import { PACKAGE_ROOT } from '../utils';

interface LintMessage {
  ruleId: string | null;
  severity: number;
  message: string;
  line: number;
  column: number;
}

interface LintResult {
  filePath: string;
  messages: LintMessage[];
}

function buildEslintConfig(config: QualityGateConfig) {
  return {
    parser: require.resolve('@typescript-eslint/parser'),
    parserOptions: {
      ecmaVersion: 2022,
      sourceType: 'module' as const,
      ecmaFeatures: { jsx: true },
    },
    plugins: ['sonarjs', 'security'],
    rules: {
      // --- SonarJS: Bug Detection ---
      'sonarjs/no-all-duplicated-branches': 'error',
      'sonarjs/no-element-overwrite': 'error',
      'sonarjs/no-identical-conditions': 'error',
      'sonarjs/no-identical-expressions': 'error',
      'sonarjs/no-one-iteration-loop': 'error',
      'sonarjs/no-use-of-empty-return-value': 'error',

      // --- SonarJS: Code Smells ---
      'sonarjs/cognitive-complexity': ['error', config.complexity.cognitive],
      'sonarjs/no-collapsible-if': 'warn',
      'sonarjs/no-duplicate-string': ['warn', { threshold: 5 }],
      'sonarjs/no-duplicated-branches': 'error',
      'sonarjs/no-identical-functions': 'error',
      'sonarjs/no-redundant-boolean': 'warn',
      'sonarjs/no-redundant-jump': 'warn',
      'sonarjs/no-small-switch': 'warn',
      'sonarjs/no-unused-collection': 'error',
      'sonarjs/prefer-immediate-return': 'warn',
      'sonarjs/prefer-single-boolean-return': 'warn',
      'sonarjs/prefer-while': 'warn',

      // --- Structural Limits ---
      'complexity': ['warn', config.complexity.cyclomatic],
      'max-lines-per-function': ['warn', {
        max: config.structural.maxLinesPerFunction,
        skipBlankLines: true,
        skipComments: true,
      }],
      'max-params': ['warn', config.structural.maxParams],
      'max-depth': ['warn', config.structural.maxDepth],
      'max-nested-callbacks': ['warn', config.structural.maxNestedCallbacks],
    } as Record<string, unknown>,
    ignorePatterns: [
      ...config.exclude,
      ...config.testExclude,
    ],
  };
}

function buildSecurityConfig(config: QualityGateConfig) {
  return {
    parser: require.resolve('@typescript-eslint/parser'),
    parserOptions: {
      ecmaVersion: 2022,
      sourceType: 'module' as const,
      ecmaFeatures: { jsx: true },
    },
    plugins: ['security'],
    rules: {
      'security/detect-eval-with-expression': 'error',
      'security/detect-non-literal-fs-filename': 'warn',
      'security/detect-non-literal-require': 'warn',
      'security/detect-possible-timing-attacks': 'warn',
      'security/detect-child-process': 'warn',
      'security/detect-no-csrf-before-method-override': 'error',
      'security/detect-non-literal-regexp': 'warn',
    } as Record<string, unknown>,
    ignorePatterns: [
      ...config.exclude,
      ...config.testExclude,
    ],
  };
}

async function runEslint(
  projectDir: string,
  overrideConfig: ReturnType<typeof buildEslintConfig>,
  srcPattern: string,
): Promise<{ errors: number; warnings: number; details: string[] }> {
  // eslint-disable-next-line @typescript-eslint/no-var-requires
  const { ESLint } = require('eslint');

  const eslint = new ESLint({
    cwd: projectDir,
    useEslintrc: false,
    overrideConfig,
    resolvePluginsRelativeTo: PACKAGE_ROOT,
    extensions: ['.ts', '.tsx', '.js', '.jsx'],
  });

  const results: LintResult[] = await eslint.lintFiles([srcPattern]);

  let errors = 0;
  let warnings = 0;
  const details: string[] = [];

  for (const result of results) {
    for (const msg of result.messages) {
      // Skip "Definition for rule X was not found" — these are from the project's
      // eslintrc leaking rules we don't load (react-hooks, @next/next, etc.)
      if (msg.message?.includes('Definition for rule') && msg.message?.includes('was not found')) {
        continue;
      }

      const rel = path.relative(projectDir, result.filePath);
      const severity = msg.severity === 2 ? 'error' : 'warning';
      const line = `${rel}:${msg.line}:${msg.column} ${severity} ${msg.message} (${msg.ruleId})`;

      if (msg.severity === 2) {
        errors++;
        details.push(line);
      } else {
        warnings++;
        if (details.length < 40) details.push(line);
      }
    }
  }

  return { errors, warnings, details };
}

export async function checkCodeQuality(projectDir: string, config: QualityGateConfig): Promise<CheckResult> {
  const start = Date.now();

  try {
    const eslintConfig = buildEslintConfig(config);
    const { errors, warnings, details } = await runEslint(projectDir, eslintConfig, config.srcPattern);

    const status = errors > 0 ? 'failed' : warnings > 0 ? 'warning' : 'passed';
    const parts: string[] = [];
    if (errors > 0) parts.push(`${errors} error${errors !== 1 ? 's' : ''}`);
    if (warnings > 0) parts.push(`${warnings} warning${warnings !== 1 ? 's' : ''}`);
    if (parts.length === 0) parts.push('Clean');

    return {
      name: 'Code Quality',
      status,
      summary: parts.join(' · '),
      details: details.length > 0 ? details : undefined,
      metrics: { errors, warnings },
      duration: Date.now() - start,
    };
  } catch (error: unknown) {
    const msg = error instanceof Error ? error.message : String(error);
    return {
      name: 'Code Quality',
      status: 'failed',
      summary: `ESLint error: ${msg.slice(0, 100)}`,
      details: [msg],
      duration: Date.now() - start,
    };
  }
}

export async function checkSecurity(projectDir: string, config: QualityGateConfig): Promise<CheckResult> {
  const start = Date.now();

  try {
    const eslintConfig = buildSecurityConfig(config);
    const { errors, warnings, details } = await runEslint(projectDir, eslintConfig, config.srcPattern);

    const total = errors + warnings;
    const status = errors > 0 ? 'failed' : warnings > 0 ? 'warning' : 'passed';
    const summary = total === 0
      ? '0 vulnerabilities'
      : `${total} issue${total !== 1 ? 's' : ''} (${errors} critical)`;

    return {
      name: 'Security',
      status,
      summary,
      details: details.length > 0 ? details : undefined,
      metrics: { errors, warnings, total },
      duration: Date.now() - start,
    };
  } catch (error: unknown) {
    const msg = error instanceof Error ? error.message : String(error);
    return {
      name: 'Security',
      status: 'failed',
      summary: `Security scan error: ${msg.slice(0, 100)}`,
      details: [msg],
      duration: Date.now() - start,
    };
  }
}
