#!/usr/bin/env node
'use strict';

// Cognitive Complexity Analyzer (AST-based)
// Implements SonarSource Cognitive Complexity specification
// https://www.sonarsource.com/docs/CognitiveComplexity.pdf
//
// Usage: node cognitive.js <directory> [--threshold=N] [--exclude=dir1,dir2]
// Output: JSON with functions array and summary

let ts;
try {
  ts = require('typescript');
} catch {
  console.log(JSON.stringify({ error: 'typescript package not found in node_modules' }));
  process.exit(1);
}

const fs = require('fs');
const path = require('path');

// ---------------------------------------------------------------------------
// CLI
// ---------------------------------------------------------------------------

const args = process.argv.slice(2);
const dir = args[0];

if (!dir) {
  console.log(JSON.stringify({ error: 'usage: node cognitive.js <directory>' }));
  process.exit(1);
}

let threshold = 15;
const excludeDirs = new Set([
  'node_modules', '.next', 'dist', 'build', 'coverage', '.git',
  '__mocks__', '__tests__', '__fixtures__',
]);
const extensions = new Set(['.ts', '.tsx', '.js', '.jsx']);

for (const arg of args.slice(1)) {
  if (arg.startsWith('--threshold=')) {
    threshold = parseInt(arg.split('=')[1], 10);
  }
  if (arg.startsWith('--exclude=')) {
    for (const d of arg.split('=')[1].split(',')) {
      excludeDirs.add(d.trim());
    }
  }
}

// ---------------------------------------------------------------------------
// File walking
// ---------------------------------------------------------------------------

function walkDir(dirPath, callback) {
  let entries;
  try { entries = fs.readdirSync(dirPath, { withFileTypes: true }); }
  catch { return; }

  for (const entry of entries) {
    if (entry.isDirectory()) {
      if (!excludeDirs.has(entry.name) && !entry.name.startsWith('.')) {
        walkDir(path.join(dirPath, entry.name), callback);
      }
    } else if (entry.isFile()) {
      const ext = path.extname(entry.name);
      if (extensions.has(ext) &&
          !entry.name.endsWith('.d.ts') &&
          !entry.name.includes('.test.') &&
          !entry.name.includes('.spec.')) {
        callback(path.join(dirPath, entry.name));
      }
    }
  }
}

// ---------------------------------------------------------------------------
// AST helpers
// ---------------------------------------------------------------------------

function isFunctionLike(node) {
  return ts.isFunctionDeclaration(node) ||
         ts.isMethodDeclaration(node) ||
         ts.isArrowFunction(node) ||
         ts.isFunctionExpression(node) ||
         ts.isGetAccessor(node) ||
         ts.isSetAccessor(node);
}

/** Extract a human-readable name for a function node. Returns null for anonymous. */
function getFunctionName(node, sf) {
  // function foo() {}
  if (ts.isFunctionDeclaration(node)) {
    return node.name ? node.name.text : '<default export>';
  }
  // class method / get / set
  if (ts.isMethodDeclaration(node) || ts.isGetAccessor(node) || ts.isSetAccessor(node)) {
    return node.name ? node.name.getText(sf) : null;
  }
  // const foo = () => {} | const foo = function() {}
  if (ts.isArrowFunction(node) || ts.isFunctionExpression(node)) {
    const parent = node.parent;
    if (ts.isVariableDeclaration(parent) && parent.name) {
      return parent.name.getText(sf);
    }
    if (ts.isPropertyAssignment(parent) && parent.name) {
      return parent.name.getText(sf);
    }
    if (ts.isExportAssignment(parent)) {
      return '<default export>';
    }
    // foo = () => {}  (e.g. class field or object property)
    if (ts.isBinaryExpression(parent) &&
        parent.operatorToken.kind === ts.SyntaxKind.EqualsToken &&
        ts.isPropertyAccessExpression(parent.left)) {
      return parent.left.name.getText(sf);
    }
    // Passed as callback arg — anonymous, skip
    return null;
  }
  return null;
}

