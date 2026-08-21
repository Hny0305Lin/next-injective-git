// Shared URL parsing and ref utilities extracted from Repo.tsx

import { useEffect, useState } from "react";
import type { AppConfig, RefInfo } from "../../lib/chain";
import type { RepoStore } from "../../lib/gitstore";

export function shortRef(refName: string): string {
  if (refName.startsWith("refs/heads/")) return refName.slice("refs/heads/".length);
  if (refName.startsWith("refs/")) return refName.slice("refs/".length);
  return refName;
}

/** Return the repository segment from either an EVM or archive route base. */
export function repoNameFromBase(base: string): string {
  const segment = base.split("/").filter(Boolean).at(-1) ?? "";
  try {
    return decodeURIComponent(segment);
  } catch {
    return segment;
  }
}

export function isCosmWasmV1ArchivePath(pathname: string): boolean {
  return pathname === "/archive/cosmwasm-v1" || pathname.startsWith("/archive/cosmwasm-v1/");
}

export function findRef(refs: RefInfo[], short: string): RefInfo | undefined {
  return (
    refs.find((r) => r.ref_name === `refs/heads/${short}`) ??
    refs.find((r) => r.ref_name === `refs/${short}`) ??
    refs.find((r) => r.ref_name === short)
  );
}

export interface View {
  kind: "tree" | "blob" | "commits" | "commit" | "refs" | "sponsors";
  ref: string;
  path: string;
}

/**
 * decodeURIComponent that tolerates a literal '%' in the segment.
 *
 * Tree/blob links embed file paths without encoding them, so a repository
 * containing a file such as "50%.md" produces a URL whose segment is not valid
 * percent-encoding. decodeURIComponent throws URIError on those, and because
 * parseView runs during render that took down the whole route.
 */
function safeDecode(segment: string): string {
  try {
    return decodeURIComponent(segment);
  } catch {
    return segment;
  }
}

export function parseView(splat: string, fallbackRef: string): View {
  const parts = splat.split("/").filter(Boolean);
  const kind = parts[0] ?? "";
  switch (kind) {
    case "tree":
    case "blob":
      return {
        kind,
        ref: safeDecode(parts[1] ?? fallbackRef),
        path: parts.slice(2).map(safeDecode).join("/"),
      };
    case "commits":
      return { kind, ref: safeDecode(parts[1] ?? fallbackRef), path: "" };
    case "commit":
      return { kind, ref: parts[1] ?? "", path: "" };
    case "refs":
    case "sponsors":
      return { kind, ref: fallbackRef, path: "" };
    default:
      return { kind: "tree", ref: fallbackRef, path: "" };
  }
}

// ---- shared hook: load a ref's packfiles from IPFS --------------------------

export function useLoadedRef(cfg: AppConfig, store: RepoStore, current: RefInfo) {
  const [ready, setReady] = useState(false);
  const [status, setStatus] = useState("");
  const [err, setErr] = useState("");

  useEffect(() => {
    setReady(false);
    setErr("");
    store
      .loadRef(cfg, current, setStatus)
      .then(() => setReady(true))
      .catch((e: unknown) => setErr(String(e)));
  }, [current.ref_name, current.commit_sha, cfg, store]);

  return { ready, status, err };
}
