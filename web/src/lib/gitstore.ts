// In-browser git object store: downloads the packfiles referenced on-chain
// from an IPFS gateway, ingests them with isomorphic-git into an in-memory
// LightningFS, then serves tree/blob/log reads for the UI.
import LightningFS from "@isomorphic-git/lightning-fs";
import * as git from "isomorphic-git";
import type { AppConfig, RefInfo } from "./chain";
import { fetchVerifiedManifest, fetchVerifiedPack, MAX_WEB_PACK_BYTES, MAX_WEB_TOTAL_BYTES } from "./successorReader";
import { digest } from "./packmanifest";

// ---- Legacy (v3 ipfs-pack-uris) pack URI validation and fetch hardening ----
//
// The v3 protocol stores bare pack URIs on-chain with no content commitment,
// so the gateway is an untrusted transport. Before any byte reaches
// isomorphic-git we therefore: validate that the URI is exactly an IPFS CID
// (no traversal, query, fragment, backslash or other scheme), fetch with
// redirect:"error" + credentials:"omit", enforce per-pack/total byte budgets,
// verify raw-codec CID content against its sha2-256 multihash, and require
// the response to carry the git packfile magic.

export class PackUriError extends Error {}

/** CIDv1 base32 / CIDv0 base58 syntax (same house pattern as packmanifest.ts). */
const CIDV1_BASE32 = /^b[a-z2-7]{20,120}$/;
const CIDV0_BASE58 = /^Qm[1-9A-HJ-NP-Za-km-z]{44}$/;

/**
 * Validate a legacy pack URI and return the bare CID. Accepts only
 * `ipfs://<CIDv1>` / `ipfs://<CIDv0>` (or the bare CID). Everything else —
 * including `..`, `//`, `?`, `#`, `\`, whitespace and other schemes — throws,
 * so a hostile on-chain pack_uris entry can never steer the request path or
 * leave the `/ipfs/<cid>` namespace.
 */
export function parsePackCid(uri: string): string {
  if (typeof uri !== "string") throw new PackUriError("pack uri must be a string");
  let cid = uri;
  if (cid.startsWith("ipfs://")) cid = cid.slice("ipfs://".length);
  if (!CIDV1_BASE32.test(cid) && !CIDV0_BASE58.test(cid)) {
    throw new PackUriError(`unsupported pack uri: ${uri}`);
  }
  return cid;
}

const BASE32_LOWER = "abcdefghijklmnopqrstuvwxyz234567";

function decodeBase32Lower(input: string): Uint8Array {
  let bits = 0;
  let value = 0;
  const out: number[] = [];
  for (const ch of input) {
    const index = BASE32_LOWER.indexOf(ch);
    if (index < 0) throw new PackUriError("cid is not valid base32");
    value = (value << 5) | index;
    bits += 5;
    if (bits >= 8) {
      out.push((value >>> (bits - 8)) & 0xff);
      bits -= 8;
    }
  }
  return Uint8Array.from(out);
}

function readVarint(bytes: Uint8Array, offset: number): [bigint, number] {
  let value = 0n;
  let shift = 0n;
  let index = offset;
  while (index < bytes.length) {
    const byte = bytes[index++];
    value |= BigInt(byte & 0x7f) << shift;
    if ((byte & 0x80) === 0) return [value, index - offset];
    shift += 7n;
    if (shift > 42n) throw new PackUriError("cid varint is too long");
  }
  throw new PackUriError("cid varint is truncated");
}

const toHex = (bytes: Uint8Array): string =>
  [...bytes].map((b) => b.toString(16).padStart(2, "0")).join("");

/**
 * Best-effort content verification of gateway bytes against the requested CID.
 *
 * - CIDv1 with the raw codec and a sha2-256 multihash: the response body is
 *   the block itself, so sha256(bytes) must equal the CID digest. A malicious
 *   or compromised gateway cannot substitute content.
 * - CIDv0 / CIDv1 dag-pb (what `ipfs add` produces for packfiles): the CID
 *   commits to the UnixFS DAG, not to the reconstructed HTTP body, so a
 *   byte-level check is impossible in the browser; these return true and rely
 *   on the packfile magic check plus the on-chain commit binding in loadRef.
 */
