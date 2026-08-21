# Public Monitor Dashboard

## Product Decision

`Monitor` is a public, read-only page for visitors to inspect the observable
health and activity of the igit project. The first release is intentionally
smaller than an internal operations console:

- the main page uses a live observation window for EVM activity and network
  status;
- the CosmWasm V1 archive appears only as an independent read-only source
  status with links to its archive viewer and public contract explorer;
- EVM V2 and CosmWasm V1 remain separate data sources and are never presented
  as one unqualified total;
- a redacted public storage-topology strip identifies the HK/US IPFS gateways
  and the Filebase/Fil.one provider roles without exposing operational metrics;
- every metric carries its source and observation scope, and the page shows
  when the data was last refreshed;
- the Monitor has no wallet writes, migration execution, or administrative
  controls.

The public route is `#/monitor` in the current hash-based Web application.
Internal infrastructure metrics remain out of scope for this route and belong
in a future protected `/monitor/ops` surface. The public route may show
curated endpoint inventory (region, role, and public URL) when it does not
claim live uptime or capacity.

## Audience And Non-Goals

The audience is any visitor, including users who have not connected a wallet.
The page must remain useful when the EVM V2 `SuiteDirectory` is not configured
or has not passed verification; in that state it reports the EVM source as
unavailable while still allowing visitors to open the independent V1 archive.

The first release does not claim to provide:

- an all-time or globally complete event count;
- a complete EVM repository or owner count without a global indexer;
- a V1 repository list or cross-owner aggregate repository data;
- a historical activity time series;
- a migration-completed count;
- infrastructure uptime or storage capacity guarantees;
- a replacement for the explicit `/archive/cosmwasm-v1` repository viewer.

## Data Source Boundaries

| Source | Public use in Monitor | Required label | Current limitation |
| --- | --- | --- | --- |
| Verified EVM V2 `SuiteDirectory` | Suite status, latest block, recent decoded activity, and any repository data reachable through the verified Directory | `EVM V2` | The checked-in profile has no real Directory yet. Activity is a bounded recent-block observation, not an all-time index. |
| CosmWasm V1 archive contract | Contract availability, session snapshot height, and links to the explicit archive viewer and public Injective Explorer | `CosmWasm V1 archive` and `Read-only` | Repository lookup remains in the dedicated archive viewer. The first Monitor release does not enumerate or aggregate V1 repositories. |
| IPFS gateway and Git pack audit | Independent reachability probes for the HK hot-tier and US durable-archive gateways, plus packfile health for indexed repositories/CIDs | `IPFS observation` | Each gateway uses three consecutive direct-browser `/ipfs/` probes and displays the median response latency. Results describe the visitor's current ISP path, not global performance. |
| Curated storage topology | Public HK/US gateway inventory plus Filebase and Fil.one provider roles and public endpoints | `IPFS gateways` and `Public inventory` | Provider badges describe the configured role only; browser code does not query private S3 metrics or credentials. |
| Future event/indexer API | Complete historical totals, daily trends, unique owners/contributors, and cross-owner migration counts | `Indexed data` | Not part of the first release. The API must expose its index height and freshness. |

The Monitor must not treat a CosmWasm contract address as an EVM
`SuiteDirectory`, and it must not silently fall back from an EVM verification
or transport error to the V1 archive.

The public Monitor probes each statically configured gateway's `/ipfs/` path
directly from the visitor's browser. HK and US start concurrently; each
gateway runs three sequential requests and reports their median HTTP response
latency. This intentionally reflects the visitor's current ISP and route, so
different networks may produce different regional rankings. A `403` from
`/healthz` is not used as a gateway signal: that path is an operational health
route, while `/ipfs/` is the public read-only gateway path.

The same-origin `/api/ipfs-health` function remains available for operational
server-side reachability checks. It uses a static network-profile and target-ID
allowlist (`target=hk` and `target=us`) and rejects arbitrary `gateway=` URL
selection, but Monitor does not use its latency as visitor latency. Monitor
runs the full source and HK/US gateway probe cycle when the page opens and
every 90 seconds thereafter; visitors can also trigger the same cycle
manually.

## First-Release Screen

### Status Strip

The top of the page shows four compact status items:

1. **EVM V2 Suite** — `Verified`, `Not configured`, `Verification failed`, or
   `Stale`, with chain ID and latest block when available.
2. **CosmWasm V1 archive** — `Online`, `Read-only`, `Unavailable`, or `Stale`,
   with the session snapshot height and shortened contract address.
3. **IPFS observation** — paired HK/US gateway statuses (`Reachable`,
   `Degraded`, or `Unknown`) and three-sample browser median latency, based
   only on probes that actually ran.
4. **Data freshness** — the newest successful query time and the scope of the
   observation window.

An unavailable source is an explicit state, not a zero. Cards and tables must
not display a numeric value when the source did not answer successfully.

### Overview KPI Cards

The first row uses scope-accurate labels:

- **Recent EVM activity** — number of decoded events in the configured recent
  block window, with the first and last block shown in a tooltip.
- **Observed EVM repositories** — only when a verified Directory-backed
  enumeration is available; otherwise show `Not indexed` rather than an
  invented total.
