import assert from "node:assert/strict";
import { readFile } from "node:fs/promises";
import test from "node:test";

const root = new URL("../", import.meta.url);
const read = (path) => readFile(new URL(path, root), "utf8");

const [readme, readmeFr, features, architecture, index, notice, licensing, gitignore] = await Promise.all([
  read("README.md"),
  read("README_FR.md"),
  read("docs/FEATURES.md"),
  read("docs/ARCHITECTURE.md"),
  read("docs/INDEX.md"),
  read("NOTICE.md"),
  read("docs/LICENSING.md"),
  read(".gitignore"),
]);

test("public documentation uses the implemented eight-signal CBA model", () => {
  const publicDocs = [readme, readmeFr, features, architecture, index];
  for (const doc of publicDocs) {
    assert.doesNotMatch(doc, /6(?:-Dimensional| dimensions? de scoring| scoring dimensions)/i);
  }

  assert.match(readme, /S\(m\) = ρ × δ × η × q × β × \(1−σ\) × τ × χ × υ × 𝟙\[ρ>θ\]/);
  assert.match(readmeFr, /S\(m\) = ρ × δ × η × q × β × \(1−σ\) × τ × χ × υ × 𝟙\[ρ>θ\]/);
  assert.match(architecture, /S\(m\) = ρ × δ × η × q × β × \(1-σ\) × τ × χ × υ × 𝟙\[ρ>θ\]/);
  assert.match(features, /Eight-Signal Scoring/);
});

test("licensing language distinguishes source availability from OSI open source", () => {
  assert.match(readme, /source-available/i);
  assert.match(readmeFr, /code source de MIRA est disponible/i);
  assert.match(notice, /PolyForm Noncommercial 1\.0\.0/);
  assert.match(licensing, /future(?: MIRA)? versions|versions futures/i);
  assert.match(notice, /OSI Open Source|pas.*open source.*OSI/i);
  assert.match(index, /LICENSING\.md/);
});

test("local worktree and Codex caches cannot enter a commit", () => {
  assert.match(gitignore, /^\.codex-gocache\/$/m);
  assert.match(gitignore, /^\.codex-tmp\/$/m);
  assert.match(gitignore, /^\.worktrees\/$/m);
});
