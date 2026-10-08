import assert from "node:assert/strict";
import test from "node:test";

import {
  SuiteConfigurationError,
  SuiteVerificationError,
  formatResourceError,
} from "../src/lib/errors.ts";
import { buildSearchPath, matchPageSuggestion, parseSearchQuery } from "../src/lib/search.ts";

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

test("page suggestions match built-in app pages", () => {
  assert.deepEqual(matchPageSuggestion("settings"), { label: "Settings", to: "/settings", keywords: ["settings", "setting"] });
  assert.equal(matchPageSuggestion("  SETTINGS ").label, "Settings");
  assert.equal(matchPageSuggestion("sett").label, "Settings");
  assert.equal(matchPageSuggestion("set").label, "Settings");
  assert.equal(matchPageSuggestion("se"), null);
  assert.equal(matchPageSuggestion("map").label, "MapMonitor");
  assert.equal(matchPageSuggestion("mapmonitor").label, "MapMonitor");
  assert.equal(matchPageSuggestion("monitor").label, "Monitor");
  assert.equal(matchPageSuggestion("explorer").label, "Activity");
  assert.equal(matchPageSuggestion("activity").label, "Activity");
  assert.equal(matchPageSuggestion("ipfs").label, "IPFS");
  assert.equal(matchPageSuggestion("dashboard").label, "Dashboard");
  assert.equal(matchPageSuggestion("archive").label, "V1 Archive");
  assert.equal(matchPageSuggestion("demo"), null);
  assert.equal(matchPageSuggestion(""), null);
  assert.equal(matchPageSuggestion("x"), null);
});