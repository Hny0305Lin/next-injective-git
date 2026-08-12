import { useState } from "react";
import { Link } from "react-router-dom";
import {
  configForProfile,
  loadConfig,
  NETWORK_PROFILES,
  saveConfig,
  type NetworkProfileId,
} from "../lib/chain";

const TESTNET_WALLETS = [
  { label: "MetaMask Wallet", inj: "inj1w5v3vhwpk7v8csaqxv5pzzfzvgaqn8qfuh5p5d", evm: "0x7519165DC1B7987C43A03328110922623A099C09" },
  { label: "Keplr Wallet", inj: "inj1ylxm0a96uxsfk5j7xza7jyycs6zvz9k4r9vkuc", evm: "0x27CDB7F4BAE1A09B525E30BBE910988684C116D5" },
  { label: "igit-dev", inj: "inj1sh4v00qgzjy25a73mqheew8q200punaglrzec5", evm: "0x85EAC7BC081488AA77D1D82F9CB8E053DE1E4FA8" },
  { label: "dev", inj: "inj1p6dn32ss5cxnfgcw8n4mu08x9vc2tskhnj7y3j", evm: "0x0E9B38AA10A60D34A30E3CEBBE3CE62B30A5C2D7" },
  { label: "collab-bob", inj: "inj1kwq44vsld7zk2l9d8vvgn7dkjh4jgvlffhqp3d", evm: "0xB3815AB21F6F85657CAD3B1889F9B695EB2433E9" },
];

export default function Settings() {
  const [cfg, setCfg] = useState(loadConfig());
  const [saved, setSaved] = useState(false);

  const restoreDefaults = () => {
    setCfg(configForProfile());
    setSaved(false);
  };

  const setProfile = (e: React.ChangeEvent<HTMLSelectElement>) => {
    setCfg(configForProfile(e.target.value as NetworkProfileId, cfg.contractVersion));
    setSaved(false);
  };

  const profile = NETWORK_PROFILES[cfg.profile];

  return (
    <div style={{ maxWidth: 640, margin: "0 auto" }}>
      <div className="card settings" style={{ padding: "24px" }}>
        <h2 style={{ margin: "0 0 6px", fontSize: "1.2rem" }}>Settings</h2>
        <p className="muted" style={{ margin: "0 0 20px", fontSize: "0.88rem", lineHeight: 1.5 }}>
          Stored in your browser only. Network endpoints, chain IDs, and contract addresses are
          supplied by the selected profile.
        </p>

        <label style={{ fontSize: "0.84rem", marginBottom: 6 }}>Network profile</label>
        <select className="field" style={{ marginBottom: 14 }} value={cfg.profile} onChange={setProfile}>
          {Object.values(NETWORK_PROFILES).map((option) => (
            <option key={option.id} value={option.id}>{option.label}</option>
          ))}
        </select>

        <div className="muted mono" style={{ fontSize: "0.76rem", lineHeight: 1.6, marginBottom: 16 }}>
          {profile.chainId} · EVM {profile.evmChainId} · automatic profile-managed chain routing
        </div>

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
          <div className="settings-wallets-scroll">
            <table style={{ width: "100%", minWidth: 640, borderCollapse: "collapse", fontSize: "0.88rem" }}>
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
        </div>
        <p className="muted" style={{ fontSize: "0.82rem", marginTop: 10 }}>
          Links:{" "}
          <a href={profile.explorer} target="_blank" rel="noreferrer">Injective Explorer ↗</a>
          {" · "}
          <a href={profile.evmExplorer} target="_blank" rel="noreferrer">Blockscout ↗</a>
        </p>
      </div>
    </div>
  );
}
