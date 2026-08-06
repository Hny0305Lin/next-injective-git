import { useState } from "react";
import { Link } from "react-router-dom";
import { loadConfig, saveConfig } from "../lib/chain";

const INJ_EXPLORER = "https://testnet.explorer.injective.network";
const EVM_EXPLORER = "https://testnet-injective.cloud.blockscout.com";

const TESTNET_WALLETS = [
  { label: "MetaMask Wallet", inj: "inj1w5v3vhwpk7v8csaqxv5pzzfzvgaqn8qfuh5p5d", evm: "0x7519165DC1B7987C43A03328110922623A099C09" },
  { label: "Keplr Wallet", inj: "inj1ylxm0a96uxsfk5j7xza7jyycs6zvz9k4r9vkuc", evm: "0x27CDB7F4BAE1A09B525E30BBE910988684C116D5" },
  { label: "igit-dev", inj: "inj1sh4v00qgzjy25a73mqheew8q200punaglrzec5", evm: "0x85EAC7BC081488AA77D1D82F9CB8E053DE1E4FA8" },
  { label: "collab-bob", inj: "inj1kwq44vsld7zk2l9d8vvgn7dkjh4jgvlffhqp3d", evm: "0xB3815AB21F6F85657CAD3B1889F9B695EB2433E9" },
];

export default function Settings() {
  const [cfg, setCfg] = useState(loadConfig());
  const [saved, setSaved] = useState(false);

  const set = (k: keyof typeof cfg) => (e: React.ChangeEvent<HTMLInputElement>) => {
    setCfg({ ...cfg, [k]: e.target.value });
    setSaved(false);
  };

  return (
    <div style={{ maxWidth: 640, margin: "0 auto" }}>
      <div className="card settings">
        <h2 style={{ margin: "0 0 4px" }}>Settings</h2>
        <p className="muted" style={{ margin: "0 0 16px", fontSize: "0.82rem" }}>
          Stored in your browser only. This app is fully client-side: it reads the chain through an
          LCD endpoint and fetches packfiles from an IPFS gateway.
        </p>
        <label>LCD endpoint</label>
        <input className="field mono" value={cfg.lcd} onChange={set("lcd")} />
        <label>repo-registry contract</label>
        <input className="field mono" value={cfg.contract} onChange={set("contract")} />
        <label>IPFS gateway</label>
        <input className="field mono" value={cfg.ipfsGateway} onChange={set("ipfsGateway")} />
        <div style={{ marginTop: 14, display: "flex", gap: 8, alignItems: "center" }}>
          <button
            onClick={() => {
              saveConfig(cfg);
              setSaved(true);
            }}
          >
            save
          </button>
          {saved && <span className="muted" style={{ fontSize: "0.82rem" }}>saved ✓</span>}
        </div>
      </div>

      <div style={{ marginTop: 24 }}>
        <div className="dash-section-title">Network</div>
        <div className="card" style={{ padding: "14px", fontSize: "0.82rem" }}>
          <div style={{ display: "flex", justifyContent: "space-between", marginBottom: 6 }}>
            <span className="muted">chain</span>
            <span><b>injective-888</b> testnet</span>
          </div>
          <div style={{ display: "flex", justifyContent: "space-between", marginBottom: 6 }}>
            <span className="muted">LCD</span>
            <code style={{ fontSize: "0.72rem" }}>{cfg.lcd}</code>
          </div>
          <div style={{ display: "flex", justifyContent: "space-between", marginBottom: 6 }}>
            <span className="muted">contract</span>
            <code style={{ fontSize: "0.72rem" }}>{cfg.contract}</code>
          </div>
          <div style={{ display: "flex", justifyContent: "space-between" }}>
            <span className="muted">gateway</span>
            <code style={{ fontSize: "0.72rem" }}>{cfg.ipfsGateway}</code>
          </div>
        </div>
      </div>

      <div style={{ marginTop: 24 }}>
        <div className="dash-section-title">Testnet wallets</div>
        <div className="card" style={{ padding: 0, overflow: "hidden" }}>
          <table className="plain" style={{ fontSize: "0.8rem" }}>
            <thead>
              <tr>
                <th>Name</th>
                <th>Injective</th>
                <th>EVM</th>
              </tr>
            </thead>
            <tbody>
              {TESTNET_WALLETS.map((w) => (
                <tr key={w.inj}>
                  <td>{w.label}</td>
                  <td className="mono">
                    <Link to={`/${w.inj}`}>{w.inj.slice(0, 14)}…</Link>
                  </td>
                  <td className="mono muted">{w.evm.slice(0, 14)}…</td>
                </tr>
              ))}
            </tbody>
          </table>
        </div>
        <p className="muted" style={{ fontSize: "0.76rem", marginTop: 8 }}>
          Links:{" "}
          <a href={INJ_EXPLORER} target="_blank" rel="noreferrer">Injective Explorer ↗</a>
          {" · "}
          <a href={EVM_EXPLORER} target="_blank" rel="noreferrer">Blockscout ↗</a>
        </p>
      </div>
    </div>
  );
}
