import assert from "node:assert/strict";
import { chmod, mkdir, mkdtemp, readFile, rm, writeFile } from "node:fs/promises";
import { spawnSync, execFileSync } from "node:child_process";
import { createHash } from "node:crypto";
import { tmpdir } from "node:os";
import { join } from "node:path";
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

test("shell installer accepts the platform-named binary used in release tarballs", async () => {
  if (process.platform !== "linux") return;
  const temp = await mkdtemp(join(tmpdir(), "mira-install-test-"));
  try {
    const sourceDir = join(temp, "archive-root");
    const mockBin = join(temp, "mock-bin");
    await mkdir(sourceDir);
    await mkdir(mockBin);
    const archive = join(temp, "mira-linux-amd64.tar.gz");
    const archivedBinary = join(sourceDir, "mira-linux-amd64");
    const contents = "#!/bin/sh\necho MIRA fixture\n";
    await writeFile(archivedBinary, contents);
    await chmod(archivedBinary, 0o755);
    execFileSync("tar", ["-czf", archive, "-C", sourceDir, "."]);

    const checksum = createHash("sha256").update(await readFile(archive)).digest("hex");
    const checksums = join(temp, "SHA256SUMS");
    await writeFile(checksums, `${checksum}  mira-linux-amd64.tar.gz\n`);
    const uname = join(mockBin, "uname");
    const curl = join(mockBin, "curl");
    await writeFile(uname, "#!/bin/sh\ncase \"$1\" in -s) echo Linux;; -m) echo x86_64;; esac\n");
    await writeFile(curl, [
      "#!/bin/sh",
      "out= url=",
      "while [ $# -gt 0 ]; do",
      "  if [ \"$1\" = -o ]; then out=$2; shift 2; else url=$1; shift; fi",
      "done",
      "case \"$url\" in",
      "  */SHA256SUMS) cp \"$MIRA_TEST_CHECKSUMS\" \"$out\";;",
      "  */mira-linux-amd64.tar.gz) cp \"$MIRA_TEST_ARCHIVE\" \"$out\";;",
      "  *) exit 22;;",
      "esac",
    ].join("\n") + "\n");
    await chmod(uname, 0o755);
    await chmod(curl, 0o755);

    const installDir = join(temp, "installed-bin");
    const result = spawnSync("sh", [new URL("../scripts/install.sh", import.meta.url).pathname], {
      encoding: "utf8",
      env: {
        ...process.env,
        HOME: join(temp, "home"),
        PATH: `${mockBin}:${process.env.PATH}`,
        MIRA_INSTALL_DIR: installDir,
        MIRA_TEST_ARCHIVE: archive,
        MIRA_TEST_CHECKSUMS: checksums,
      },
    });

    assert.equal(result.status, 0, result.stderr);
    assert.equal(await readFile(join(installDir, "mira"), "utf8"), contents);
  } finally {
    await rm(temp, { recursive: true, force: true });
  }
});

test("PowerShell installer maps supported Windows architectures", () => {
  assert.match(powershell, /mira-windows-\$arch\.exe\.zip/);
  assert.match(powershell, /X64/);
  assert.match(powershell, /Arm64/);
  assert.match(powershell, /MIRA_INSTALL_DIR/);
  assert.match(powershell, /SHA256SUMS/);
});

test("PowerShell installer extracts the executable name published in the archive", () => {
  assert.match(powershell, /\$binaryName\s*=\s*\$asset\s*-replace\s+'\\\\?\.zip\$'/);
  assert.match(powershell, /Join-Path \$extractedDir \$binaryName/);
});

test("release workflow publishes checksums beside all binary archives", () => {
  assert.match(workflow, /sha256sum .*SHA256SUMS/);
  assert.match(workflow, /dist\/SHA256SUMS/);
  for (const suffix of ["linux-amd64", "linux-arm64", "darwin-amd64", "darwin-arm64", "windows-amd64", "windows-arm64"]) {
    assert.match(workflow, new RegExp(suffix));
  }
});
