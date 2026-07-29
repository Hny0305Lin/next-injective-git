import { useState } from "react";
import { Link } from "react-router-dom";
import { listRefs, loadConfig, resolveOwner } from "../lib/chain";
import { inspectCid, type CidInfo } from "../lib/gitstore";

const fmtBytes = (n: number) =>
  n < 1024 ? `${n} B` : n < 1024 * 1024 ? `${(n / 1024).toFixed(1)} KB` : `${(n / 1024 / 1024).toFixed(2)} MB`;

interface RepoCid extends CidInfo {
  ref: string;
  uri: string;
}

// IPFS block explorer: inspect any CID, or audit every packfile CID a repo
// references on-chain (reachability + size + git object count). Doubles as the
// pinning health check — red rows are CIDs no reachable node is serving.
export default function IpfsExplorer() {
  const cfg = loadConfig();
  const [cid, setCid] = useState("");
  const [one, setOne] = useState<CidInfo | null>(null);
  const [oneBusy, setOneBusy] = useState(false);

  const [repoRef, setRepoRef] = useState("");
  const [repoCids, setRepoCids] = useState<RepoCid[] | null>(null);
  const [repoBusy, setRepoBusy] = useState(false);
  const [err, setErr] = useState("");

  const inspectOne = async () => {
    if (!cid.trim()) return;
    setOneBusy(true);
    setOne(null);
    setErr("");
    try {
      setOne(await inspectCid(cfg, cid.trim()));
    } catch (e) {
      setErr(String(e instanceof Error ? e.message : e));
    } finally {
      setOneBusy(false);
    }
  };

  const inspectRepo = async () => {
    const parts = repoRef.trim().replace(/^(igit|inj):\/\//, "").split("/");
    if (parts.length < 2) {
      setErr("enter as owner/repo (owner may be an inj1… address or username)");
      return;
    }
    setRepoBusy(true);
    setRepoCids(null);
    setErr("");
    try {
      const owner = await resolveOwner(cfg, parts[0]);
      const refs = await listRefs(cfg, owner, parts[1]);
      const seen = new Set<string>();
      const jobs: { ref: string; uri: string }[] = [];
      for (const r of refs) {
        for (const uri of r.pack_uris) {
          if (seen.has(uri)) continue;
          seen.add(uri);
          jobs.push({ ref: r.ref_name, uri });
        }
      }
      const results = await Promise.all(
        jobs.map(async (j) => ({ ...(await inspectCid(cfg, j.uri)), ref: j.ref, uri: j.uri })),
      );
      setRepoCids(results);
    } catch (e) {
      setErr(String(e instanceof Error ? e.message : e));
    } finally {
      setRepoBusy(false);
    }
  };

  const alive = repoCids?.filter((c) => c.ok).length ?? 0;

  return (
    <div className="explorer">
      <div className="explorer-head">
        <h1>IPFS Explorer</h1>
        <Link className="muted" to="/explorer">← Block explorer</Link>
      </div>
      <p className="muted">
        Packfiles are stored on IPFS and referenced on-chain. Gateway:{" "}
        <code className="mono">{cfg.ipfsGateway}</code> ·{" "}
        <Link to="/settings" className="muted">change</Link>
      </p>
      {err && <div className="error">{err}</div>}

      <h2 className="explorer-sub">Inspect a CID</h2>
      <form className="explorer-search" onSubmit={(e) => { e.preventDefault(); inspectOne(); }}>
        <input
          className="field mono"
          value={cid}
          onChange={(e) => setCid(e.target.value)}
          placeholder="ipfs://bafy… or a bare CID"
        />
        <button type="submit" disabled={oneBusy}>{oneBusy ? "…" : "Inspect"}</button>
      </form>
      {one && <CidCard c={one} gateway={cfg.ipfsGateway} />}

      <h2 className="explorer-sub">Audit a repo’s packfiles</h2>
      <p className="muted small">
        Lists every CID a repo references and checks the gateway can serve it — this is the pinning health view.
      </p>
      <form className="explorer-search" onSubmit={(e) => { e.preventDefault(); inspectRepo(); }}>
        <input
          className="field mono"
          value={repoRef}
          onChange={(e) => setRepoRef(e.target.value)}
          placeholder="owner/repo  (e.g. inj1sh4v…/demo-showcase)"
        />
        <button type="submit" disabled={repoBusy}>{repoBusy ? "checking…" : "Audit"}</button>
      </form>

      {repoCids && (
        <>
          <div className="muted small" style={{ margin: "6px 0" }}>
            {alive}/{repoCids.length} CIDs reachable via this gateway
          </div>
          <table className="explorer-table">
            <thead><tr><th>status</th><th>ref</th><th>CID</th><th>size</th><th>objects</th></tr></thead>
            <tbody>
              {repoCids.map((c) => (
                <tr key={c.uri}>
                  <td>{c.ok ? <span className="ok-tag">alive</span> : <span className="fail-tag">unreachable</span>}</td>
                  <td className="mono small">{c.ref.replace("refs/heads/", "").replace("refs/tags/", "tag:")}</td>
                  <td className="mono small">{c.cid.slice(0, 14)}…</td>
                  <td className="small">{c.ok ? fmtBytes(c.size) : "—"}</td>
                  <td className="small">{c.objectCount ?? (c.isPack ? "?" : "—")}</td>
                </tr>
              ))}
            </tbody>
          </table>
        </>
      )}
    </div>
  );
}

function CidCard({ c, gateway }: { c: CidInfo; gateway: string }) {
  const gw = gateway.replace(/\/+$/, "");
  return (
    <div className="card tx-card">
      <div className="tx-card-top">
        <code className="mono">{c.cid}</code>
        <span className={c.ok ? "ok-tag" : "fail-tag"}>{c.ok ? "reachable" : "unreachable"}</span>
      </div>
      {c.ok ? (
        <div className="tx-badges">
          <span className="kv">size <b>{fmtBytes(c.size)}</b></span>
          <span className="kv">git packfile <b>{c.isPack ? "yes" : "no"}</b></span>
          {c.isPack && <span className="kv">objects <b>{c.objectCount}</b></span>}
          {c.isPack && <span className="kv">pack v<b>{c.version}</b></span>}
          <a className="muted small" href={`${gw}/ipfs/${c.cid}`} target="_blank" rel="noreferrer">open on gateway ↗</a>
        </div>
      ) : (
        <div className="muted small">{c.error ?? "no reachable node is serving this CID"} — it may need pinning.</div>
      )}
    </div>
  );
}