export async function verifyLegacyCidContent(cid: string, bytes: Uint8Array): Promise<boolean> {
  if (!CIDV1_BASE32.test(cid)) {
    if (CIDV0_BASE58.test(cid)) return true; // dag-pb UnixFS root: unverifiable here
    throw new PackUriError(`unsupported pack uri: ${cid}`);
  }
  const decoded = decodeBase32Lower(cid.slice(1)); // 'b' is the multibase prefix
  const [version, versionLen] = readVarint(decoded, 0);
  if (version !== 1n) return false;
  const [codec, codecLen] = readVarint(decoded, versionLen);
  if (codec !== 0x55n) return true; // dag-pb / other DAG codecs: skip byte check
  const multihash = decoded.subarray(versionLen + codecLen);
  if (multihash.length !== 34 || multihash[0] !== 0x12 || multihash[1] !== 0x20) return true; // non sha2-256: skip
  return (await digest(bytes)) === toHex(multihash.subarray(2));
}

function isPackfileMagic(bytes: Uint8Array): boolean {
  return bytes.length >= 12 && bytes[0] === 0x50 && bytes[1] === 0x41 && bytes[2] === 0x43 && bytes[3] === 0x4b;
}

export interface PackFetchBudget {
  total: number;
}

/**
 * Fetch one legacy pack URI through the configured IPFS gateway with the same
 * transport discipline as the verified BYOS reader: no redirects, no
 * credentials, declared+actual length budgets, CID syntax validation and
 * (for raw CIDs) content-hash verification before the bytes are returned.
 */
export async function fetchLegacyPack(
  cfg: AppConfig,
  uri: string,
  budget: PackFetchBudget,
): Promise<Uint8Array> {
  const cid = parsePackCid(uri);
  const gateway = cfg.ipfsGateway.replace(/\/+$/, "");
  const url = `${gateway}/ipfs/${encodeURIComponent(cid)}`;
  let response: Response;
  try {
    response = await fetch(url, { credentials: "omit", redirect: "error" });
  } catch (error) {
    throw new PackUriError(`gateway fetch failed for ${cid}: ${error instanceof Error ? error.message : String(error)}`);
  }
  if (!response.ok) throw new PackUriError(`gateway HTTP ${response.status} for ${cid}`);
  const declared = Number(response.headers.get("content-length") ?? "");
  if (Number.isFinite(declared) && declared > MAX_WEB_PACK_BYTES) {
    throw new PackUriError(`pack ${cid} declares ${declared} bytes, over the ${MAX_WEB_PACK_BYTES}-byte web budget`);
  }
  const bytes = new Uint8Array(await response.arrayBuffer());
  if (bytes.length > MAX_WEB_PACK_BYTES) {
    throw new PackUriError(`pack ${cid} is ${bytes.length} bytes, over the ${MAX_WEB_PACK_BYTES}-byte web budget`);
  }
  if (budget.total + bytes.length > MAX_WEB_TOTAL_BYTES) {
    throw new PackUriError("repository packs exceed the total web download budget");
  }
  budget.total += bytes.length;
  if (!(await verifyLegacyCidContent(cid, bytes))) {
    throw new PackUriError(`pack bytes do not match the requested CID ${cid}`);
  }
  if (!isPackfileMagic(bytes)) {
    throw new PackUriError(`content for ${cid} is not a git packfile`);
  }
  return bytes;
}

export interface TreeItem {
  name: string;
  oid: string;
  type: "blob" | "tree" | "commit" | "special";
  mode: string;
}

export interface CommitMeta {
  oid: string;
  message: string;
  author: string;
  timestamp: number;
  parents: string[];
}

export type ChangeKind = "added" | "removed" | "modified";

export interface FileChange {
  path: string;
  kind: ChangeKind;
  /** unified diff for text files; null for binary/oversized */
  patch: string | null;
}

