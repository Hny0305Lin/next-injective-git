import { ArrowUpRight, Info } from "lucide-react";
import { memo, useEffect, useMemo, useState } from "react";
import { Link } from "react-router-dom";
import { useWallet } from "../lib/WalletContext";
import { WalletModal } from "../components/WalletModal";
import {
  contractActivity,
  contractConfig,
  formatInj,
  loadConfig,
  txByHash,
  type ContractConfig,
  type ContractTx,
  type TxDetail,
} from "../lib/chain";

const short = (s: string, n = 8) => (s.length > n * 2 ? `${s.slice(0, n)}…${s.slice(-4)}` : s);

// Colour per contract action for quick scanning.
const ActionBadge = memo(function ActionBadge({ action }: { action: string }) {
  return <span className={`action-badge a-${action || "unknown"}`}>{action || "?"}</span>;
});

// A compact, contract-scoped block explorer: recent activity + tx lookup.
export default function Explorer() {
  const cfg = useMemo(() => loadConfig(), []);
  const { address, walletModalOpen, openWalletModal, closeWalletModal } = useWallet();
  const [scope, setScope] = useState<"mine" | "all">("mine");
  const [cc, setCc] = useState<ContractConfig | null>(null);
  const [rows, setRows] = useState<ContractTx[]>([]);
  const [loading, setLoading] = useState(true);
  const [err, setErr] = useState("");
  const [hash, setHash] = useState("");
  const [detail, setDetail] = useState<TxDetail | null | undefined>(undefined); // undefined = none, null = not found
  const [detailBusy, setDetailBusy] = useState(false);

  const needConnect = scope === "mine" && !address;

  useEffect(() => {
    contractConfig(cfg).then(setCc).catch(() => setCc(null));
  }, [cfg]);

  useEffect(() => {
    if (needConnect) {
      setRows([]);
      setLoading(false);
      return;
    }
    let cancelled = false;
    setLoading(true);
    setErr("");
    contractActivity(cfg, 50, scope === "mine" ? address : undefined)
      .then((a) => !cancelled && setRows(a))
      .catch((e) => !cancelled && setErr(String(e instanceof Error ? e.message : e)))
      .finally(() => !cancelled && setLoading(false));
    return () => {
      cancelled = true;
    };
  }, [scope, address, cfg]);

  const lookup = async (h: string) => {
    const q = h.trim();
    if (!q) return;
    setDetailBusy(true);
    setDetail(undefined);
    try {
      setDetail(await txByHash(cfg, q));
    } catch (e) {
      setErr(String(e instanceof Error ? e.message : e));
    } finally {
      setDetailBusy(false);
    }
  };

  return (
    <div className="explorer">
      <div className="explorer-head">
        <h1>Block Explorer</h1>
        <Link className="muted icon-link" to="/ipfs">IPFS explorer <ArrowUpRight size={14} /></Link>
      </div>
      <p className="muted">
        Activity across SuiteDirectory <code className="mono">{short(cfg.suiteDirectory, 10)}</code> ·
        EVM chain <code className="mono">{cfg.evmChainId}</code>
      </p>

      {cc && (
        <div className="card explorer-config">
          <span>platform fee <b>{(cc.platform_fee_bps / 100).toFixed(2)}%</b></span>
          <span>suite version <b>{cc.suite_version}</b></span>
          <span>treasury <code className="mono">{short(cc.treasury, 8)}</code></span>
          <span>admin <code className="mono">{short(cc.admin, 8)}</code></span>
        </div>
      )}

      <form
        className="explorer-search"
        onSubmit={(e) => {
          e.preventDefault();
          lookup(hash);
        }}
      >
        <input
          className="field mono"
          value={hash}
          onChange={(e) => setHash(e.target.value)}
          placeholder="look up a tx hash…"
        />
        <button type="submit" disabled={detailBusy}>{detailBusy ? "…" : "Inspect"}</button>
      </form>

      {detail === null && <div className="muted">tx not found (or not indexed yet).</div>}
      {detail && <TxCard d={detail} />}

      <div className="explorer-scope">
        <h2 className="explorer-sub">{scope === "mine" ? "Your activity" : "All activity"}</h2>
        <div className="scope-toggle">
          <button className={scope === "mine" ? "on" : ""} onClick={() => setScope("mine")}>Mine</button>
          <button className={scope === "all" ? "on" : ""} onClick={() => setScope("all")}>All</button>
        </div>
      </div>
      <p className="muted small privacy-note">
        <Info size={14} /> On-chain data is public. “Mine” only filters this view to your address; it does not hide anything from others.
      </p>
      {err && <div className="error">{err}</div>}
      {needConnect ? (
        <div className="sponsor-connect">
          <span className="muted">Connect your wallet to see only your on-chain activity.</span>
          <button onClick={openWalletModal}>Connect Wallet</button>
        </div>
      ) : loading ? (
        <div className="muted">loading…</div>
      ) : rows.length === 0 ? (
        <div className="muted">
          {scope === "mine" ? "no activity from your address yet." : "no contract transactions found."}
        </div>
      ) : (
        <table className="explorer-table">
          <thead>
            <tr><th>action</th><th>summary</th><th>sender</th><th>height</th><th>tx</th></tr>
          </thead>
          <tbody>
            {rows.map((r) => (
              <tr key={r.txhash}>
                <td>
                  <ActionBadge action={r.action} />
                  {r.code !== 0 && <span className="fail-tag">failed</span>}
                </td>
                <td className="mono small">{summarize(r)}</td>
                <td className="mono small">{short(r.sender, 6)}</td>
                <td className="small">{r.height}</td>
                <td>
                  <button className="linkish mono small" onClick={() => { setHash(r.txhash); lookup(r.txhash); }}>
                    {short(r.txhash, 6)}
                  </button>
                </td>
              </tr>
            ))}
          </tbody>
        </table>
      )}
      {walletModalOpen && <WalletModal onClose={closeWalletModal} />}
    </div>
  );
}

