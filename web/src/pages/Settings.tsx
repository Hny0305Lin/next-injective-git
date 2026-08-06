import { useState } from "react";
import { Link } from "react-router-dom";
import { loadConfig, saveConfig, type AppConfig } from "../lib/chain";

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

  const restoreDefaults = () => {
    const def: AppConfig = {
      lcd: "https://k8s.testnet.lcd.injective.network",
      contract: "inj1mg6x7ht3zyyszed9aq67q6kd0y5rtq7wf756jh",
      ipfsGateway: "https://igit-hk.haohanyh.ovh",
    };
    setCfg(def);
    setSaved(false);
  };

  const set = (k: keyof typeof cfg) => (e: React.ChangeEvent<HTMLInputElement>) => {
    setCfg({ ...cfg, [k]: e.target.value });
    setSaved(false);
  };

  return (
    <div style={{ maxWidth: 640, margin: "0 auto" }}>
      <div className="card settings" style={{ padding: "24px" }}>
        <h2 style={{ margin: "0 0 6px", fontSize: "1.2rem" }}>Settings</h2>
        <p className="muted" style={{ margin: "0 0 20px", fontSize: "0.88rem", lineHeight: 1.5 }}>
          Stored in your browser only. This app is fully client-side: it reads the chain through an
          LCD endpoint and fetches packfiles from an IPFS gateway.
        </p>

        <label style={{ fontSize: "0.84rem", marginBottom: 6 }}>LCD endpoint</label>
        <input className="field mono" style={{ marginBottom: 14 }} value={cfg.lcd} onChange={set("lcd")} />

        <label style={{ fontSize: "0.84rem", marginBottom: 6 }}>repo-registry contract</label>
        <input className="field mono" style={{ marginBottom: 14 }} value={cfg.contract} onChange={set("contract")} />

        <label style={{ fontSize: "0.84rem", marginBottom: 6 }}>IPFS gateway</label>
        <input className="field mono" style={{ marginBottom: 16 }} value={cfg.ipfsGateway} onChange={set("ipfsGateway")} />

        <div style={{ display: "flex", gap: 10, alignItems: "center" }}>
          <button
            onClick={() => {
              saveConfig(cfg);
              setSaved(true);
            }}
            style={{ padding: "7px 18px", fontSize: "0.88rem" }}
          >
            save
          </button>
          <button
            onClick={restoreDefaults}
            style={{ padding: "7px 14px", fontSize: "0.88rem", color: "var(--fg-muted)" }}
          >
            restore defaults
          </button>
          {saved && <span className="muted" style={{ fontSize: "0.88rem" }}>saved ✓</span>}
        </div>
      </div>

      <div style={{ marginTop: 28 }}>
        <div className="dash-section-title" style={{ fontSize: "0.82rem" }}>Testnet wallets</div>
        <div className="card" style={{ padding: "16px 20px" }}>
          <table style={{ width: "100%", borderCollapse: "collapse", fontSize: "0.88rem" }}>
            <thead>
              <tr style={{ borderBottom: "1px solid var(--border)" }}>
                <th style={{ textAlign: "left", padding: "0 0 10px", fontSize: "0.78rem", fontWeight: 500, color: "var(--fg-muted)", textTransform: "uppercase", letterSpacing: "0.04em", width: 140 }}>Name</th>
                <th style={{ textAlign: "left", padding: "0 0 10px", fontSize: "0.78rem", fontWeight: 500, color: "var(--fg-muted)", textTransform: "uppercase", letterSpacing: "0.04em" }}>Injective</th>
                <th style={{ textAlign: "left", padding: "0 0 10px", fontSize: "0.78rem", fontWeight: 500, color: "var(--fg-muted)", textTransform: "uppercase", letterSpacing: "0.04em" }}>EVM</th>
              </tr>
            </thead>
            <tbody>
              {TESTNET_WALLETS.map((w) => (
                <tr key={w.inj} style={{ borderBottom: "1px solid var(--border)" }}>
                  <td style={{ padding: "10px 0", fontWeight: 500 }}>{w.label}</td>
                  <td className="mono" style={{ padding: "10px 12px 10px 0", fontSize: "0.82rem" }}>
                    <Link to={`/${w.inj}`}>{w.inj}</Link>
                  </td>
                  <td className="mono muted" style={{ padding: "10px 0", fontSize: "0.82rem" }}>{w.evm}</td>
                </tr>
              ))}
            </tbody>
          </table>
        </div>
        <p className="muted" style={{ fontSize: "0.82rem", marginTop: 10 }}>
          Links:{" "}
          <a href={INJ_EXPLORER} target="_blank" rel="noreferrer">Injective Explorer ↗</a>
          {" · "}
          <a href={EVM_EXPLORER} target="_blank" rel="noreferrer">Blockscout ↗</a>
        </p>
      </div>
    </div>
  );
}