// One RepoStore per owner/repo, shared across route changes so packs are
// only downloaded and indexed once per page load.
const stores = new Map<string, RepoStore>();
export function getRepoStore(key: string): RepoStore {
  let s = stores.get(key);
  if (!s) {
    s = new RepoStore(`igit-${key}`);
    stores.set(key, s);
  }
  return s;
}

export class RepoStore {
  private fs: LightningFS;
  private dir = "/repo";
  private loaded = new Set<string>();

  constructor(storeName: string) {
    // wipe: true keeps every visit deterministic (packs are re-fetched)
    this.fs = new LightningFS(storeName, { wipe: true } as never);
  }

  /**
   * Download + index every pack URI of a ref (in order).
   *
   * Legacy v3 path: the on-chain ref carries no content commitment beyond the
   * CID itself, so every fetch goes through fetchLegacyPack (strict CID
   * validation, redirect/credential/budget discipline, raw-CID hash checks)
   * and, after ingestion, the packs must contain the on-chain commit_sha —
   * otherwise the loaded content is rejected as not belonging to this ref.
   */
  async loadRef(cfg: AppConfig, ref: RefInfo, onProgress?: (msg: string) => void) {
    await this.ensureInit();
    const budget: PackFetchBudget = { total: 0 };
    for (let i = 0; i < ref.pack_uris.length; i++) {
      const uri = ref.pack_uris[i];
      if (this.loaded.has(uri)) continue;
      onProgress?.(`downloading pack ${i + 1}/${ref.pack_uris.length}`);
      const bytes = await fetchLegacyPack(cfg, uri, budget);
      onProgress?.(`indexing pack ${i + 1}/${ref.pack_uris.length}`);
      await this.ingestPack(bytes, i);
      this.loaded.add(uri);
    }
    // Bind the ingested objects to the on-chain ref: the recorded commit must
    // resolve locally, so substituted pack bytes that don't carry the chain's
    // commit_sha are rejected before the UI renders anything from them.
    if (ref.commit_sha) {
      try {
        await git.readCommit({ fs: this.fs, dir: this.dir, oid: ref.commit_sha });
      } catch {
        throw new PackUriError(`loaded packs do not contain the on-chain commit ${ref.commit_sha}`);
      }
    }
  }

  private async ensureInit() {
    try {
      await this.fs.promises.stat(`${this.dir}/.git`);
    } catch {
      await git.init({ fs: this.fs, dir: this.dir, defaultBranch: "main" });
    }
  }

  private async ingestPack(bytes: Uint8Array, seq: number) {
    const packDir = `${this.dir}/.git/objects/pack`;
    await this.mkdirp(packDir);
    const name = `pack-web${seq}-${Date.now()}.pack`;
    await this.fs.promises.writeFile(`${packDir}/${name}`, bytes);
    await git.indexPack({
      fs: this.fs,
      dir: this.dir,
      filepath: `.git/objects/pack/${name}`,
    });
  }

  private async mkdirp(path: string) {
    const parts = path.split("/").filter(Boolean);
    let cur = "";
    for (const p of parts) {
      cur += `/${p}`;
      try {
        await this.fs.promises.mkdir(cur);
      } catch {
        /* exists */
      }
    }
  }

