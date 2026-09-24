import assert from "node:assert/strict";
import { readFile } from "node:fs/promises";
import test from "node:test";

const root = new URL("../", import.meta.url);
const shell = await readFile(new URL("scripts/install.sh", root), "utf8");
const powershell = await readFile(new URL("scripts/install.ps1", root), "utf8");
const workflow = await readFile(new URL(".github/workflows/release.yml", root), "utf8");

test("shell installer maps supported Unix platforms to release assets", () => {
  assert.match(shell, /mira-\$\{OS\}-\$\{ARCH\}\.tar\.gz/);
  assert.match(shell, /Linux\)/);
  assert.match(shell, /Darwin\)/);
  assert.match(shell, /x86_64\|amd64/);
  assert.match(shell, /aarch64\|arm64/);
  assert.match(shell, /SHA256SUMS/);
  assert.match(shell, /MIRA_INSTALL_DIR/);
});

test("PowerShell installer maps supported Windows architectures", () => {
  assert.match(powershell, /mira-windows-\$arch\.exe\.zip/);
  assert.match(powershell, /X64/);
  assert.match(powershell, /Arm64/);
  assert.match(powershell, /MIRA_INSTALL_DIR/);
  assert.match(powershell, /SHA256SUMS/);
});

test("release workflow publishes checksums beside all binary archives", () => {
  assert.match(workflow, /sha256sum .*SHA256SUMS/);
  assert.match(workflow, /dist\/SHA256SUMS/);
  for (const suffix of ["linux-amd64", "linux-arm64", "darwin-amd64", "darwin-arm64", "windows-amd64", "windows-arm64"]) {
    assert.match(workflow, new RegExp(suffix));
  }
});
