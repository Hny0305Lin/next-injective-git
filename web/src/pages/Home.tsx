import { useEffect, useState } from "react";
import { contractConfig, formatInj, loadConfig, type ContractConfig } from "../lib/chain";

export default function Home() {
  const cfg = loadConfig();
  const [cc, setCc] = useState<ContractConfig | null>(null);
  const [err, setErr] = useState("");

  useEffect(() => {
    contractConfig(cfg)
      .then(setCc)
      .catch((e) => setErr(String(e)));
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, []);

  return (
    <div>
      <div className="hero">
        <h1>
          Git, <span className="accent-grad">on-chain</span>.
        </h1>
        <p className="muted">
          Repository metadata &amp; refs live on Injective; packfiles live on IPFS.
          <br />
          Search an owner above — an <code>inj1…</code> address, a username, or{" "}
          <code>owner/repo</code>.
        </p>
        <div className="cmd">
          {`igit init my-repo "hello chain"\nigit push inj main\nigit clone igit://alice/my-repo`}
        </div>
      </div>

      {err && <div className="error">chain unreachable: {err}</div>}
      {cc && (
        <div className="stat-row">
          <div className="card stat">
            <div className="v">{cc.platform_fee_bps / 100}%</div>
            <div className="k">platform fee (sponsorships)</div>
          </div>
          <div className="card stat">
            <div className="v">
              {formatInj(cc.username_deposit.amount, cc.username_deposit.denom)}
            </div>
            <div className="k">username deposit (refundable)</div>
          </div>
          <div className="card stat">
            <div className="v mono" style={{ fontSize: "0.78rem" }}>
              {cfg.contract}
            </div>
            <div className="k">repo-registry contract (testnet)</div>
          </div>
        </div>
      )}
    </div>
  );
}
