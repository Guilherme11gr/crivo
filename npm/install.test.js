"use strict";

const assert = require("node:assert/strict");
const fs = require("node:fs");
const http = require("node:http");
const os = require("node:os");
const path = require("node:path");
const test = require("node:test");
const zlib = require("node:zlib");

const packageJson = require("./package.json");
const { ensureBinary, fetch, getAssetName, getPlatform, sha256Hex } = require("./install");

function listen(server) {
  return new Promise((resolve) => {
    server.listen(0, "127.0.0.1", () => resolve(server.address().port));
  });
}

function close(server) {
  return new Promise((resolve, reject) => {
    server.close((error) => (error ? reject(error) : resolve()));
  });
}

// Builds a minimal .tar.gz containing a single regular file. Enough for the
// install fixture: `tar xzf` only needs a valid archive with the binary entry.
function makeTarGz(fileName, content) {
  const header = Buffer.alloc(512);
  header.write(fileName, 0, 100, "utf8");
  header.write("0000644", 100, 8, "utf8"); // mode
  header.write("0000000", 108, 8, "utf8"); // uid
  header.write("0000000", 116, 8, "utf8"); // gid
  header.write(content.length.toString(8).padStart(11, "0") + "\0", 124, 12, "utf8"); // size
  header.write("00000000000", 136, 12, "utf8"); // mtime
  header.write("        ", 148, 8, "utf8"); // chksum placeholder (8 spaces)
  header.write("0", 156, 1, "utf8"); // typeflag: regular file

  let sum = 0;
  for (let i = 0; i < 512; i++) sum += header[i];
  header.write(sum.toString(8).padStart(6, "0") + "\0 ", 148, 8, "utf8");

  const body = Buffer.concat([
    header,
    content,
    Buffer.alloc((512 - (content.length % 512)) % 512),
  ]);
  const end = Buffer.alloc(1024); // two zero blocks mark end of archive
  return zlib.gzipSync(Buffer.concat([body, end]));
}

// Serves a fake release: the binary archive plus a checksums.txt. Returns the
// port and the archive buffer so tests can compute hashes.
async function serveRelease({ archive, checksumsText }) {
  const server = http.createServer((request, response) => {
    if (request.url === "/checksums.txt") {
      response.writeHead(200, { "Content-Type": "text/plain" });
      response.end(checksumsText);
      return;
    }
    if (request.url === `/${getAssetName(getPlatform().platform, getPlatform().arch)}`) {
      response.writeHead(200);
      response.end(archive);
      return;
    }
    response.writeHead(404);
    response.end();
  });
  const port = await listen(server);
  return { server, port };
}

function tmpBinDir() {
  return fs.mkdtempSync(path.join(os.tmpdir(), "crivo-test-"));
}

test("package installation has no automatic postinstall hook", () => {
  assert.equal(packageJson.scripts.postinstall, undefined);
  assert.equal(packageJson.scripts["install-binary"], "node install.js");
});

test("binary download fails within the configured timeout", async () => {
  const server = http.createServer(() => {});
  const port = await listen(server);

  try {
    await assert.rejects(
      fetch(`http://127.0.0.1:${port}/binary`, { timeoutMs: 50 }),
      /Download timed out after 50ms/
    );
  } finally {
    await close(server);
  }
});

test("binary download stops after the redirect limit", async () => {
  const server = http.createServer((_request, response) => {
    response.writeHead(302, { Location: "/loop" });
    response.end();
  });
  const port = await listen(server);

  try {
    await assert.rejects(
      fetch(`http://127.0.0.1:${port}/loop`, { timeoutMs: 1_000, maxRedirects: 2 }),
      /Too many redirects/
    );
  } finally {
    await close(server);
  }
});

test("install succeeds when the release checksum matches", async () => {
  const { platform, arch } = getPlatform();
  const assetName = getAssetName(platform, arch);
  const archive = makeTarGz(platform === "windows" ? "crivo.exe" : "crivo", Buffer.from("fake-binary"));
  const checksumsText = `${sha256Hex(archive)}  ${assetName}\n`;

  const { server, port } = await serveRelease({ archive, checksumsText });
  const binDir = tmpBinDir();
  try {
    const destPath = await ensureBinary({
      baseUrl: `http://127.0.0.1:${port}`,
      binDir,
    });
    assert.equal(fs.existsSync(destPath), true, "binary should be written");
    assert.equal(fs.readFileSync(destPath, "utf8"), "fake-binary");
  } finally {
    await close(server);
    fs.rmSync(binDir, { recursive: true, force: true });
  }
});

test("install fails hard when the release checksum mismatches and writes nothing", async () => {
  const { platform, arch } = getPlatform();
  const assetName = getAssetName(platform, arch);
  const archive = makeTarGz(platform === "windows" ? "crivo.exe" : "crivo", Buffer.from("fake-binary"));
  const checksumsText = `${"0".repeat(64)}  ${assetName}\n`; // wrong hash

  const { server, port } = await serveRelease({ archive, checksumsText });
  const binDir = tmpBinDir();
  try {
    await assert.rejects(
      ensureBinary({ baseUrl: `http://127.0.0.1:${port}`, binDir }),
      /Checksum verification failed/
    );
    const binaryName = platform === "windows" ? "crivo.exe" : "crivo";
    assert.equal(fs.existsSync(path.join(binDir, binaryName)), false, "no binary should be written");
  } finally {
    await close(server);
    fs.rmSync(binDir, { recursive: true, force: true });
  }
});

test("install fails hard when checksums.txt has no entry for the asset", async () => {
  const { platform, arch } = getPlatform();
  const assetName = getAssetName(platform, arch);
  const archive = makeTarGz(platform === "windows" ? "crivo.exe" : "crivo", Buffer.from("fake-binary"));
  const checksumsText = `${"0".repeat(64)}  some-other-asset.tar.gz\n`; // no entry for our asset

  const { server, port } = await serveRelease({ archive, checksumsText });
  const binDir = tmpBinDir();
  try {
    await assert.rejects(
      ensureBinary({ baseUrl: `http://127.0.0.1:${port}`, binDir }),
      new RegExp(`no entry for ${assetName}`)
    );
    const binaryName = platform === "windows" ? "crivo.exe" : "crivo";
    assert.equal(fs.existsSync(path.join(binDir, binaryName)), false, "no binary should be written");
  } finally {
    await close(server);
    fs.rmSync(binDir, { recursive: true, force: true });
  }
});
