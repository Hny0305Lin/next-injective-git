import { CheckCircle2, ExternalLink, LoaderCircle, Lock, RotateCcw, Save } from "lucide-react";
import { useState } from "react";
import { Link } from "react-router-dom";
import {
  NETWORK_PROFILES,
  configForProfile,
  formatError,
  isLocalDeployment,
  isSuiteDirectoryConfigured,
  loadConfig,
  saveConfig,
  verifySuite,
  type NetworkProfileId,
} from "../../lib/chain";

const TESTNET_WALLETS = [
  { label: "MetaMask Wallet", inj: "inj1w5v3vhwpk7v8csaqxv5pzzfzvgaqn8qfuh5p5d", evm: "0x7519165DC1B7987C43A03328110922623A099C09" },
  { label: "Test Wallet 2", inj: "inj1ylxm0a96uxsfk5j7xza7jyycs6zvz9k4r9vkuc", evm: "0x27CDB7F4BAE1A09B525E30BBE910988684C116D5" },
  { label: "igit-dev", inj: "inj1sh4v00qgzjy25a73mqheew8q200punaglrzec5", evm: "0x85EAC7BC081488AA77D1D82F9CB8E053DE1E4FA8" },
  { label: "dev", inj: "inj1p6dn32ss5cxnfgcw8n4mu08x9vc2tskhnj7y3j", evm: "0x0E9B38AA10A60D34A30E3CEBBE3CE62B30A5C2D7" },
  { label: "collab-bob", inj: "inj1kwq44vsld7zk2l9d8vvgn7dkjh4jgvlffhqp3d", evm: "0xB3815AB21F6F85657CAD3B1889F9B695EB2433E9" },
];

