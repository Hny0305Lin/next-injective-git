// Suite version compatibility matrix.
//
// This module is the single source of truth for how the web dispatches
// repository reads by on-chain suite protocol version. CLI (git-remote-igit)
// and contracts (SuiteDirectory.sol) each maintain their own dispatch; this
// file only governs the web read path.
//
// ┌──────────┬─────────────────────┬──────────────────────┬────────────────────────┐
// │ Protocol │ Ref shape on-chain  │ Pack retrieval path  │ Web reader             │
// ├──────────┼─────────────────────┼──────────────────────┼────────────────────────┤
// │ v1       │ CosmWasm (archived) │ IPFS (historic)      │ cosmwasm-v1.ts         │
// │ v2*      │ EVM (pre-v3, none   │ IPFS (assumed same   │ v3 reader (fallback)   │
// │          │ deployed to date)   │   shape as v3)       │                        │
// │ v3       │ commitSha+packUris  │ IPFS gateway         │ registry.ts (legacy)   │
// │          │   string[]          │ (hardened reader)    │ gitstore.ts loadRef    │
// │ v4       │ manifestDigest+     │ BYOS (aws-s3/r2)     │ registry.ts (successor)│
// │          │   size+locator      │   verified manifest  │ gitstore.ts verified   │
// │ v5+      │ unknown             │ unknown              │ attempt v4 ABI, warn   │
// └──────────┴─────────────────────┴──────────────────────┴────────────────────────┘
//
// * v2: no suiteVersion=2 EVM suite has been deployed. If one ever surfaces,
//   its refs would predate the successor ABI; the IPFS-pack reader (v3 path)
//   is the only plausible shape, so it is mapped there as a fallback.
//
// Adding v5: update the READER_ABI_VERSION table below and add the new
// version to KNOWN_VERSIONS. If the ref ABI changed, also add a decoder to
// registry.ts and a reader to gitstore.ts; otherwise v4's reader covers it.
import { SUITE_VERSION, SUITE_SUCCESSOR_VERSION } from "./abis";

// ── Version constants (single import site for downstream modules) ──

export const V1_ARCHIVE = 1n;      // CosmWasm archive (read-only, separate path)
export const V3_IPFS = SUITE_VERSION;         // 3n: legacy IPFS packUris
export const V4_BYOS = SUITE_SUCCESSOR_VERSION; // 4n: successor manifest commitment

// ── Ref ABI shape dispatched by version ──

export type RefShape = "ipfs-pack-uris" | "manifest-commitment" | "unknown";

/** Which on-chain ref ABI shape a given suite protocol version uses. */
export function refShapeForVersion(version: bigint): RefShape {
  if (version <= V3_IPFS) return "ipfs-pack-uris";      // v1*, v2*, v3
  if (version === V4_BYOS) return "manifest-commitment"; // v4
  return "unknown";                                       // v5+
}

/**
 * True when repositories under this suite protocol version store packs in
 * BYOS buckets (aws-s3 / cloudflare-r2) behind a verified manifest, instead
 * of IPFS pack URIs. Used by repo badges to tell EVM V4 repositories apart
 * from EVM V2/V3 ones.
 */
export function usesByosStorage(version: bigint): boolean {
  return refShapeForVersion(version) === "manifest-commitment";
}

// ── Future-version policy ──

/**
 * For suite versions the web hasn't been explicitly taught about (v5+),
 * attempt to read refs using the latest known ABI (v4). This gives a
 * forward-compatible "best effort" read: if the ABI hasn't changed,
 * browsing just works; if it has, the user gets a clear decode error
 * instead of a blanket version rejection.
 */
export const FALLBACK_READER_VERSION = V4_BYOS;

/**
 * Versions the web explicitly supports (verified against a deployed suite).
 * Anything outside this set triggers a warning banner in the UI but is
 * still readable through the fallback path.
 */
export const KNOWN_VERSIONS: readonly bigint[] = [V3_IPFS, V4_BYOS];

/**
 * Returns true when the suite version is explicitly known and tested.
 * Unknown versions are readable through fallback but display a warning.
 */
export function isKnownVersion(version: bigint): boolean {
  return KNOWN_VERSIONS.includes(version);
}

/**
 * Returns the ABI version to use for reading refs from a suite of the given
 * protocol version. For known versions this is the version itself; for
 * unknown future versions it falls back to the latest known ABI.
 */
export function readerAbiVersion(suiteVersion: bigint): bigint {
  if (isKnownVersion(suiteVersion)) return suiteVersion;
  return FALLBACK_READER_VERSION;
}

/**
 * Human-readable label for the UI version badge.
 */
export function versionLabel(version: bigint): string {
  if (version === V1_ARCHIVE) return "V1 Archive";
  if (version === V3_IPFS) return "Suite v3 · IPFS";
  if (version === V4_BYOS) return "Suite v4 · BYOS";
  return `Suite v${version} (untested)`;
}
