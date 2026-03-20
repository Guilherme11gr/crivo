import * as fs from 'fs';
import * as path from 'path';
import { RunOptions, DEFAULT_CONFIG } from './config';
import { runQualityGate } from './runner';
import { colorize, COLORS } from './utils';

const HELP = `
  quality-gate — Lightweight quality gate for TypeScript projects

  Usage:
    quality-gate [command] [options]

  Commands:
    run         Run quality gate checks (default)
    init        Initialize quality gate in current project

  Options:
    --ci        CI mode (markdown output, no colors)
    --output    Save markdown report to file
    --verbose   Show all details (not just failures)
    --fix       Auto-fix ESLint issues where possible
    --help      Show this help message

  Examples:
    npx quality-gate                           Run all checks
    npx quality-gate --verbose                 Run with full details
    npx quality-gate --ci --output report.md   CI mode with report file
    npx quality-gate init                      Setup in current project
`;

function parseArgs(argv: string[]): { command: string; options: RunOptions } {
  const args = argv.slice(2);
  let command = 'run';

  const options: RunOptions = {
    ci: false,
    verbose: false,
    fix: false,
  };

  for (let i = 0; i < args.length; i++) {
    const arg = args[i];

    switch (arg) {
      case 'run':
      case 'init':
        command = arg;
        break;
      case '--ci':
        options.ci = true;
        break;
      case '--verbose':
      case '-v':
        options.verbose = true;
        break;
      case '--fix':
        options.fix = true;
        break;
      case '--output':
      case '-o':
        options.output = args[++i];
        break;
      case '--help':
      case '-h':
        console.log(HELP);
        process.exit(0);
        break;
      default:
        if (arg.startsWith('-')) {
          console.error(`Unknown option: ${arg}`);
          console.log(HELP);
          process.exit(1);
        }
    }
  }

  return { command, options };
}

async function initProject(projectDir: string): Promise<void> {
  console.log('');
  console.log(colorize('  🚀 Initializing Quality Gate', COLORS.bold, COLORS.cyan));
  console.log('');

  // 1. Create .qualitygate.json
  const configPath = path.join(projectDir, '.qualitygate.json');
  if (fs.existsSync(configPath)) {
    console.log(colorize('  ⏭️  .qualitygate.json already exists, skipping', COLORS.yellow));
  } else {
    fs.writeFileSync(configPath, JSON.stringify(DEFAULT_CONFIG, null, 2), 'utf-8');
    console.log(colorize('  ✅ Created .qualitygate.json', COLORS.green));
  }

  // 2. Create GitHub Actions workflow
  const workflowDir = path.join(projectDir, '.github', 'workflows');
  const workflowPath = path.join(workflowDir, 'quality-gate.yml');

  if (fs.existsSync(workflowPath)) {
    console.log(colorize('  ⏭️  .github/workflows/quality-gate.yml already exists, skipping', COLORS.yellow));
  } else {
    fs.mkdirSync(workflowDir, { recursive: true });
    const templatePath = path.join(__dirname, '..', 'templates', 'quality-gate.yml');
    if (fs.existsSync(templatePath)) {
      fs.copyFileSync(templatePath, workflowPath);
    } else {
      // Inline template as fallback
      const template = `name: Quality Gate

on:
  pull_request:
    branches: [main]
  push:
    branches: [main]

permissions:
  contents: read
  pull-requests: write

jobs:
  quality-gate:
    name: Quality Gate
    runs-on: ubuntu-latest
    steps:
      - uses: actions/checkout@v4
      - uses: actions/setup-node@v4
        with:
          node-version: '20'
          cache: 'npm'
      - run: npm ci
      - name: Run Quality Gate
        run: npx quality-gate --ci --output quality-gate-report.md
      - name: Comment PR
        if: github.event_name == 'pull_request' && always()
        uses: marocchino/sticky-pull-request-comment@v2
        with:
          path: quality-gate-report.md
`;
      fs.writeFileSync(workflowPath, template, 'utf-8');
    }
    console.log(colorize('  ✅ Created .github/workflows/quality-gate.yml', COLORS.green));
  }

  console.log('');
  console.log(colorize('  Done! Next steps:', COLORS.bold));
  console.log(colorize('  1. Review .qualitygate.json and adjust thresholds', COLORS.dim));
  console.log(colorize('  2. Run: npx quality-gate', COLORS.dim));
  console.log(colorize('  3. Commit the config and workflow files', COLORS.dim));
  console.log('');
}

async function main(): Promise<void> {
  const { command, options } = parseArgs(process.argv);
  const projectDir = process.cwd();

  try {
    switch (command) {
      case 'init':
        await initProject(projectDir);
        break;

      case 'run':
      default: {
        const result = await runQualityGate(projectDir, options);
        process.exit(result.passed ? 0 : 1);
      }
    }
  } catch (error: unknown) {
    const msg = error instanceof Error ? error.message : String(error);
    console.error(colorize(`\n  ❌ Fatal error: ${msg}\n`, COLORS.red));
    process.exit(1);
  }
}

main();