  /**
   * Verified BYOS load path for successor refs: the manifest commitment is
   * fetched with a bounded length, checked against the on-chain digest/size,
   * parsed against the repo/ref context, and each pack is downloaded, hashed
   * and size-checked before isomorphic-git ingests it. Memory budgets reject
   * oversized repositories instead of exhausting the tab.
   */
  async loadVerifiedRef(cfg: AppConfig, ref: RefInfo, onProgress?: (msg: string) => void) {
    await this.ensureInit();
    if (!ref.commitment) throw new Error("ref has no on-chain commitment");
    const commitment = ref.commitment;
    const manifest = await fetchVerifiedManifest(
      { chainId: String(cfg.evmChainId), suiteDirectory: commitment.suite_directory, repoId: commitment.repo_id, refName: ref.ref_name },
      { sha256: commitment.manifest_digest, size: String(commitment.manifest_size), bootstrapLocator: commitment.bootstrap_locator },
    );
    const budget = { total: 0 };
    for (let i = 0; i < manifest.packs.length; i++) {
      const entry = manifest.packs[i];
      if (this.loaded.has(entry.sha256)) continue;
      onProgress?.(`downloading verified pack ${i + 1}/${manifest.packs.length}`);
      const bytes = await fetchVerifiedPack(entry, budget);
      onProgress?.(`indexing verified pack ${i + 1}/${manifest.packs.length}`);
      await this.ingestPack(bytes, i);
      this.loaded.add(entry.sha256);
    }
  }
  /**
   * Storage-neutral dispatch by ref shape (see suite-compat.ts):
   *   - manifest-commitment (v4+): verified BYOS reader
   *   - ipfs-pack-uris (v3-): hardened legacy IPFS reader (strict CID
   *     validation + transport/budget discipline + on-chain commit binding;
   *     no other entry point may fetch legacy packs)
   * Unknown future versions with a commitment field also use the verified reader.
   */
  async loadRefVerifiedDispatch(cfg: AppConfig, ref: RefInfo, onProgress?: (msg: string) => void) {
    if (ref.commitment) return this.loadVerifiedRef(cfg, ref, onProgress);
    return this.loadRef(cfg, ref, onProgress);
  }
  /** List a directory at a commit; path "" = repo root. */
  async listTree(commit: string, path: string): Promise<TreeItem[]> {
    const oid = await this.treeOidAtPath(commit, path);
    const { tree } = await git.readTree({ fs: this.fs, dir: this.dir, oid });
    return tree.map((e) => ({
      name: e.path,
      oid: e.oid,
      type: e.type === "blob" ? "blob" : e.type === "tree" ? "tree" : "special",
      mode: e.mode,
    }));
  }

  /** Read a file blob at a commit. */
  async readFile(commit: string, path: string): Promise<Uint8Array> {
    const { blob } = await git.readBlob({
      fs: this.fs,
      dir: this.dir,
      oid: commit,
      filepath: path,
    });
    return blob;
  }

  /** Commit history reachable from a commit (newest first). */
  async log(commit: string, depth = 50): Promise<CommitMeta[]> {
    const entries = await git.log({
      fs: this.fs,
      dir: this.dir,
      ref: commit,
      depth,
    });
    return entries.map((e) => ({
      oid: e.oid,
      message: e.commit.message,
      author: e.commit.author.name,
      timestamp: e.commit.author.timestamp,
      parents: e.commit.parent,
    }));
  }

  /** Metadata of a single commit. */
  async commitMeta(oid: string): Promise<CommitMeta> {
    const { commit } = await git.readCommit({ fs: this.fs, dir: this.dir, oid });
    return {
      oid,
      message: commit.message,
      author: commit.author.name,
      timestamp: commit.author.timestamp,
      parents: commit.parent,
    };
  }

