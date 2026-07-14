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
const DOWNLOAD_TIMEOUT_MS = 30_000;
const GO_INSTALL_TIMEOUT_MS = 120_000;
const MAX_REDIRECTS = 5;

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
  return platform === "windows" ? "crivo.exe" : "crivo";
}

function getDownloadUrl(platform, arch) {
  const ext = platform === "windows" ? ".zip" : ".tar.gz";
  return `https://github.com/${REPO}/releases/download/v${VERSION}/crivo_${platform}_${arch}${ext}`;
}

function fetch(url, options = {}) {
  const timeoutMs = options.timeoutMs ?? DOWNLOAD_TIMEOUT_MS;
  const deadline = options.deadline ?? Date.now() + timeoutMs;
  const redirectsRemaining = options.redirectsRemaining ?? options.maxRedirects ?? MAX_REDIRECTS;

  return new Promise((resolve, reject) => {
    const remainingMs = deadline - Date.now();
    if (remainingMs <= 0) {
      reject(new Error(`Download timed out after ${timeoutMs}ms: ${url}`));
      return;
    }

    const mod = url.startsWith("https") ? https : http;
    let settled = false;
    let timer;

    const settle = (callback, value) => {
      if (settled) return;
      settled = true;
      clearTimeout(timer);
      callback(value);
    };

    const request = mod.get(
      url,
      { headers: { "User-Agent": "crivo-npm" } },
      (res) => {
        if (res.statusCode >= 300 && res.statusCode < 400 && res.headers.location) {
          res.resume();
          if (redirectsRemaining <= 0) {
            settle(reject, new Error(`Too many redirects while downloading: ${url}`));
            return;
          }

          const redirectUrl = new URL(res.headers.location, url).toString();
          settle(
            resolve,
            fetch(redirectUrl, {
              timeoutMs,
              deadline,
              redirectsRemaining: redirectsRemaining - 1,
            })
          );
          return;
        }
        if (res.statusCode !== 200) {
          res.resume();
          settle(reject, new Error(`HTTP ${res.statusCode} for ${url}`));
          return;
        }
        const chunks = [];
        res.on("data", (chunk) => chunks.push(chunk));
        res.on("end", () => settle(resolve, Buffer.concat(chunks)));
        res.on("error", (error) => settle(reject, error));
      }
    );

    timer = setTimeout(() => {
      request.destroy(new Error(`Download timed out after ${timeoutMs}ms: ${url}`));
    }, remainingMs);
    request.on("error", (error) => settle(reject, error));
  });
}

async function extractTarGz(buffer, destDir, binaryName) {
  const tmpFile = path.join(os.tmpdir(), `crivo-${Date.now()}.tar.gz`);
  fs.writeFileSync(tmpFile, buffer);
  try {
    execSync(`tar xzf "${tmpFile}" -C "${destDir}" ${binaryName}`, { stdio: "pipe" });
  } finally {
    fs.unlinkSync(tmpFile);
  }
}

async function extractZip(buffer, destDir, binaryName) {
  const tmpFile = path.join(os.tmpdir(), `crivo-${Date.now()}.zip`);
  const tmpExtract = path.join(os.tmpdir(), `crivo-extract-${Date.now()}`);
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
    execSync(`go install github.com/${REPO}/cmd/crivo@v${VERSION}`, {
      stdio: "inherit",
      timeout: GO_INSTALL_TIMEOUT_MS,
    });
    const gopath = execSync("go env GOPATH", { encoding: "utf8" }).trim();
    const binaryName = os.platform() === "win32" ? "crivo.exe" : "crivo";
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

async function ensureBinary() {
  const { platform, arch } = getPlatform();
  const binaryName = getBinaryName(platform);
  const binDir = path.join(__dirname, "bin");
  const destPath = path.join(binDir, binaryName);

  // Skip if binary already exists
  if (fs.existsSync(destPath)) {
    return destPath;
  }

  fs.mkdirSync(binDir, { recursive: true });

  const url = getDownloadUrl(platform, arch);
  console.log(`  Downloading crivo v${VERSION} for ${platform}/${arch}...`);

  try {
    const buffer = await fetch(url);

    if (platform === "windows") {
      await extractZip(buffer, binDir, binaryName);
    } else {
      await extractTarGz(buffer, binDir, binaryName);
    }

    fs.chmodSync(destPath, 0o755);
    console.log(`  Installed crivo to ${destPath}`);
    return destPath;
  } catch (err) {
    console.warn(`  GitHub release not found: ${err.message}`);

    if (await tryGoInstall()) {
      console.log(`  Installed via go install`);
      return destPath;
    }

    throw new Error(
      `Failed to install crivo.\n` +
        `  Install manually: go install github.com/${REPO}/cmd/crivo@latest\n` +
        `  Or download from: https://github.com/${REPO}/releases`
    );
  }
}

async function main() {
  try {
    await ensureBinary();
  } catch (error) {
    console.error(`  ${error.message}`);
    process.exitCode = 1;
  }
}

if (require.main === module) {
  main();
}

module.exports = {
  DOWNLOAD_TIMEOUT_MS,
  MAX_REDIRECTS,
  ensureBinary,
  fetch,
  getDownloadUrl,
  getPlatform,
};
