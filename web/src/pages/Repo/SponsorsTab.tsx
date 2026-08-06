import { useEffect, useState } from "react";
import { Link } from "react-router-dom";
import { Award } from "lucide-react";
import {
  badgesByRepo,
  formatFunds,
  formatInj,
  listCollaborators,
  revenueSplits,
  sponsorEvents,
  sponsorTotals,
  timeAgo,
  type AppConfig,
  type BadgeInfo,
  type CollaboratorInfo,
  type SponsorEvent,
  type SponsorTotal,
  type SplitEntry,
} from "../../lib/chain";
import SponsorForm from "./SponsorForm";

const BAR_COLORS = ["#4493f8", "#3fb950", "#d29922", "#a371f7", "#f85149", "#39c5cf"];

export default function SponsorsTab({
  cfg,
  addr,
  repo,
  owner,
}: {
  cfg: AppConfig;
  addr: string;
  repo: string;
  owner: string;
}) {
  const [totals, setTotals] = useState<SponsorTotal[] | null>(null);
  const [splits, setSplits] = useState<SplitEntry[]>([]);
  const [collabs, setCollabs] = useState<CollaboratorInfo[]>([]);
  const [events, setEvents] = useState<SponsorEvent[]>([]);
  const [badges, setBadges] = useState<BadgeInfo[]>([]);
  const [err, setErr] = useState("");

  useEffect(() => {
    (async () => {
      try {
        const [t, s, c, events, badges] = await Promise.all([
          sponsorTotals(cfg, addr, repo),
          revenueSplits(cfg, addr, repo),
          listCollaborators(cfg, addr, repo),
          sponsorEvents(cfg, addr, repo).catch(() => []),
          badgesByRepo(cfg, addr, repo).catch(() => []),
        ]);
        setTotals(t);
        setSplits(s);
        setCollabs(c);
        setEvents(events);
        setBadges(badges);
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
      <SponsorForm cfg={cfg} addr={addr} repo={repo} />

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

      {events.length > 0 && (
        <>
          <div className="dash-section-title" style={{ marginTop: 20 }}>Sponsor wall</div>
          {events.map((e) => (
            <div className="card sponsor-entry" key={e.txhash}>
              <div>
                <b>{formatFunds(e.funds)}</b>{" "}
                <span className="muted">
                  from <code>{e.sponsor.slice(0, 14)}…</code> ·{" "}
                  {e.timestamp.slice(0, 16).replace("T", " ")}
                </span>
              </div>
              {e.message && <div className="sponsor-msg">"{e.message}"</div>}
            </div>
          ))}
        </>
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

      {badges.length > 0 && (
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
        </>
      )}
    </div>
  );
}