/** True if the function is at module / class level (not nested inside another function). */
function isTopLevel(node) {
  let p = node.parent;
  while (p) {
    if (isFunctionLike(p)) return false;
    p = p.parent;
  }
  return true;
}

// ---------------------------------------------------------------------------
// Cognitive Complexity computation
// ---------------------------------------------------------------------------

function computeComplexity(fnNode) {
  let complexity = 0;

  function visitBlock(block, nesting) {
    if (!block) return;
    if (ts.isBlock(block)) {
      for (const stmt of block.statements) visit(stmt, nesting);
    } else {
      // Single statement body (no braces) or expression body of arrow
      visit(block, nesting);
    }
  }

  function visit(node, nesting) {
    if (!node) return;

    // -----------------------------------------------------------------------
    // Nested function / lambda — increases nesting, contributes to parent
    // -----------------------------------------------------------------------
    if (isFunctionLike(node) && node !== fnNode) {
      const body = node.body;
      if (body) {
        if (ts.isBlock(body)) {
          for (const stmt of body.statements) visit(stmt, nesting + 1);
        } else {
          visit(body, nesting + 1);
        }
      }
      return;
    }

    // -----------------------------------------------------------------------
    // if / else if / else
    // -----------------------------------------------------------------------
    if (ts.isIfStatement(node)) {
      const isElseIf = ts.isIfStatement(node.parent) &&
                        node.parent.elseStatement === node;

      complexity += 1;                    // structural increment
      if (!isElseIf) complexity += nesting; // nesting increment (not for else-if)

      // Visit condition (may contain logical operators)
      visit(node.expression, nesting);

      // Then block — nesting + 1
      visitBlock(node.thenStatement, nesting + 1);

      // Else
      if (node.elseStatement) {
        if (ts.isIfStatement(node.elseStatement)) {
          // else-if: visit at SAME nesting (no extra nesting for the else-if)
          visit(node.elseStatement, nesting);
        } else {
          // plain else: +1 structural, no nesting increment on the +1 itself
          complexity += 1;
          visitBlock(node.elseStatement, nesting + 1);
        }
      }
      return;
    }

    // -----------------------------------------------------------------------
    // Loops: for / for-in / for-of / while / do-while
    // -----------------------------------------------------------------------
    if (ts.isForStatement(node)) {
      complexity += 1 + nesting;
      if (node.initializer) visit(node.initializer, nesting);
      if (node.condition)   visit(node.condition, nesting);
      if (node.incrementor) visit(node.incrementor, nesting);
      visitBlock(node.statement, nesting + 1);
      return;
    }
    if (ts.isForInStatement(node) || ts.isForOfStatement(node)) {
      complexity += 1 + nesting;
      visit(node.expression, nesting);
      visitBlock(node.statement, nesting + 1);
      return;
    }
    if (ts.isWhileStatement(node)) {
      complexity += 1 + nesting;
      visit(node.expression, nesting);
      visitBlock(node.statement, nesting + 1);
      return;
    }
    if (ts.isDoStatement(node)) {
      complexity += 1 + nesting;
      visitBlock(node.statement, nesting + 1);
      visit(node.expression, nesting);
      return;
    }

    // -----------------------------------------------------------------------
    // switch
    // -----------------------------------------------------------------------
    if (ts.isSwitchStatement(node)) {
      complexity += 1 + nesting;
      visit(node.expression, nesting);
      for (const clause of node.caseBlock.clauses) {
        for (const stmt of clause.statements) {
          visit(stmt, nesting + 1);
        }
      }
      return;
    }

    // -----------------------------------------------------------------------
    // try / catch
    // -----------------------------------------------------------------------
    if (ts.isTryStatement(node)) {
      visitBlock(node.tryBlock, nesting);
      if (node.catchClause) visit(node.catchClause, nesting);
      if (node.finallyBlock) visitBlock(node.finallyBlock, nesting);
      return;
    }
    if (ts.isCatchClause(node)) {
      complexity += 1 + nesting;
      visitBlock(node.block, nesting + 1);
      return;
    }

    // -----------------------------------------------------------------------
    // Ternary / conditional expression
    // -----------------------------------------------------------------------
    if (ts.isConditionalExpression(node)) {
      complexity += 1 + nesting;
      visit(node.condition, nesting);
      visit(node.whenTrue, nesting + 1);
      visit(node.whenFalse, nesting + 1);
      return;
    }

    // -----------------------------------------------------------------------
    // Logical operators: &&, ||, ??
    // Each NEW sequence of a different operator gets +1 (fundamental increment).
    // a && b && c        = +1 (one sequence)
    // a && b || c        = +2 (two sequences)
    // a && b || c && d   = +3 (three sequences)
    // -----------------------------------------------------------------------
    if (ts.isBinaryExpression(node)) {
      const op = node.operatorToken.kind;
      if (op === ts.SyntaxKind.AmpersandAmpersandToken ||
          op === ts.SyntaxKind.BarBarToken ||
          op === ts.SyntaxKind.QuestionQuestionToken) {
        // +1 only if left side is NOT the same logical operator (new sequence)
        const leftIsSameOp = ts.isBinaryExpression(node.left) &&
                             node.left.operatorToken.kind === op;
        if (!leftIsSameOp) {
          complexity += 1; // fundamental increment — no nesting added
        }
        visit(node.left, nesting);
        visit(node.right, nesting);
        return;
      }
    }

    // -----------------------------------------------------------------------
    // Labeled break / continue
    // -----------------------------------------------------------------------
    if ((ts.isBreakStatement(node) || ts.isContinueStatement(node)) && node.label) {
      complexity += 1;
    }

    // -----------------------------------------------------------------------
    // Default: recurse into children
    // -----------------------------------------------------------------------
    ts.forEachChild(node, child => visit(child, nesting));
  }

  // Start from function body
  const body = fnNode.body;
  if (body) {
    if (ts.isBlock(body)) {
      for (const stmt of body.statements) visit(stmt, 0);
    } else {
      // Arrow function with expression body: () => expr
      visit(body, 0);
    }
  }

  return complexity;
}

