#!/usr/bin/env node
"use strict";

const https = require("https");
const http = require("http");
const fs = require("fs");
const path = require("path");
const { execSync } = require("child_process");
const os = require("os");
const zlib = require("zlib");

const VERSION = require("./package.json").version;
const REPO = "guilherme11gr/crivo";

const PLATFORM_MAP = {
  darwin: "darwin",
  linux: "linux",
  win32: "windows",
};

const ARCH_MAP = {
  x64: "amd64",
  arm64: "arm64",
};

function getPlatform() {
  const platform = PLATFORM_MAP[os.platform()];
  const arch = ARCH_MAP[os.arch()];
  if (!platform || !arch) {
    throw new Error(
      `Unsupported platform: ${os.platform()}-${os.arch()}. ` +
        `Supported: darwin-x64, darwin-arm64, linux-x64, linux-arm64, win32-x64`
    );
  }
  return { platform, arch };
}

function getBinaryName(platform) {
  return platform === "windows" ? "qg.exe" : "qg";
}

function getDownloadUrl(platform, arch) {
  const ext = platform === "windows" ? ".zip" : ".tar.gz";
  // Matches goreleaser name_template: quality-gate_<os>_<arch>
  return `https://github.com/${REPO}/releases/download/v${VERSION}/quality-gate_${platform}_${arch}${ext}`;
}

function fetch(url) {
  return new Promise((resolve, reject) => {
    const mod = url.startsWith("https") ? https : http;
    mod
      .get(url, { headers: { "User-Agent": "quality-gate-npm" } }, (res) => {
        if (res.statusCode >= 300 && res.statusCode < 400 && res.headers.location) {
          return fetch(res.headers.location).then(resolve, reject);
        }
        if (res.statusCode !== 200) {
          return reject(new Error(`HTTP ${res.statusCode} for ${url}`));
        }
        const chunks = [];
        res.on("data", (chunk) => chunks.push(chunk));
        res.on("end", () => resolve(Buffer.concat(chunks)));
        res.on("error", reject);
      })
      .on("error", reject);
  });
}

async function extractTarGz(buffer, destDir, binaryName) {
  // Use tar command (available on macOS/Linux)
  const tmpFile = path.join(os.tmpdir(), `qg-${Date.now()}.tar.gz`);
  fs.writeFileSync(tmpFile, buffer);
  try {
    execSync(`tar xzf "${tmpFile}" -C "${destDir}" ${binaryName}`, { stdio: "pipe" });
  } finally {
    fs.unlinkSync(tmpFile);
  }
}

async function extractZip(buffer, destDir, binaryName) {
  // Use PowerShell on Windows
  const tmpFile = path.join(os.tmpdir(), `qg-${Date.now()}.zip`);
  const tmpExtract = path.join(os.tmpdir(), `qg-extract-${Date.now()}`);
  fs.writeFileSync(tmpFile, buffer);
  try {
    fs.mkdirSync(tmpExtract, { recursive: true });
    execSync(
      `powershell -Command "Expand-Archive -Path '${tmpFile}' -DestinationPath '${tmpExtract}' -Force"`,
      { stdio: "pipe" }
    );
    const src = path.join(tmpExtract, binaryName);
    const dest = path.join(destDir, binaryName);
    fs.copyFileSync(src, dest);
  } finally {
    fs.unlinkSync(tmpFile);
    fs.rmSync(tmpExtract, { recursive: true, force: true });
  }
}

async function tryGoInstall() {
  console.log("  Trying go install as fallback...");
  try {
    execSync(`go install github.com/${REPO}/cmd/qg@v${VERSION}`, {
      stdio: "inherit",
    });
    // Find where go installed it
    const gopath = execSync("go env GOPATH", { encoding: "utf8" }).trim();
    const binaryName = os.platform() === "win32" ? "qg.exe" : "qg";
    const src = path.join(gopath, "bin", binaryName);
    if (fs.existsSync(src)) {
      const binDir = path.join(__dirname, "bin");
      fs.mkdirSync(binDir, { recursive: true });
      fs.copyFileSync(src, path.join(binDir, binaryName));
      fs.chmodSync(path.join(binDir, binaryName), 0o755);
      return true;
    }
  } catch {
    // go not available
  }
  return false;
}

async function main() {
  const { platform, arch } = getPlatform();
  const binaryName = getBinaryName(platform);
  const binDir = path.join(__dirname, "bin");
  const destPath = path.join(binDir, binaryName);

  // Skip if binary already exists
  if (fs.existsSync(destPath)) {
    console.log(`  quality-gate binary already exists`);
    return;
  }

  fs.mkdirSync(binDir, { recursive: true });

  const url = getDownloadUrl(platform, arch);
  console.log(`  Downloading quality-gate v${VERSION} for ${platform}/${arch}...`);

  try {
    const buffer = await fetch(url);

    if (platform === "windows") {
      await extractZip(buffer, binDir, binaryName);
    } else {
      await extractTarGz(buffer, binDir, binaryName);
    }

    fs.chmodSync(destPath, 0o755);
    console.log(`  Installed quality-gate to ${destPath}`);
  } catch (err) {
    console.warn(`  GitHub release not found: ${err.message}`);

    if (await tryGoInstall()) {
      console.log(`  Installed via go install`);
      return;
    }

    console.error(
      `  Failed to install quality-gate.\n` +
        `  Install manually: go install github.com/${REPO}/cmd/qg@latest\n` +
        `  Or download from: https://github.com/${REPO}/releases`
    );
    process.exit(1);
  }
}

main();
