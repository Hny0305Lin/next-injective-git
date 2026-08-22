import assert from "node:assert/strict";
import { execFile } from "node:child_process";
import { readFile } from "node:fs/promises";
import { promisify } from "node:util";
import test from "node:test";
import { fileURLToPath } from "node:url";

import { validateReleaseProfileSource } from "../scripts/release-profile-check.mjs";

const execFileAsync = promisify(execFile);
const profileUrl = new URL("../src/lib/profile.ts", import.meta.url);
const checkerUrl = new URL("../scripts/release-profile-check.mjs", import.meta.url);

function fixture({ defaultProfile = "injective-testnet", directory = "" } = {}) {
  return `
    const NETWORK_PROFILES = {
      "injective-testnet": { id: "injective-testnet", suiteDirectory: ${JSON.stringify(directory)} },
    };
    const DEFAULT_PROFILE = ${JSON.stringify(defaultProfile)};
  `;
}

test("checked-in Web SuiteDirectory profile passes the AST gate and CLI", async () => {
  const source = await readFile(profileUrl, "utf8");
  assert.deepEqual(validateReleaseProfileSource(source, fileURLToPath(profileUrl)), {
    defaultProfile: "injective-testnet",
    profileIds: ["injective-testnet"],
  });
  const { stdout, stderr } = await execFileAsync(process.execPath, [
    fileURLToPath(checkerUrl),
    fileURLToPath(profileUrl),
  ]);
  assert.equal(stderr, "");
  assert.match(stdout, /^WEB RELEASE PROFILE: PASS \(default=injective-testnet, profiles=1\)\r?\n$/);
});

test("release profile gate rejects a missing default", () => {
  assert.throws(
    () => validateReleaseProfileSource(fixture({ defaultProfile: "missing" })),
    /DEFAULT_PROFILE "missing" is not a key/,
  );
});

test("release profile gate rejects a Directory before evidence approval", () => {
  assert.throws(
    () => validateReleaseProfileSource(fixture({ directory: "0x1111111111111111111111111111111111111111" })),
    /must remain empty before cutover evidence approval/,
  );
});

test("release profile gate requires a static Directory field", () => {
  const source = `
    const inherited = { suiteDirectory: "" };
    const NETWORK_PROFILES = { "injective-testnet": { ...inherited } };
    const DEFAULT_PROFILE = "injective-testnet";
  `;
  assert.throws(() => validateReleaseProfileSource(source), /may contain only static property assignments/);
});