// ---------------------------------------------------------------------------
// File analysis
// ---------------------------------------------------------------------------

function analyzeFile(filePath, rootDir) {
  let source;
  try { source = fs.readFileSync(filePath, 'utf-8'); }
  catch { return { functions: [], lines: 0 }; }

  const relPath = path.relative(rootDir, filePath).replace(/\\/g, '/');
  const lines = source.split('\n').length;

  const scriptKind = (filePath.endsWith('.tsx') || filePath.endsWith('.jsx'))
    ? ts.ScriptKind.TSX
    : ts.ScriptKind.TS;

  let sf;
  try {
    sf = ts.createSourceFile(filePath, source, ts.ScriptTarget.Latest, true, scriptKind);
  } catch {
    return { functions: [], lines };
  }

  const results = [];

  function findFunctions(node) {
    if (isFunctionLike(node) && isTopLevel(node)) {
      const name = getFunctionName(node, sf);
      if (name) {
        const { line } = sf.getLineAndCharacterOfPosition(node.getStart(sf));
        const complexity = computeComplexity(node);
        results.push({ file: relPath, name, line: line + 1, complexity });
      }
      return; // Don't recurse — nested functions contribute to parent
    }
    ts.forEachChild(node, findFunctions);
  }

  ts.forEachChild(sf, findFunctions);
  return { functions: results, lines };
}

// ---------------------------------------------------------------------------
// Main
// ---------------------------------------------------------------------------

const allFunctions = [];
let totalLines = 0;

walkDir(dir, (filePath) => {
  const { functions, lines } = analyzeFile(filePath, dir);
  allFunctions.push(...functions);
  totalLines += lines;
});

const totalFunctions = allFunctions.length;
const totalComplexity = allFunctions.reduce((sum, f) => sum + f.complexity, 0);

const output = {
  functions: allFunctions,
  summary: {
    totalFunctions,
    totalLines,
    violations: allFunctions.filter(f => f.complexity > threshold).length,
    maxComplexity: allFunctions.reduce((max, f) => Math.max(max, f.complexity), 0),
    avgComplexity: totalFunctions > 0 ? totalComplexity / totalFunctions : 0,
  },
};

console.log(JSON.stringify(output));