  /** Changed files between a commit and its first parent (or empty tree). */
  async diffCommit(oid: string): Promise<FileChange[]> {
    const meta = await this.commitMeta(oid);
    const parent = meta.parents[0];
    const trees = parent
      ? [git.TREE({ ref: parent }), git.TREE({ ref: oid })]
      : [git.TREE({ ref: oid })]; // root commit: everything is an add

    const { createTwoFilesPatch } = await import("diff");
    const changes: FileChange[] = [];

    const textOrNull = (b: Uint8Array | void | undefined) =>
      b instanceof Uint8Array ? decodeText(b) : "";
    const patchFor = (path: string, a: string | null, b: string | null): string | null => {
      if (a === null || b === null) return null; // binary
      if (a.length + b.length > 400_000) return null; // too large to render
      return createTwoFilesPatch(path, path, a, b, "", "", { context: 3 });
    };

    if (!parent) {
      // root commit: walk the single tree, mark every blob as added
      await git.walk({
        fs: this.fs,
        dir: this.dir,
        trees,
        map: async (path, [entry]) => {
          if (path === "." || !entry || (await entry.type()) !== "blob") return;
          const content = textOrNull(await entry.content());
          changes.push({ path, kind: "added", patch: patchFor(path, "", content) });
        },
      });
      return changes;
    }

    await git.walk({
      fs: this.fs,
      dir: this.dir,
      trees,
      map: async (path, [a, b]) => {
        if (path === ".") return;
        const aType = a ? await a.type() : undefined;
        const bType = b ? await b.type() : undefined;
        if (aType === "tree" || bType === "tree") return; // recurse implicitly
        const aOid = a ? await a.oid() : undefined;
        const bOid = b ? await b.oid() : undefined;
        if (aOid === bOid) return;
        if (a && !b) {
          changes.push({
            path,
            kind: "removed",
            patch: patchFor(path, textOrNull(await a.content()), ""),
          });
        } else if (!a && b) {
          changes.push({
            path,
            kind: "added",
            patch: patchFor(path, "", textOrNull(await b.content())),
          });
        } else if (a && b) {
          changes.push({
            path,
            kind: "modified",
            patch: patchFor(path, textOrNull(await a.content()), textOrNull(await b.content())),
          });
        }
      },
    });
    return changes;
  }

  private async treeOidAtPath(commit: string, path: string): Promise<string> {
    const { commit: c } = await git.readCommit({ fs: this.fs, dir: this.dir, oid: commit });
    let oid = c.tree;
    if (!path) return oid;
    for (const part of path.split("/").filter(Boolean)) {
      const { tree } = await git.readTree({ fs: this.fs, dir: this.dir, oid });
      const entry = tree.find((e) => e.path === part && e.type === "tree");
      if (!entry) throw new Error(`directory not found: ${path}`);
      oid = entry.oid;
    }
    return oid;
  }
}

/** Heuristic: treat as text if it decodes cleanly and has no NUL bytes. */
export function decodeText(bytes: Uint8Array): string | null {
  if (bytes.includes(0)) return null;
  try {
    return new TextDecoder("utf-8", { fatal: true }).decode(bytes);
  } catch {
    return null;
  }
}

// ---- IPFS block explorer: fetch a CID and inspect the git packfile ----

export interface CidInfo {
  cid: string;
  ok: boolean; // reachable through the gateway
  status: number; // HTTP status
  size: number; // bytes
  isPack: boolean; // starts with the "PACK" magic
  version: number | null;
  objectCount: number | null; // parsed from the pack header
  error?: string;
}

/**
 * Fetch a pack URI (ipfs://<cid> or bare cid) through the configured gateway
 * and inspect it: reachability, size, and — if it's a git packfile — the
 * object count from the 12-byte header ("PACK" + u32 version + u32 count).
 */
export async function inspectCid(cfg: AppConfig, uri: string): Promise<CidInfo> {
  const info: CidInfo = { cid: "", ok: false, status: 0, size: 0, isPack: false, version: null, objectCount: null };
  try {
    const cid = parsePackCid(uri);
    info.cid = cid;
    const gw = cfg.ipfsGateway.replace(/\/+$/, "");
    const resp = await fetch(`${gw}/ipfs/${encodeURIComponent(cid)}`, { credentials: "omit", redirect: "error" });
    info.status = resp.status;
    if (!resp.ok) {
      info.error = `gateway HTTP ${resp.status}`;
      return info;
    }
    const bytes = new Uint8Array(await resp.arrayBuffer());
    info.ok = true;
    info.size = bytes.length;
    if (bytes.length >= 12 && bytes[0] === 0x50 && bytes[1] === 0x41 && bytes[2] === 0x43 && bytes[3] === 0x4b) {
      // "PACK" magic — read big-endian u32 version (4..8) and count (8..12)
      const dv = new DataView(bytes.buffer, bytes.byteOffset, bytes.byteLength);
      info.isPack = true;
      info.version = dv.getUint32(4, false);
      info.objectCount = dv.getUint32(8, false);
    }
    return info;
  } catch (e) {
    info.error = e instanceof Error ? e.message : String(e);
    return info;
  }
}
