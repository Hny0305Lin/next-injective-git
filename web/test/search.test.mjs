import assert from "node:assert/strict";
import test from "node:test";

import {
  SuiteConfigurationError,
  SuiteVerificationError,
  formatResourceError,
} from "../src/lib/errors.ts";
import { buildSearchPath, parseSearchQuery } from "../src/lib/search.ts";

test("global search normalizes schemes and display-prefixed usernames", () => {
  assert.deepEqual(parseSearchQuery("  igit://@alice/demo  "), {
    archive: false,
    parts: ["alice", "demo"],
  });
  assert.equal(buildSearchPath("igit://@alice/demo"), "/alice/demo");
  assert.equal(buildSearchPath("archive://@alice/demo"), "/archive/cosmwasm-v1/alice/demo");
});

test("global search encodes route segments and ignores extra path segments", () => {
  assert.equal(buildSearchPath("owner name/repo#1/refs/main"), "/owner%20name/repo%231");
  assert.equal(buildSearchPath("%40alice/demo"), "/alice/demo");
  assert.equal(buildSearchPath("@"), null);
  assert.equal(buildSearchPath("  "), null);
});

test("resource errors explain Suite configuration failures", () => {
  assert.match(
    formatResourceError(new SuiteConfigurationError(), "owner"),
    /Open Settings.*SuiteDirectory/,
  );
  assert.match(
    formatResourceError(new SuiteVerificationError("bad binding"), "owner"),
    /could not be verified.*Settings/,
  );
  assert.match(
    formatResourceError(new Error("UsernameNotFound(alice)"), "owner"),
    /Could not find this owner.*address or username/i,
  );
});
