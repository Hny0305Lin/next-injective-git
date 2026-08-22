import { useEffect, useState } from "react";
import type { FormEvent } from "react";
import { Link } from "react-router-dom";
import { Award, Plus, Save, Trash2 } from "lucide-react";
import {
  badgesByRepo,
  awardBadgeWithEvm,
  formatError,
  formatInj,
  listCollaborators,
  revenueSplits,
  setRevenueSplitsWithEconomicModule,
  sponsorTotals,
  timeAgo,
  type AppConfig,
  type BadgeInfo,
  type CollaboratorInfo,
  type SponsorTotal,
  type SplitEntry,
} from "../../lib/chain";
import type { Eip1193 } from "../../lib/wallet";
import type { Hex } from "viem";
import SponsorForm from "./SponsorForm";

const BAR_COLORS = ["#4493f8", "#3fb950", "#d29922", "#a371f7", "#f85149", "#39c5cf"];

export default function SponsorsTab({
  cfg,
  addr,
  repo,
  owner,
  repoId,
  badgeProvider,
  economicProvider,
}: {
  cfg: AppConfig;
  addr: string;
  repo: string;
  owner: string;
  repoId: Hex | null;
  badgeProvider?: Eip1193;
  economicProvider?: Eip1193;
}) {
  const [totals, setTotals] = useState<SponsorTotal[] | null>(null);
  const [splits, setSplits] = useState<SplitEntry[]>([]);
  const [collabs, setCollabs] = useState<CollaboratorInfo[]>([]);
  const [badges, setBadges] = useState<BadgeInfo[]>([]);
  const [err, setErr] = useState("");
  const [badgeRecipient, setBadgeRecipient] = useState("");
  const [badgeReason, setBadgeReason] = useState("");
  const [badgeBusy, setBadgeBusy] = useState(false);
  const [badgeError, setBadgeError] = useState("");
  const [badgeQueryError, setBadgeQueryError] = useState("");
  const [splitDraft, setSplitDraft] = useState<Array<{ address: string; percent: string }>>([]);
  const [splitBusy, setSplitBusy] = useState(false);
  const [splitError, setSplitError] = useState("");
  const [splitTx, setSplitTx] = useState("");

  const draftFromSplits = (values: SplitEntry[]) => values.map((split) => ({
    address: split.address,
    percent: (split.bps / 100).toFixed(2).replace(/\.00$/, "").replace(/(\.\d)0$/, "$1"),
  }));

  const percentageToBps = (value: string): number => {
    const normalized = value.trim();
    if (!/^(?:0|[1-9]\d?)(?:\.\d{1,2})?$|^100(?:\.0{1,2})?$/.test(normalized)) {
      throw new Error(`invalid revenue split percentage: ${value}`);
    }
    const [whole, fraction = ""] = normalized.split(".");
    const bps = Number.parseInt(whole, 10) * 100 + Number.parseInt(fraction.padEnd(2, "0") || "0", 10);
    if (bps <= 0 || bps > 10_000) {
      throw new Error("each revenue split must be between 0.01% and 100%");
    }
    return bps;
  };

  const reloadBadges = async () => {
    try {
      setBadges(await badgesByRepo(cfg, addr, repo));
      setBadgeQueryError("");
    } catch (error) {
      setBadges([]);
      setBadgeQueryError(formatError(error));
    }
  };

  const awardBadge = async (event: FormEvent) => {
    event.preventDefault();
    if (!badgeProvider || !repoId) return;
    setBadgeBusy(true);
    setBadgeError("");
    try {
      await awardBadgeWithEvm(badgeProvider, cfg, repoId, badgeRecipient.trim(), badgeReason);
      setBadgeRecipient("");
      setBadgeReason("");
      await reloadBadges();
    } catch (error) {
      setBadgeError(formatError(error));
    } finally {
      setBadgeBusy(false);
    }
  };

  const saveRevenueSplits = async (event: FormEvent) => {
    event.preventDefault();
    if (!economicProvider || !repoId) return;
    setSplitBusy(true);
    setSplitError("");
    setSplitTx("");
    try {
      const next = splitDraft.map((split) => ({
        address: split.address.trim(),
        bps: percentageToBps(split.percent),
      }));
      const total = next.reduce((sum, split) => sum + split.bps, 0);
      if (total > 10_000) throw new Error(`revenue split total ${total / 100}% exceeds 100%`);
      const txHash = await setRevenueSplitsWithEconomicModule(economicProvider, cfg, repoId, next);
      const refreshed = await revenueSplits(cfg, addr, repo);
      setSplits(refreshed);
      setSplitDraft(draftFromSplits(refreshed));
      setSplitTx(txHash);
    } catch (error) {
      setSplitError(formatError(error));
    } finally {
      setSplitBusy(false);
    }
  };

  useEffect(() => {
    (async () => {
      try {
        const [t, s, c, badgeResult] = await Promise.all([
          sponsorTotals(cfg, addr, repo),
          revenueSplits(cfg, addr, repo),
          listCollaborators(cfg, addr, repo),
          badgesByRepo(cfg, addr, repo).then(
            (value) => ({ value, error: "" }),
            (error) => ({ value: [] as BadgeInfo[], error: formatError(error) }),
          ),
        ]);
        setTotals(t);
        setSplits(s);
        setSplitDraft(draftFromSplits(s));
        setCollabs(c);
        setBadges(badgeResult.value);
        setBadgeQueryError(badgeResult.error);
      } catch (e) {
        setErr(String(e));
      }
    })();
  }, [cfg, addr, repo]);

  if (err) return <div className="error" role="alert">{err}</div>;
  if (!totals) return <div className="spinner" aria-live="polite">querying chain…</div>;

  const splitTotal = splits.reduce((a, s) => a + s.bps, 0);

  return (
    <div>
      <SponsorForm cfg={cfg} addr={addr} repo={repo} repoId={repoId} />

      <div className="dash-section-title">Lifetime sponsorship</div>
      {totals.length === 0 ? (
        <p className="muted">
          no sponsorships yet — be the first: <code>igit sponsor {owner} {repo} 0.1</code>
        </p>
      ) : (
        totals.map((t) => (
          <div key={t.denom} className="card" style={{ marginBottom: 8 }}>
            <b>{formatInj(t.amount, t.denom)}</b> <span className="muted">total received</span>
          </div>
        ))
      )}

      <div className="dash-section-title" style={{ marginTop: 20 }}>Revenue split</div>
      <div className="split-bar" role="img" aria-label={`Revenue split: total allocated ${(splitTotal / 100).toFixed(1)}%, owner receives remainder`}>
        {splits.map((s, i) => (
          <div
            key={s.address}
            style={{ width: `${s.bps / 100}%`, background: BAR_COLORS[i % BAR_COLORS.length] }}
            title={`${s.address} ${s.bps / 100}%`}
          />
        ))}
        <div
          style={{ width: `${(10000 - splitTotal) / 100}%`, background: "#6e7681" }}
          title="owner"
        />
      </div>
      <table className="plain">
        <tbody>
          {splits.map((s, i) => (
            <tr key={s.address}>
              <td>
                <span style={{ color: BAR_COLORS[i % BAR_COLORS.length] }} aria-hidden="true">■</span>
              </td>
              <td>
                <code>{s.address}</code>
              </td>
              <td>{s.bps / 100}%</td>
            </tr>
          ))}
          <tr>
            <td>
              <span style={{ color: "#6e7681" }} aria-hidden="true">■</span>
            </td>
            <td>owner (remainder)</td>
            <td>{(10000 - splitTotal) / 100}%</td>
          </tr>
        </tbody>
      </table>

      {economicProvider && repoId && (
        <form className="split-editor" onSubmit={saveRevenueSplits}>
          <div className="split-editor-head">
            <b>Revenue split recipients</b>
            <span className="muted">{splitDraft.reduce((sum, split) => {
              try {
                return sum + percentageToBps(split.percent);
              } catch {
                return sum;
              }
            }, 0) / 100}% allocated</span>
          </div>
          <div className="split-editor-rows">
            {splitDraft.map((split, index) => (
              <div className="split-editor-row" key={index}>
                <input
                  className="field"
                  value={split.address}
                  onChange={(event) => setSplitDraft((current) => current.map((entry, entryIndex) => (
                    entryIndex === index ? { ...entry, address: event.target.value } : entry
                  )))}
                  placeholder="inj1… recipient"
                  aria-label={`Revenue split recipient ${index + 1}`}
                  disabled={splitBusy}
                  required
                />
                <div className="split-percent-input">
                  <input
                    className="field"
                    type="number"
                    min="0.01"
                    max="100"
                    step="0.01"
                    value={split.percent}
                    onChange={(event) => setSplitDraft((current) => current.map((entry, entryIndex) => (
                      entryIndex === index ? { ...entry, percent: event.target.value } : entry
                    )))}
                    aria-label={`Revenue split percentage ${index + 1}`}
                    disabled={splitBusy}
                    required
                  />
                  <span className="muted">%</span>
                </div>
                <button
                  className="icon-action danger"
                  type="button"
                  onClick={() => setSplitDraft((current) => current.filter((_, entryIndex) => entryIndex !== index))}
                  disabled={splitBusy}
                  title="remove recipient"
                  aria-label={`Remove revenue split recipient ${index + 1}`}
                >
                  <Trash2 size={15} />
                </button>
              </div>
            ))}
          </div>
          <div className="split-editor-actions">
            <button
              type="button"
              onClick={() => setSplitDraft((current) => [...current, { address: "", percent: "" }])}
              disabled={splitBusy || splitDraft.length >= 20}
            >
              <Plus size={15} /> Add recipient
            </button>
            <button className="primary" type="submit" disabled={splitBusy}>
              <Save size={15} /> {splitBusy ? "Saving…" : "Save splits"}
            </button>
          </div>
          {splitError && <div className="error" role="alert">{splitError}</div>}
          {splitTx && (
            <div className="sponsor-ok" aria-live="polite">
              Revenue splits updated in <code>{splitTx.slice(0, 12)}…</code>
            </div>
          )}
        </form>
      )}

      {collabs.length > 0 && (
        <>
          <div className="dash-section-title" style={{ marginTop: 20 }}>Collaborators</div>
          <table className="plain">
            <tbody>
              {collabs.map((c) => (
                <tr key={c.address}>
                  <td><code>{c.address}</code></td>
                  <td>{c.role}</td>
                </tr>
              ))}
            </tbody>
          </table>
        </>
      )}

      {(badges.length > 0 || badgeQueryError) && (
        <>
          <div className="dash-section-title" style={{ marginTop: 20 }}>
            <Award size={14} style={{ verticalAlign: "-2px", marginRight: 4 }} />
            Badges awarded
          </div>
          {badges.map((b) => (
            <div className="card sponsor-entry" key={b.id}>
              <div>
                <b>#{b.id}</b>{" "}
                <span className="muted">
                  to <Link to={`/${b.recipient}`}>{b.recipient.slice(0, 14)}…</Link> ·{" "}
                  {timeAgo(b.awarded_at)}
                </span>
              </div>
              <div className="sponsor-msg">"{b.reason}"</div>
            </div>
          ))}
          {badgeQueryError && (
            <div className="error" role="alert">
              Badge history unavailable: {badgeQueryError}
            </div>
          )}
        </>
      )}

      {badgeProvider && repoId && (
        <form onSubmit={awardBadge} style={{ marginTop: 16 }}>
          <div className="dash-section-title">Award contribution badge</div>
          <div style={{ display: "grid", gridTemplateColumns: "minmax(220px, 1fr) 2fr auto", gap: 8 }}>
            <input
              className="field"
              value={badgeRecipient}
              onChange={(event) => setBadgeRecipient(event.target.value)}
              placeholder="inj1… recipient"
              aria-label="Badge recipient"
              required
            />
            <input
              className="field"
              value={badgeReason}
              onChange={(event) => setBadgeReason(event.target.value)}
              placeholder="Contribution"
              aria-label="Badge reason"
              maxLength={256}
              required
            />
            <button className="btn primary" type="submit" disabled={badgeBusy}>
              <Award size={15} /> {badgeBusy ? "Awarding…" : "Award"}
            </button>
          </div>
          {badgeError && <div className="error" role="alert" style={{ marginTop: 8 }}>{badgeError}</div>}
        </form>
      )}
    </div>
  );
}
