#!/usr/bin/env node
"use strict";

const { execFileSync } = require("child_process");
const path = require("path");
const os = require("os");

const binaryName = os.platform() === "win32" ? "crivo.exe" : "crivo";
const binaryPath = path.join(__dirname, binaryName);

try {
  execFileSync(binaryPath, process.argv.slice(2), { stdio: "inherit" });
} catch (err) {
  if (err.status !== undefined) {
    process.exit(err.status);
  }
  console.error(`Failed to run crivo: ${err.message}`);
  console.error(`Expected binary at: ${binaryPath}`);
  console.error(`Run: npm rebuild crivo`);
  process.exit(1);
}
