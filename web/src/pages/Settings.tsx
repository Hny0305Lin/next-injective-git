import { useState } from "react";
import { loadConfig, saveConfig } from "../lib/chain";

export default function Settings() {
  const [cfg, setCfg] = useState(loadConfig());
  const [saved, setSaved] = useState(false);

  const set = (k: keyof typeof cfg) => (e: React.ChangeEvent<HTMLInputElement>) => {
    setCfg({ ...cfg, [k]: e.target.value });
    setSaved(false);
  };

  return (
    <div className="card settings" style={{ maxWidth: 640, margin: "0 auto" }}>
      <h2>Settings</h2>
      <p className="muted">
        Stored in your browser only. This app is fully client-side: it reads the chain through an
        LCD endpoint and fetches packfiles from an IPFS gateway.
      </p>
      <label>LCD endpoint</label>
      <input className="field mono" value={cfg.lcd} onChange={set("lcd")} />
      <label>repo-registry contract</label>
      <input className="field mono" value={cfg.contract} onChange={set("contract")} />
      <label>IPFS gateway (use http://127.0.0.1:8080 with a local Kubo)</label>
      <input className="field mono" value={cfg.ipfsGateway} onChange={set("ipfsGateway")} />
      <div style={{ marginTop: 16, display: "flex", gap: 8 }}>
        <button
          onClick={() => {
            saveConfig(cfg);
            setSaved(true);
          }}
        >
          save
        </button>
        {saved && <span className="muted">saved ✓</span>}
      </div>
    </div>
  );
}