export default function Settings() {
  const [cfg, setCfg] = useState(loadConfig());
  const [saved, setSaved] = useState(false);
  const [suiteInput, setSuiteInput] = useState(cfg.suiteDirectory);
  const [saving, setSaving] = useState(false);
  const [saveError, setSaveError] = useState("");
  const [suiteVerified, setSuiteVerified] = useState(false);
  const [suiteEditable] = useState(isLocalDeployment);

  const restoreDefaults = () => {
    if (!suiteEditable) return;
    const defaults = configForProfile();
    setCfg(defaults);
    setSuiteInput(defaults.suiteDirectory);
    setSaved(false);
    setSaveError("");
    setSuiteVerified(false);
  };

  const setProfile = (event: React.ChangeEvent<HTMLSelectElement>) => {
    const next = configForProfile(event.target.value as NetworkProfileId);
    setCfg(next);
    setSuiteInput(next.suiteDirectory);
    setSaved(false);
    setSaveError("");
    setSuiteVerified(false);
  };

  const persist = async () => {
    if (!suiteEditable) return;
    const suiteDirectory = suiteInput.trim();
    const next = { ...cfg, suiteDirectory };
    setSaving(true);
    setSaved(false);
    setSaveError("");
    setSuiteVerified(false);
    try {
      if (suiteDirectory) {
        if (!isSuiteDirectoryConfigured(suiteDirectory)) {
          throw new Error("Enter a 0x-prefixed, 40-byte EVM contract address.");
        }
        await verifySuite(next, true);
      }
      saveConfig(next);
      setCfg(next);
      setSuiteVerified(Boolean(suiteDirectory));
      setSaved(true);
    } catch (error) {
      setSaveError(formatError(error));
    } finally {
      setSaving(false);
    }
  };

  const profile = NETWORK_PROFILES[cfg.profile];

  return (
    <div className="settings-page">
      <div className="page-heading">
        <div>
          <h1>Settings</h1>
          <p>Configure the network profile and inspect local test wallets.</p>
        </div>
      </div>

      <section className="card settings" aria-labelledby="network-settings-title">
        <h2 id="network-settings-title" className="settings-section-title">Network profile</h2>
        <p className="muted settings-section-copy">
          Stored in your browser only. Endpoints, chain IDs, and contract addresses come from the selected profile.
        </p>

        <label htmlFor="network-profile">Profile</label>
        <select id="network-profile" className="field" value={cfg.profile} onChange={setProfile}>
          {Object.values(NETWORK_PROFILES).map((option) => (
            <option key={option.id} value={option.id}>{option.label}</option>
          ))}
        </select>

        <div className="settings-profile-meta mono">
          <span>EVM chain <b>{profile.evmChainId}</b></span>
          <span>RPC <b>{profile.evmRpc}</b></span>
        </div>

        <label htmlFor="suite-directory">SuiteDirectory address</label>
        <input
          id="suite-directory"
          className="field mono settings-suite-input"
          value={suiteInput}
          onChange={(event) => {
            if (!suiteEditable) return;
            setSuiteInput(event.target.value);
            setSaved(false);
            setSaveError("");
            setSuiteVerified(false);
          }}
          placeholder="0x..."
          spellCheck={false}
          autoComplete="off"
          readOnly={!suiteEditable}
          aria-readonly={!suiteEditable}
          title={suiteEditable ? undefined : "Read-only on a public deployment. Select the text to copy it."}
          onFocus={(event) => {
            if (!suiteEditable) event.currentTarget.select();
          }}
        />
        <p className="muted settings-field-help">
          {suiteEditable
            ? "The public testnet profile intentionally has no Directory yet. A local override is saved only after the active Suite, all seven modules, code hashes, and bindings verify successfully."
            : "This public deployment pins the Directory from its released profile, so the address is copy-only here. Run igit-web locally to verify and save your own override."}
        </p>

        <div className="settings-actions">
          <button
            className="primary"
            onClick={persist}
            disabled={saving || !suiteEditable}
            aria-disabled={saving || !suiteEditable}
          >
            {saving ? <LoaderCircle className="suite-alert-spinner" size={14} /> : <Save size={14} />}
            {saving ? "Verifying Suite..." : suiteInput.trim() ? "Verify and save" : "Save profile"}
          </button>
          <button onClick={restoreDefaults} disabled={saving || !suiteEditable} aria-disabled={saving || !suiteEditable}>
            <RotateCcw size={14} /> Restore defaults
          </button>
          {!suiteEditable && (
            <span className="settings-locked-note">
              <Lock size={13} /> Locked on public deployment
            </span>
          )}
          {saved && (
            <span className="success-message">
              <CheckCircle2 size={14} /> {suiteVerified ? "Suite verified and saved" : "Saved without a Suite"}
            </span>
          )}
        </div>
        {saveError && <div className="error settings-save-error" role="alert">{saveError}</div>}
      </section>

      <section className="settings-wallet-section" aria-labelledby="testnet-wallets-title">
        <div className="section-heading">
          <div>
            <h2 id="testnet-wallets-title">Testnet wallets</h2>
            <p>Known development accounts for the active profile.</p>
          </div>
        </div>

        <div className="card settings-wallet-card">
          <div className="settings-wallets-scroll">
            <table className="settings-wallet-table">
              <thead>
                <tr>
                  <th>Name</th>
                  <th>Injective</th>
                  <th>EVM</th>
                </tr>
              </thead>
              <tbody>
                {TESTNET_WALLETS.map((wallet) => (
                  <tr key={wallet.inj}>
                    <td>{wallet.label}</td>
                    <td className="mono"><Link to={`/${wallet.inj}`}>{wallet.inj}</Link></td>
                    <td className="mono muted">{wallet.evm}</td>
                  </tr>
                ))}
              </tbody>
            </table>
          </div>
        </div>

        <a className="settings-external-link" href={profile.evmExplorer} target="_blank" rel="noreferrer">
          Open Blockscout <ExternalLink size={13} />
        </a>
      </section>
    </div>
  );
}
