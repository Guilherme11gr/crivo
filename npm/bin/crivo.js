#!/usr/bin/env node
"use strict";

const { execFileSync } = require("child_process");
const { ensureBinary } = require("../install");

async function main() {
  let binaryPath;
  try {
    binaryPath = await ensureBinary();
  } catch (err) {
    console.error(`Failed to install crivo: ${err.message}`);
    process.exitCode = 1;
    return;
  }

  try {
    execFileSync(binaryPath, process.argv.slice(2), { stdio: "inherit" });
  } catch (err) {
    if (err.status !== undefined) {
      process.exitCode = err.status;
      return;
    }
    console.error(`Failed to run crivo: ${err.message}`);
    console.error(`Expected binary at: ${binaryPath}`);
    console.error(`Run: node node_modules/crivo/install.js`);
    process.exitCode = 1;
  }
}

main();
