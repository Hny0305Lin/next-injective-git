import assert from "node:assert/strict";
import test from "node:test";

import {
  SuiteConfigurationError,
  SuiteVerificationError,
  formatResourceError,
} from "../src/lib/errors.ts";
import { buildSearchPath, matchPageSuggestion, parseSearchQuery } from "../src/lib/search.ts";
import { parseSearchScope, searchSubmitTarget } from "../src/lib/search-prefix.ts";

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

test("docs queries suggest the documentation site", () => {
  const docs = matchPageSuggestion("docs");
  assert.equal(docs.label, "Docs");
  assert.equal(docs.href, "https://docs.igit.xyz/");
  assert.equal(docs.to, undefined);
  assert.equal(matchPageSuggestion("  DOCS ").label, "Docs");
  assert.equal(matchPageSuggestion("Docs").label, "Docs");
  assert.equal(matchPageSuggestion("doc").label, "Docs");
  assert.equal(matchPageSuggestion("documentation").label, "Docs");
  assert.equal(matchPageSuggestion("文档").label, "Docs");
  assert.equal(matchPageSuggestion("do"), null);
  assert.equal(matchPageSuggestion("docker"), null);
});

test("qualifier prefixes parse user: and repo: scopes", () => {
  assert.deepEqual(parseSearchScope("user:alice"), { kind: "user", value: "alice", keywords: "" });
  assert.deepEqual(parseSearchScope("USER:@Alice  demo "), { kind: "user", value: "Alice", keywords: "demo" });
  assert.deepEqual(parseSearchScope("repo:alice/demo"), { kind: "repo", value: "alice/demo", keywords: "" });
  assert.deepEqual(parseSearchScope("demo repo:widget"), { kind: "repo", value: "widget", keywords: "demo" });
  assert.deepEqual(parseSearchScope("user:inj1abc...def"), { kind: "user", value: "inj1abc...def", keywords: "" });
});

test("unregistered or malformed prefixes fall back to plain keywords", () => {
  assert.equal(parseSearchScope("plain query"), null);
  // Not registered yet: future docs:/search: prefixes keep today's behavior.
  assert.equal(parseSearchScope("docs:guide"), null);
  assert.equal(parseSearchScope("search:term"), null);
  // URL schemes and malformed tokens stay untouched.
  assert.equal(parseSearchScope("igit://@alice/demo"), null);
  assert.equal(parseSearchScope("archive://@alice/demo"), null);
  assert.equal(parseSearchScope("user:"), null);
  assert.equal(parseSearchScope("repo:"), null);
  assert.equal(parseSearchScope("user:user:alice"), null);
  assert.equal(parseSearchScope(""), null);
});

test("bare user: queries submit straight to the owner page", () => {
  assert.deepEqual(searchSubmitTarget("user:alice"), { kind: "owner", owner: "alice" });
  assert.deepEqual(searchSubmitTarget("user:@alice"), { kind: "owner", owner: "alice" });
  assert.deepEqual(searchSubmitTarget("user:alice widget"), { kind: "query", query: "user:alice widget" });
  assert.deepEqual(searchSubmitTarget("repo:demo"), { kind: "query", query: "repo:demo" });
  assert.deepEqual(searchSubmitTarget("repo:alice/demo"), { kind: "query", query: "repo:alice/demo" });
  assert.deepEqual(searchSubmitTarget("plain demo"), { kind: "query", query: "plain demo" });
  assert.equal(searchSubmitTarget("   "), null);
  assert.equal(searchSubmitTarget(""), null);
});