- **Latest observed block** — the newest EVM block used by the current
  observation, or `Unavailable` when the EVM source cannot be verified.
- **IPFS reachability** — reachable indexed pack/CID checks divided by checks
  completed in the same observation run. Show `No probe data` when no checks
  have run.

The UI must avoid ambiguous labels such as `Total repositories`, `All-time
activity`, or `Network uptime` until a supporting indexer or metrics service is
available.

### Overview Sections

The Overview tab contains:

- a small EVM activity sparkline or bar series with the observation window in
  the caption;
- a health panel for the three public data sources;
- a list of recently observed EVM activity or repositories when the verified
  Suite supports it;
- a V1 archive status item linking to its explicit archive viewer and public
  Injective Explorer, without listing V1 repositories in Monitor;
- a short data-source note linking to the relevant Explorer or archive route.

The first implementation may use CSS/SVG sparklines. A chart dependency is
not required for the MVP.

### Activity Tab

The Activity table displays at most the recent bounded result set and includes:

- action (`create_repo`, `update_ref`, `sponsor`, `award_badge`, moderation,
  and other decoded actions supported by the source);
- source badge (`EVM V2` in the first release);
- owner and repository;
- actor, shortened by default;
- block/height and relative time;
- transaction hash when one exists;
- success, unavailable, or stale state.

The table must say `Recent observation` rather than implying a complete event
history. Every row links to the most specific available explorer, repository,
or archive page.

### Storage Tab

The public Storage surface (the topology strip and Storage tab) is an aggregate
view of completed checks, not a server console. It may show:

- the HK and US public gateway nodes plus Filebase and Fil.one provider roles
  when each entry is explicitly marked as
  topology metadata rather than an uptime claim;
- indexed pack/CID count;
- reachable and unreachable counts;
- reachability percentage;
- last successful probe time;
- optional p50/p95 gateway latency when a public probe supplies it;
- repositories with missing or unreachable pack URIs.

The tab must not expose private Kubo addresses, filesystem paths, bucket names,
credentials, replication queue internals, or exact internal alert thresholds.
Public gateway/provider URLs may be linked when they are already intended for
read-only external access.

### Migration Tab

The Migration tab explains the V1 boundary and gives visitors an actionable
read-only view:

- `Legacy CosmWasm V1 read-only snapshot`;
- snapshot height and source contract;
- links to the independent V1 archive viewer and public contract explorer;
- `Migration required` and `Migration unavailable` status.

The page must not show `Migrated`, `Imported`, or `Migration complete` unless a
reviewed EVM import record and verifiable on-chain evidence exist. The current
product has no open migration action, so the control is informational and
disabled. Monitor does not list V1 repositories or aggregate their data in the
first release;
repository browsing remains under `#/archive/cosmwasm-v1`.

## Public Data And Privacy Rules

Public chain addresses and transaction hashes may be linked, but the default
display uses shortened values. The Monitor should prefer aggregate counts and
repository names over prominent personal identity details. It must never expose
private keys, object-storage credentials, internal email settings, hostnames,
filesystem paths, or raw operational logs.

Moderation data shown publicly must be limited to the status already exposed by
the repository/archive view. Unpublished report text, reviewer identity, and
internal enforcement notes remain out of scope.

## Freshness And Failure Semantics

Every source-backed value includes:

- source name;
- query/index snapshot height or block range;
- `updated_at` or `generated_at`;
- stale threshold used by the UI;
- a link or tooltip explaining the scope.

If a query fails, the UI keeps the last successful value only when it can show
the value's age and marks it `Stale`. Otherwise it shows `Unavailable`. A
failed EVM Suite verification must not hide or corrupt the independent V1
archive section.

## Later Indexer And Ops Work

An indexer or cache API is required before publishing:

- all-time repository/owner totals;
- daily or weekly activity history;
- unique contributors;
- all-time sponsorship volume;
- complete cross-owner V1 migration backlog;
- accurate 30-day growth percentages.

A separate protected `/monitor/ops` surface may later consume a redacted
metrics API for Kubo, CAR archive, durable CID sync, replication, queue, and
gateway latency health. The public Monitor must not read server-local files or
Prometheus textfiles directly from a browser.

## Acceptance Criteria For The MVP

- A visitor can open `#/monitor` without connecting a wallet.
- EVM V2, V1 archive, and IPFS values have distinct source badges.
- The V1 status links to the explicit archive route and is visibly read-only.
- Monitor does not render a V1 repository list or cross-owner aggregate
  repository data in the first release.
- Missing EVM `SuiteDirectory` produces a clear `Not configured` state, not a
  fake zero and not an implicit V1 fallback.
- Activity counts state their block window and never claim to be all-time.
- Failed or stale queries have explicit states and do not become numeric zero.
- The HK and US IPFS gateways appear in the same gateway section and retain
  independent reachability, three-sample browser median latency, probe-source,
  and freshness states.
- Filebase and Fil.one remain clearly labelled provider inventory; they are
  not presented as IPFS gateway probes or as live storage-capacity metrics.
- No Monitor control signs, broadcasts, edits, sponsors, transfers, or starts
  migration.
- The public topology strip contains no bucket name, private address, secret,
  credential, or server-local metric.
