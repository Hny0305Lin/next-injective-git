import assert from "node:assert/strict";
import { execFile } from "node:child_process";
import { readFile } from "node:fs/promises";
import { promisify } from "node:util";
import test from "node:test";
import { fileURLToPath } from "node:url";

import { validateReleaseProfileSource } from "../scripts/release-profile-check.mjs";

const execFileAsync = promisify(execFile);
const chainUrl = new URL("../src/lib/chain.ts", import.meta.url);
const checkerUrl = new URL("../scripts/release-profile-check.mjs", import.meta.url);

function fixture({
  defaultProfile = "injective-testnet",
  defaultVersion = "auto",
  profiles = [
    ["injective-testnet", "", "", ""],
    ["injective-mainnet", "", "", ""],
  ],
} = {}) {
  const entries = profiles.map(([id, evmContract, evmBadgeModule, evmEconomicModule]) => `
    ${JSON.stringify(id)}: {
      id: ${JSON.stringify(id)},
      evmContract: ${JSON.stringify(evmContract)},
      evmBadgeModule: ${JSON.stringify(evmBadgeModule)},
      evmEconomicModule: ${JSON.stringify(evmEconomicModule)},
    },`).join("");
  return `
    type ContractVersion = "auto" | "v1" | "v2";
    export const NETWORK_PROFILES = {${entries}
    } satisfies Record<string, object>;
    const DEFAULT_PROFILE = ${JSON.stringify(defaultProfile)};
    const DEFAULT_CONTRACT_VERSION: ContractVersion = ${JSON.stringify(defaultVersion)};
  `;
}

test("checked-in Web release profile passes the AST gate and CLI", async () => {
  const source = await readFile(chainUrl, "utf8");
  assert.deepEqual(
    validateReleaseProfileSource(source, fileURLToPath(chainUrl)),
    {
      defaultProfile: "injective-testnet",
      defaultContractVersion: "auto",
      profileIds: ["injective-testnet"],
    },
  );

  const { stdout, stderr } = await execFileAsync(
    process.execPath,
    [fileURLToPath(checkerUrl), fileURLToPath(chainUrl)],
  );
  assert.equal(stderr, "");
  assert.match(
    stdout,
    /^WEB RELEASE PROFILE: PASS \(default=injective-testnet, version=auto, profiles=1\)\r?\n$/,
  );
});

test("release profile gate rejects a default profile missing from the map", () => {
  assert.throws(
    () => validateReleaseProfileSource(fixture({ defaultProfile: "missing" })),
    /DEFAULT_PROFILE "missing" is not a key in NETWORK_PROFILES/,
  );
});

test("release profile gate rejects a non-auto default contract version", () => {
  assert.throws(
    () => validateReleaseProfileSource(fixture({ defaultVersion: "v2" })),
    /DEFAULT_CONTRACT_VERSION must be the string literal "auto"/,
  );
});

test("release profile gate rejects any preconfigured built-in V2 address", () => {
  assert.throws(
    () => validateReleaseProfileSource(fixture({
      profiles: [
        ["injective-testnet", "", "", ""],
        ["injective-mainnet", "0x1111111111111111111111111111111111111111", "", ""],
      ],
    })),
    /NETWORK_PROFILES\["injective-mainnet"\]\.evmContract must be the empty string literal/,
  );
});

test("release profile gate rejects any preconfigured badge module address", () => {
  assert.throws(
    () => validateReleaseProfileSource(fixture({
      profiles: [
        ["injective-testnet", "", "", ""],
        ["injective-mainnet", "", "0x2222222222222222222222222222222222222222", ""],
      ],
    })),
    /NETWORK_PROFILES\["injective-mainnet"\]\.evmBadgeModule must be the empty string literal/,
  );
});

test("release profile gate requires one static badge module field per profile", () => {
  const missing = `
    const NETWORK_PROFILES = {
      "injective-testnet": { evmContract: "" },
    };
    const DEFAULT_PROFILE = "injective-testnet";
    const DEFAULT_CONTRACT_VERSION = "auto";
  `;
  assert.throws(
    () => validateReleaseProfileSource(missing),
    /must define evmBadgeModule exactly once/,
  );
});

test("release profile gate rejects any preconfigured economic module address", () => {
  assert.throws(
    () => validateReleaseProfileSource(fixture({
      profiles: [
        ["injective-testnet", "", "", ""],
        ["injective-mainnet", "", "", "0x3333333333333333333333333333333333333333"],
      ],
    })),
    /NETWORK_PROFILES\["injective-mainnet"\]\.evmEconomicModule must be the empty string literal/,
  );
});

test("release profile gate requires one static economic module field per profile", () => {
  const missing = `
    const NETWORK_PROFILES = {
      "injective-testnet": { evmContract: "", evmBadgeModule: "" },
    };
    const DEFAULT_PROFILE = "injective-testnet";
    const DEFAULT_CONTRACT_VERSION = "auto";
  `;
  assert.throws(
    () => validateReleaseProfileSource(missing),
    /must define evmEconomicModule exactly once/,
  );
});

test("release profile gate fails closed on dynamic or spread profile definitions", () => {
  const dynamic = `
    const inherited = { evmContract: "", evmBadgeModule: "", evmEconomicModule: "" };
    const NETWORK_PROFILES = {
      "injective-testnet": { ...inherited },
    };
    const DEFAULT_PROFILE = "injective-testnet";
    const DEFAULT_CONTRACT_VERSION = "auto";
  `;
  assert.throws(
    () => validateReleaseProfileSource(dynamic),
    /may contain only static property assignments/,
  );
});
