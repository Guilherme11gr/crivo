"use strict";

const assert = require("node:assert/strict");
const http = require("node:http");
const test = require("node:test");

const packageJson = require("./package.json");
const { fetch } = require("./install");

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
