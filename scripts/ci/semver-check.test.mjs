import assert from "node:assert/strict";
import { spawnSync } from "node:child_process";
import { dirname, resolve } from "node:path";
import test from "node:test";
import { fileURLToPath } from "node:url";

import { isReleaseVersion } from "./semver-check.mjs";

const script = resolve(dirname(fileURLToPath(import.meta.url)), "semver-check.mjs");

test("accepts strict SemVer 2.0.0 release tags", () => {
  for (const value of [
    "v0.0.0",
    "v1.2.3",
    "v1.2.3-rc.1+build.2",
    "v1.0.0-alpha",
    "v1.0.0-alpha.1",
    "v1.0.0-0.3.7",
    "v1.0.0-x.7.z.92",
    "v1.0.0+20130313144700",
  ]) {
    assert.equal(isReleaseVersion(value), true, value);
  }
});

test("rejects malformed or non-canonical release tags", () => {
  for (const value of [
    "1.2.3",
    "v01.2.3",
    "v1.02.3",
    "v1.2.03",
    "v1.2",
    "v1.2.3-",
    "v1.2.3-a..b",
    "v1.2.3-01",
    "v1.2.3+",
    "v1.2.3+build..2",
    "v1.2.3_rc1",
  ]) {
    assert.equal(isReleaseVersion(value), false, value);
  }
});

test("CLI exits nonzero for invalid tags and succeeds for valid tags", () => {
  const valid = spawnSync(process.execPath, [script, "v1.2.3-rc.1+build.2"], { encoding: "utf8" });
  assert.equal(valid.status, 0, valid.stderr);
  assert.match(valid.stdout, /release tag check: pass/);

  const invalid = spawnSync(process.execPath, [script, "v01.2.3"], { encoding: "utf8" });
  assert.equal(invalid.status, 1);
  assert.match(invalid.stderr, /not strict SemVer/);
});
