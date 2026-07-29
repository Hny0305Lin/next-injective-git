// In-browser git object store: downloads the packfiles referenced on-chain
// from an IPFS gateway, ingests them with isomorphic-git into an in-memory
// LightningFS, then serves tree/blob/log reads for the UI.
import LightningFS from "@isomorphic-git/lightning-fs";
import * as git from "isomorphic-git";
import type { AppConfig, RefInfo } from "./chain";

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
}

export class RepoStore {
  private fs: LightningFS;
  private dir = "/repo";
  private loaded = new Set<string>();

  constructor(storeName: string) {
    // wipe: true keeps every visit deterministic (packs are re-fetched)
    this.fs = new LightningFS(storeName, { wipe: true } as never);
  }

  /** Download + index every pack URI of a ref (in order). */
  async loadRef(cfg: AppConfig, ref: RefInfo, onProgress?: (msg: string) => void) {
    await this.ensureInit();
    for (let i = 0; i < ref.pack_uris.length; i++) {
      const uri = ref.pack_uris[i];
      if (this.loaded.has(uri)) continue;
      onProgress?.(`downloading pack ${i + 1}/${ref.pack_uris.length}`);
      const bytes = await this.fetchPack(cfg, uri);
      onProgress?.(`indexing pack ${i + 1}/${ref.pack_uris.length}`);
      await this.ingestPack(bytes, i);
      this.loaded.add(uri);
    }
  }

  private async ensureInit() {
    try {
      await this.fs.promises.stat(`${this.dir}/.git`);
    } catch {
      await git.init({ fs: this.fs, dir: this.dir, defaultBranch: "main" });
    }
  }

  private async fetchPack(cfg: AppConfig, uri: string): Promise<Uint8Array> {
    let cid = uri;
    if (uri.startsWith("ipfs://")) cid = uri.slice("ipfs://".length);
    else if (uri.includes("://")) throw new Error(`unsupported pack uri: ${uri}`);
    const gw = cfg.ipfsGateway.replace(/\/+$/, "");
    const resp = await fetch(`${gw}/ipfs/${cid}`);
    if (!resp.ok) throw new Error(`gateway HTTP ${resp.status} for ${cid}`);
    return new Uint8Array(await resp.arrayBuffer());
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
    }));
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
