import { useEffect, useState } from "react";
import { Link } from "react-router-dom";
import { contractConfig, formatInj, loadConfig, type ContractConfig } from "../lib/chain";

const TESTNET_WALLETS = [
  { label: "MetaMask Wallet", inj: "inj1w5v3vhwpk7v8csaqxv5pzzfzvgaqn8qfuh5p5d", evm: "0x7519165DC1B7987C43A03328110922623A099C09" },
  { label: "Keplr Wallet", inj: "inj1ylxm0a96uxsfk5j7xza7jyycs6zvz9k4r9vkuc", evm: "0x27CDB7F4BAE1A09B525E30BBE910988684C116D5" },
  { label: "igit-dev", inj: "inj1sh4v00qgzjy25a73mqheew8q200punaglrzec5", evm: "0x85EAC7BC081488AA77D1D82F9CB8E053DE1E4FA8" },
  { label: "collab-bob", inj: "inj1kwq44vsld7zk2l9d8vvgn7dkjh4jgvlffhqp3d", evm: "0xB3815AB21F6F85657CAD3B1889F9B695EB2433E9" },
];

const TESTNET_CONTRACTS = [
  { label: "repo-registry (live)", inj: "inj1mg6x7ht3zyyszed9aq67q6kd0y5rtq7wf756jh" },
  { label: "repo-registry (old)", inj: "inj17jshk9dwjhu42mx2ywhy0k2a9qy6v0d6qeua37" },
];

// External explorers for the testnet address table.
// Cosmos side (inj1…) → official Injective explorer; EVM side (0x…) → Blockscout.
const INJ_EXPLORER = "https://testnet.explorer.injective.network";
const EVM_EXPLORER = "https://testnet-injective.cloud.blockscout.com";

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
        </div>
      )}

      <div className="addr-section">
        <h2>
          igit <span className="badge">injective-888</span>
        </h2>
        <h3>Wallet</h3>
        <div className="card addr-card">
          <table className="plain addr-table">
            <thead>
              <tr>
                <th>Name</th>
                <th>Injective</th>
                <th>EVM</th>
                <th>Explorer</th>
              </tr>
            </thead>
            <tbody>
              {TESTNET_WALLETS.map((w) => (
                <tr key={w.inj}>
                  <td>{w.label}</td>
                  <td className="mono">
                    <Link to={`/${w.inj}`}>{w.inj}</Link>
                  </td>
                  <td className="mono muted">{w.evm}</td>
                  <td className="addr-links">
                    <a href={`${INJ_EXPLORER}/account/${w.inj}`} target="_blank" rel="noreferrer">
                      Injective ↗
                    </a>
                    <a href={`${EVM_EXPLORER}/address/${w.evm}`} target="_blank" rel="noreferrer">
                      Blockscout ↗
                    </a>
                  </td>
                </tr>
              ))}
            </tbody>
          </table>
        </div>
        <h3>Contract</h3>
        <div className="card addr-card">
          <table className="plain addr-table">
            <thead>
              <tr>
                <th>Name</th>
                <th>Injective</th>
                <th>Explorer</th>
              </tr>
            </thead>
            <tbody>
              {TESTNET_CONTRACTS.map((c) => (
                <tr key={c.inj}>
                  <td>{c.label}</td>
                  <td className="mono">{c.inj}</td>
                  <td className="addr-links">
                    <a href={`${INJ_EXPLORER}/contract/${c.inj}`} target="_blank" rel="noreferrer">
                      Contract ↗
                    </a>
                  </td>
                </tr>
              ))}
            </tbody>
          </table>
        </div>
      </div>
    </div>
  );
}
