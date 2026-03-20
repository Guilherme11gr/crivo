import * as fs from 'fs';
import * as path from 'path';

export interface QualityGateConfig {
  checks: {
    typescript: boolean;
    lint: boolean;
    coverage: boolean;
    duplication: boolean;
  };
  coverage: {
    lines: number;
    branches: number;
    functions: number;
    statements: number;
  };
  duplication: {
    threshold: number;
    minLines: number;
    minTokens: number;
  };
  complexity: {
    cognitive: number;
    cyclomatic: number;
  };
  structural: {
    maxLinesPerFunction: number;
    maxParams: number;
    maxDepth: number;
    maxNestedCallbacks: number;
  };
  exclude: string[];
  testExclude: string[];
  srcPattern: string;
}

export interface CheckResult {
  name: string;
  status: 'passed' | 'failed' | 'warning' | 'skipped';
  summary: string;
  details?: string[];
  metrics?: Record<string, number>;
  duration: number;
}

export interface QualityGateResult {
  passed: boolean;
  checks: CheckResult[];
  duration: number;
}

export interface RunOptions {
  ci: boolean;
  output?: string;
  verbose: boolean;
  fix: boolean;
}

export const DEFAULT_CONFIG: QualityGateConfig = {
  checks: {
    typescript: true,
    lint: true,
    coverage: true,
    duplication: true,
  },
  coverage: {
    lines: 60,
    branches: 50,
    functions: 60,
    statements: 60,
  },
  duplication: {
    threshold: 5,
    minLines: 5,
    minTokens: 50,
  },
  complexity: {
    cognitive: 15,
    cyclomatic: 10,
  },
  structural: {
    maxLinesPerFunction: 100,
    maxParams: 4,
    maxDepth: 4,
    maxNestedCallbacks: 3,
  },
  exclude: [
    '**/node_modules/**',
    '**/dist/**',
    '**/build/**',
    '**/.next/**',
  ],
  testExclude: [
    '**/*.test.*',
    '**/*.spec.*',
    '**/__tests__/**',
  ],
  srcPattern: 'src',
};

export function loadConfig(projectDir: string): QualityGateConfig {
  const configPath = path.join(projectDir, '.qualitygate.json');

  if (!fs.existsSync(configPath)) {
    return { ...DEFAULT_CONFIG };
  }

  const raw = fs.readFileSync(configPath, 'utf-8');
  const userConfig = JSON.parse(raw);
  return deepMerge(DEFAULT_CONFIG, userConfig);
}

// eslint-disable-next-line @typescript-eslint/no-explicit-any
function deepMerge<T extends Record<string, any>>(target: T, source: Partial<T>): T {
  const result = { ...target };

  for (const key of Object.keys(source) as (keyof T)[]) {
    const srcVal = source[key];
    const tgtVal = target[key];

    if (
      srcVal && typeof srcVal === 'object' && !Array.isArray(srcVal) &&
      tgtVal && typeof tgtVal === 'object' && !Array.isArray(tgtVal)
    ) {
      result[key] = deepMerge(tgtVal as Record<string, unknown>, srcVal as Record<string, unknown>) as T[keyof T];
    } else if (srcVal !== undefined) {
      result[key] = srcVal as T[keyof T];
    }
  }

  return result;
}