// Pick the most meaningful EVM event attributes per action for the feed summary.
function summarize(r: ContractTx): string {
  const attributes = r.attributes;
  switch (r.action) {
    case "sponsor":
      return `${attributes.owner ?? ""}/${attributes.repo ?? ""} ${attributes.funds ? formatFundsShort(attributes.funds) : ""}`.trim();
    case "update_ref":
      return `${attributes.repo ?? ""} ${attributes.ref ?? ""} ${attributes.sha ? attributes.sha.slice(0, 8) : ""}`.trim();
    case "create_repo":
      return attributes.name ?? attributes.repo ?? "";
    case "award_badge":
      return `#${attributes.badge_id ?? ""} → ${attributes.recipient ? attributes.recipient.slice(0, 12) : ""}`;
    default:
      return Object.entries(attributes).filter(([k]) => k !== "action").slice(0, 2).map(([k, v]) => `${k}=${v}`).join(" ");
  }
}
function formatFundsShort(funds: string): string {
  const m = funds.match(/(\d+)inj/);
  return m ? `${formatInj(m[1], "inj")} INJ` : funds;
}

function TxCard({ d }: { d: TxDetail }) {
  return (
    <div className="card tx-card">
      <div className="tx-card-top">
        <code className="mono">{d.txhash}</code>
        <span className={d.code === 0 ? "ok-tag" : "fail-tag"}>{d.code === 0 ? "success" : `code ${d.code}`}</span>
      </div>
      <div className="tx-meta muted small">
        height {d.height} · {new Date(d.timestamp).toLocaleString()} · gas {d.gasUsed}/{d.gasWanted}
      </div>
      <div className="tx-badges">
        <span className="kv">signMode <b>{d.signMode || "?"}</b></span>
        <span className="kv">pubkey <b className="mono">{d.pubkeyType.split(".").pop() || "?"}</b></span>
      </div>
      {d.messages.map((m, i) => (
        <div key={i} className="tx-msg">
          <div className="muted small">{m.type}</div>
          <pre className="mono small">{JSON.stringify(m.body, null, 2)}</pre>
        </div>
      ))}
      {d.rawLog && d.code !== 0 && <pre className="mono small error">{d.rawLog}</pre>}
    </div>
  );
}
