import { useCallback, useEffect, useMemo, useRef, useState } from "react";
import L from "leaflet";
import "leaflet/dist/leaflet.css";
import {
  Activity,
  Archive as ArchiveIcon,
  Box,
  Clock3,
  Cloud,
  Database,
  Map as MapIcon,
  RefreshCw,
  Server,
} from "lucide-react";
import { Badge } from "../components/ui/badge";
import {
  CONFIG_CHANGED_EVENT,
  isSuiteDirectoryConfigured,
  loadConfig,
  networkProfile,
} from "../lib/profile";
import {
  collectStorageStats,
  latestEvmBlock,
  loadCachedStorageStats,
  saveCachedStorageStats,
  type StorageStats,
} from "../lib/storage-stats";
import { AMAP_KEY, loadAMap, wgsToGcj } from "../lib/amap";
import {
  contractActivity,
  EVM_ACTIVITY_BLOCK_WINDOW,
  formatError,
  verifySuite,
  type ContractTx,
} from "../lib/chain";
import { prepareCosmWasmV1Snapshot } from "../lib/cosmwasm-v1";
import { BROWSER_GATEWAY_PROBE_SAMPLE_COUNT, probeGatewayFromBrowser } from "../lib/ipfs-probe";
import { useWallet } from "../lib/WalletContext";
import type {
  IpfsGatewayDefinition,
  IpfsGatewayId,
  IpfsGatewaySnapshot,
  PublicStorageProvider,
  SourceSnapshot,
  SourceState,
} from "../lib/types";
import { SourceCard } from "./Monitor/SourceCard";
import { MetricCard } from "./Monitor/MetricCard";
import { IpfsGatewayCard } from "./Monitor/IpfsGatewayCard";
import { PublicStorageProviderCard } from "./Monitor/PublicStorageProviderCard";
import { formatBlock, formatCheckedAt } from "./Monitor/utils";

// ---------------------------------------------------------------------------
// Infrastructure tour. The map cycles through the storage buckets, IPFS
// nodes, and gateway servers the igit client can reach. Coordinates are
// representative city positions for the board (public endpoint facts come
// from the Monitor page; BYOS rows show example bucket platforms).
// ---------------------------------------------------------------------------

type InfraKind = "ipfs-gateway" | "s3-provider" | "byos-bucket";

type InfraEdge = { name: string; latlng: [number, number] };

type InfraPoint = {
  id: string;
  name: string;
  kind: InfraKind;
  endpoint: string;
  description: string;
  latlng: [number, number];
  // Anycat-style points light up every edge city while they are the active
  // tour stop; the main dot anchors the region summary.
  edges?: readonly InfraEdge[];
};

const KIND_META: Record<InfraKind, { label: string; color: string }> = {
  "ipfs-gateway": { label: "IPFS gateway", color: "#4f46e5" },
  "s3-provider": { label: "S3 storage", color: "#b45309" },
  "byos-bucket": { label: "BYOS bucket", color: "#0369a1" },
};

const INFRA_POINTS: readonly InfraPoint[] = [
  {
    id: "hk-gateway",
    name: "Hong Kong IPFS gateway",
    kind: "ipfs-gateway",
    endpoint: "https://igit-hk.haohanyh.ovh",
    description: "Hot-tier read-only pack gateway.",
    latlng: [22.3193, 114.1694],
  },
  {
    id: "filebase",
    name: "Filebase bucket",
    kind: "s3-provider",
    endpoint: "https://s3.filebase.com",
    description: "External S3-compatible legacy replica · Northern Virginia, US East.",
    latlng: [38.95, -77.2],
  },
  {
    id: "filone",
    name: "Fil.one bucket",
    kind: "s3-provider",
    endpoint: "https://us-east-1.s3.fil.one",
    description: "External S3-compatible CAR archive path · Detroit, Michigan.",
    latlng: [42.3314, -83.0458],
  },
  {
    id: "r2",
    name: "Cloudflare R2 (BYOS example)",
    kind: "byos-bucket",
    endpoint: "api.cloudflare.com · anycast",
    description: "Suite v4 BYOS bucket platform · APAC anycast edge.",
    latlng: [19.8, 108.2],
    edges: [
      { name: "Tokyo", latlng: [35.6762, 139.6503] },
      { name: "Seoul", latlng: [37.5665, 126.978] },
      { name: "Osaka", latlng: [34.6937, 135.5023] },
      { name: "Mumbai", latlng: [19.076, 72.8777] },
      { name: "Singapore", latlng: [1.3521, 103.8198] },
      { name: "Jakarta", latlng: [-6.2088, 106.8456] },
      { name: "Ningxia", latlng: [38.4872, 106.2309] },
      { name: "Pune", latlng: [18.5204, 73.8567] },
    ],
  },
];

const TOUR_INTERVAL_MS = 45_000;
const TOUR_ZOOM = 7;
// Grace window after a user marker click before the auto-tour moves on;
// every further click re-arms it.
const TOUR_CLICK_PAUSE_MS = 10_000;

// The marquee re-runs the Monitor page's read-only probes on the Monitor
// page's own refresh cadence.
const PROBE_REFRESH_INTERVAL_MS = 90_000;

// Latency labels and status badges only render once the map is zoomed in far
// enough that neighboring annotations cannot collide (world view stays clean).
const LABEL_ZOOM = 6;

// A gateway pin counts as online when a browser probe reached it and the
// median latency stayed under this ceiling.
const GATEWAY_ONLINE_LATENCY_MS = 5_000;

// The chain currently has a single suite owner; its Owner page is where the
// wallet-gated popup link points. With more observed owners the popup shows
// the observed owner count instead of a direct link.
const OBSERVED_OWNER_PATH = "/inj1sh4v00qgzjy25a73mqheew8q200punaglrzec5";

// The popup lists at most this many repository rows before the "others" line.
const POPUP_REPO_ROW_LIMIT = 10;

// On-demand endpoint pings for the map pins. A pin is probed only while its
// stop is the active tour target (or was just clicked); nothing polls the
// other endpoints in the background. Each run takes a few no-cors HEAD
// samples (DNS + TCP + TLS + headers; the opaque body is never read, so the
// endpoints' CORS headers don't matter). Successful samples join a small
// rolling window whose AVERAGE is displayed, and each entry stays valid for
// POINT_PROBE_TTL_MS. Entries are persisted, so reopening the page inside
// the window keeps the last average without touching the network.
const POINT_PROBE_TIMEOUT_MS = 8_000;
const POINT_PROBE_SAMPLES = 3;
const POINT_PROBE_SAMPLE_CAP = 9;
const POINT_PROBE_TTL_MS = 180_000;
const POINT_PROBE_STORAGE_KEY = "igit-mapmonitor-point-probe";

const POINT_PROBE_TARGETS: readonly { id: string; gateway: boolean; url: string }[] = [
  { id: "hk-gateway", gateway: true, url: "https://igit-hk.haohanyh.ovh" },
  { id: "filebase", gateway: false, url: "https://s3.filebase.com" },
  { id: "filone", gateway: false, url: "https://s3.us-east-1.filonecontent.com" },
];

const POINT_PROBE_TARGET_MAP = new Map(POINT_PROBE_TARGETS.map((target) => [target.id, target]));

interface PointProbeEntry {
  /** Recent successful latencies (capped); the pin shows their average. */
  samples: number[];
  /** Whether the latest run reached the endpoint at least once. */
  reachable: boolean;
  /** When the latest run completed; entries expire after POINT_PROBE_TTL_MS. */
  sampledAt: number;
}

function averageLatency(samples: readonly number[]): number | null {
  if (samples.length === 0) return null;
  return Math.round(samples.reduce((sum, value) => sum + value, 0) / samples.length);
}

async function probeEndpointLatency(url: string): Promise<number> {
  const controller = new AbortController();
  const timer = setTimeout(() => controller.abort(), POINT_PROBE_TIMEOUT_MS);
  const started = performance.now();
  try {
    await fetch(url, { method: "HEAD", mode: "no-cors", cache: "no-store", signal: controller.signal });
    return Math.max(0, Math.round(performance.now() - started));
  } finally {
    clearTimeout(timer);
  }
}

async function samplePointLatency(url: string): Promise<number[]> {
  const samples: number[] = [];
  for (let index = 0; index < POINT_PROBE_SAMPLES; index += 1) {
    try {
      samples.push(await probeEndpointLatency(url));
    } catch {
      // Unreachable sample; reachability needs only one success.
    }
  }
  return samples;
}

function loadPointProbeCache(): Record<string, PointProbeEntry> {
  try {
    const parsed = JSON.parse(localStorage.getItem(POINT_PROBE_STORAGE_KEY) ?? "{}") as Record<string, PointProbeEntry>;
    const clean: Record<string, PointProbeEntry> = {};
    for (const [id, entry] of Object.entries(parsed)) {
      if (entry == null || typeof entry !== "object" || typeof entry.sampledAt !== "number") continue;
      const samples = Array.isArray(entry.samples)
        ? entry.samples.filter((value) => typeof value === "number" && Number.isFinite(value))
        : [];
      clean[id] = { samples, reachable: entry.reachable === true, sampledAt: entry.sampledAt };
    }
    return clean;
  } catch {
    return {};
  }
}

function savePointProbeCache(record: Record<string, PointProbeEntry>): void {
  try {
    localStorage.setItem(POINT_PROBE_STORAGE_KEY, JSON.stringify(record));
  } catch {
    // Storage may be unavailable (private mode); caching is best-effort.
  }
}

// Per-point live observation shown on the map pins.
type PointLiveStatus = { online: boolean | null; latencyMs: number | null };

const UNKNOWN_STATUS: PointLiveStatus = { online: null, latencyMs: null };

function dotHtml(kind: InfraKind, active: boolean): string {
  const color = KIND_META[kind].color;
  return active
    ? '<span class="mapmonitor-dot lit"></span>'
    : `<span class="mapmonitor-dot" style="--dot-color: ${color}"></span>`;
}

function edgeDotHtml(): string {
  return '<span class="mapmonitor-dot edge lit"></span>';
}

// Pin = kind dot with a live meta row underneath: latency label first, then
// the online/offline/unknown status dot to its right. The wrapper keeps a
// fixed 16px box so marker anchoring stays centered on the dot while the meta
// row overflows below. The meta row stays in the DOM but is hidden below
// LABEL_ZOOM via the `mapmonitor-zoom-labels` container class each engine
// toggles on zoom changes.
function pinHtml(kind: InfraKind, active: boolean, status: PointLiveStatus): string {
  const badge = status.online == null ? "unknown" : status.online ? "online" : "offline";
  const latency = status.latencyMs != null
    ? `<span class="mapmonitor-latency">${Math.round(status.latencyMs)}ms</span>`
    : "";
  return `<span class="mapmonitor-pin-wrap">${dotHtml(kind, active)}<span class="mapmonitor-pin-meta">${latency}<span class="mapmonitor-status ${badge}"></span></span></span>`;
}

// HTML dot markers instead of SVG circleMarkers: icons are counter-scaled by
// Leaflet during zoom/flyTo animations, so their pixel size stays constant
// while the map pane is being scaled. SVG circles would visually balloon.
function dotIcon(kind: InfraKind, active: boolean, status: PointLiveStatus): L.DivIcon {
  return L.divIcon({
    className: "mapmonitor-pin",
    html: pinHtml(kind, active, status),
    iconSize: [16, 16],
    iconAnchor: [8, 8],
  });
}

// Smaller companion dot for anycast edge cities; every edge lights up red
// and pulses while its parent point is the active tour stop (the parent's
// main dot then keeps its normal kind color).
function edgeIcon(): L.DivIcon {
  return L.divIcon({
    className: "mapmonitor-pin",
    html: edgeDotHtml(),
    iconSize: [12, 12],
    iconAnchor: [6, 6],
  });
}

// ---------------------------------------------------------------------------
// Live marquee. The card components are the exact ones rendered by the Monitor
// page, and every snapshot comes from the same read-only collection functions
// the Monitor page runs: verifySuite, eth_blockNumber, contractActivity,
// browser gateway probes, and the CosmWasm V1 archive snapshot. Metadata
// below mirrors the Monitor page's public gateway/provider inventories.
// ---------------------------------------------------------------------------

const MARQUEE_GATEWAYS: readonly IpfsGatewayDefinition[] = [
  {
    id: "hk",
    title: "Hong Kong gateway",
    description: "Read-only IPFS gateway for the hot tier.",
    region: "Hong Kong",
    endpoint: "https://igit-hk.haohanyh.ovh",
    href: "https://igit-hk.haohanyh.ovh/ipfs/",
    icon: Server,
  },
  {
    id: "us",
    title: "US gateway",
    description: "Read-only IPFS gateway for the durable archive.",
    region: "United States",
    endpoint: "https://igit-us.haohanyh.ovh",
    href: "https://igit-us.haohanyh.ovh/ipfs/",
    icon: Server,
  },
];

const MARQUEE_PROVIDERS: readonly PublicStorageProvider[] = [
  {
    id: "filebase",
    title: "Filebase",
    description: "External S3-compatible copy retained for recovery.",
    kind: "Storage provider",
    region: "External provider",
    role: "Legacy replica",
    endpoint: "https://s3.filebase.com",
    href: "https://s3.filebase.com",
    status: "legacy",
    icon: Database,
  },
  {
    id: "filone",
    title: "Fil.one",
    description: "External S3-compatible provider for CAR archives.",
    kind: "Storage provider",
    region: "US East",
    role: "Current archive path",
    endpoint: "https://us-east-1.s3.fil.one",
    href: "https://us-east-1.s3.fil.one",
    status: "current",
    icon: Cloud,
  },
];

// Marquee probe state: each source lands independently, so one slow or failed
// probe never blocks the other cards (a failed card keeps its last value or
// falls back to "—").
type ProbeSnapshotState = {
  evm: SourceSnapshot;
  evmV4: SourceSnapshot;
  v1: SourceSnapshot & { snapshotHeight: number | null };
  ipfs: Record<IpfsGatewayId, IpfsGatewaySnapshot>;
  latestBlock: bigint | null;
  activityCount: number | null;
  activityError: string;
  lastRefresh: number | null;
  refreshing: boolean;
};

function initialGatewaySnapshots(): Record<IpfsGatewayId, IpfsGatewaySnapshot> {
  return Object.fromEntries(
    MARQUEE_GATEWAYS.map((gateway) => [gateway.id, {
      state: "loading",
      detail: "Checking gateway",
      checkedAt: null,
      latencyMs: null,
      statusCode: null,
      probeSource: null,
      sampleCount: BROWSER_GATEWAY_PROBE_SAMPLE_COUNT,
      responseSamples: 0,
      reachableSamples: 0,
    }]),
  ) as Record<IpfsGatewayId, IpfsGatewaySnapshot>;
}

const INITIAL_PROBE: ProbeSnapshotState = {
  evm: { state: "loading", detail: "Checking SuiteDirectory", checkedAt: null },
  evmV4: { state: "loading", detail: "Checking successor SuiteDirectory", checkedAt: null },
  v1: { state: "loading", detail: "Checking archive endpoint", checkedAt: null, snapshotHeight: null },
  ipfs: initialGatewaySnapshots(),
  latestBlock: null,
  activityCount: null,
  activityError: "",
  lastRefresh: null,
  refreshing: true,
};

// One copy of every Monitor page card, in Monitor page order, fed by the live
// probe state. Skeletons only cover the first collection round; later rounds
// keep the last values while they refresh in the background.
function MarqueeGroup({
  probe,
  profileLabel,
  hidden = false,
}: {
  probe: ProbeSnapshotState;
  profileLabel: string;
  hidden?: boolean;
}) {
  const firstLoad = probe.lastRefresh == null;
  return (
    <div className="mapmonitor-marquee-group" aria-hidden={hidden || undefined} aria-label="Live monitor cards">
      <div className="mapmonitor-marquee-item">
        <SourceCard
          icon={Server}
          title="EVM V2/V3 Suite"
          description="Verified Directory trust root"
          source={probe.evm}
        />
      </div>
      <div className="mapmonitor-marquee-item">
        <SourceCard
          icon={Database}
          title="EVM V4"
          description="Successor suite · BYOS storage buckets"
          source={probe.evmV4}
        />
      </div>
      <div className="mapmonitor-marquee-item">
        <SourceCard
          icon={ArchiveIcon}
          title="CosmWasm V1 archive"
          description="Independent historical read-only surface"
          source={probe.v1}
        />
      </div>
      {MARQUEE_GATEWAYS.map((gateway) => (
        <div className="mapmonitor-marquee-item" key={gateway.id}>
          <IpfsGatewayCard gateway={gateway} snapshot={probe.ipfs[gateway.id]} />
        </div>
      ))}
      {MARQUEE_PROVIDERS.map((provider) => (
        <div className="mapmonitor-marquee-item" key={provider.id}>
          <PublicStorageProviderCard provider={provider} />
        </div>
      ))}
      <div className="mapmonitor-marquee-item">
        <MetricCard
          icon={Activity}
          label="Recent activity"
          value={probe.evm.state === "healthy" && probe.activityCount != null ? String(probe.activityCount) : "—"}
          detail={
            probe.evm.state === "healthy"
              ? `up to 50 events · ${EVM_ACTIVITY_BLOCK_WINDOW.toLocaleString()} blocks`
              : probe.activityError || "EVM Suite unavailable"
          }
          loading={firstLoad && probe.activityCount == null}
        />
      </div>
      <div className="mapmonitor-marquee-item">
        <MetricCard
          icon={Box}
          label="Latest observed block"
          value={formatBlock(probe.latestBlock)}
          detail={profileLabel}
          loading={firstLoad && probe.latestBlock == null}
        />
      </div>
      <div className="mapmonitor-marquee-item">
        <MetricCard
          icon={Server}
          label="Observation window"
          value={`${(EVM_ACTIVITY_BLOCK_WINDOW / 1000).toFixed(0)}k blocks`}
          detail="EVM activity sample"
        />
      </div>
      <div className="mapmonitor-marquee-item">
        <MetricCard
          icon={Clock3}
          label="Last refresh"
          value={formatCheckedAt(probe.lastRefresh)}
          detail={`auto refresh · ${PROBE_REFRESH_INTERVAL_MS / 1000}s`}
          loading={firstLoad}
        />
      </div>
    </div>
  );
}

// Maps live storage observations onto the tour endpoints: the HK gateway
// row reports the whole v3 IPFS layer (both gateways serve the same packs),
// bucket rows key off their manifest location hosts, and R2 reports every
// cloudflare-r2 location observed across user BYOS buckets.
function liveBoardRow(
  id: string,
  data: StorageStats | null,
): { packs: number | null; repos: number | null } {
  if (data == null) return { packs: null, repos: null };
  switch (id) {
    case "hk-gateway":
      return { packs: data.packsByProvider.ipfs, repos: data.reposByProvider.ipfs };
    case "filebase":
      return {
        packs: data.packsByHost["s3.filebase.com"] ?? 0,
        repos: data.reposByHost["s3.filebase.com"] ?? 0,
      };
    case "filone":
      return {
        packs: data.packsByHost["us-east-1.s3.fil.one"] ?? 0,
        repos: data.reposByHost["us-east-1.s3.fil.one"] ?? 0,
      };
    case "r2":
      return {
        packs: data.packsByProvider["cloudflare-r2"],
        repos: data.reposByProvider["cloudflare-r2"],
      };
    default:
      return { packs: null, repos: null };
  }
}

// "Cached 3h ago" label for board results served from the week cache.
function sampleAgeLabel(sampledAt: number): string {
  const minutes = Math.max(1, Math.round((Date.now() - sampledAt) / 60_000));
  if (minutes < 60) return `${minutes}m ago`;
  const hours = Math.round(minutes / 60);
  if (hours < 24) return `${hours}h ago`;
  return `${Math.round(hours / 24)}d ago`;
}

// Repo-name samples behind the popup list for each tour endpoint (same keying
// as liveBoardRow): the HK stop reports the v3 IPFS layer, bucket rows key off
// their manifest location hosts, and R2 reports every cloudflare-r2 location.
// Names come from the bounded board walk; totals come from its aggregate
// counts, so the popup can render "+ N others" without extra requests. The
// optional chains keep week-old cached stats (saved before the name fields
// existed) rendering as an empty list instead of crashing.
function livePointRepos(
  id: string,
  data: StorageStats | null,
): { names: readonly string[]; total: number | null } {
  if (data == null) return { names: [], total: null };
  switch (id) {
    case "hk-gateway":
      return { names: data.repoNamesByProvider?.ipfs ?? [], total: data.reposByProvider.ipfs };
    case "filebase":
      return {
        names: data.repoNamesByHost?.["s3.filebase.com"] ?? [],
        total: data.reposByHost["s3.filebase.com"] ?? 0,
      };
    case "filone":
      return {
        names: data.repoNamesByHost?.["us-east-1.s3.fil.one"] ?? [],
        total: data.reposByHost["us-east-1.s3.fil.one"] ?? 0,
      };
    case "r2":
      return {
        names: data.repoNamesByProvider?.["cloudflare-r2"] ?? [],
        total: data.reposByProvider["cloudflare-r2"],
      };
    default:
      return { names: [], total: null };
  }
}

// ---------------------------------------------------------------------------
// Map engines. Both render the same tour: colored kind dots (plus live
// status badges and gateway latency labels from LABEL_ZOOM up), a red lit
// stop (or, for anycast stops, hidden main + lit red edge cities), an info
// popup, and an eased flight. Leaflet/OpenStreetMap stays as the key-less
// dev fallback; production uses AMap/Gaode (GCJ-02, mainland-safe borders).
// ---------------------------------------------------------------------------

type MapEngine = {
  showStop(point: InfraPoint): void;
  /** Re-render marker pins and open popup content from current callbacks. */
  refresh(): void;
  destroy(): void;
};

// Engines are created once per mount; the callbacks are read through refs so
// fresh probe data and wallet state reach the live pins without rebuilding
// the engine.
type EngineCallbacks = {
  onStopClick: (index: number) => void;
  buildPopup: (point: InfraPoint) => string;
  statusFor: (point: InfraPoint) => PointLiveStatus;
};

function popupShell(inner: string): string {
  return `<div class="mapmonitor-popup">${inner}</div>`;
}

// Repository names come from chain data and land in innerHTML popups; escape
// them before interpolation.
function escapeHtml(value: string): string {
  return value
    .replaceAll("&", "&amp;")
    .replaceAll("<", "&lt;")
    .replaceAll(">", "&gt;")
    .replaceAll('"', "&quot;")
    .replaceAll("'", "&#39;");
}

/* eslint-disable @typescript-eslint/no-explicit-any */
// AMap (Gaode) engine: JS API 2.0, HTML content markers keep the shared dot
// styles (pulse included); coordinates convert WGS-84 -> GCJ-02. AMap arrays
// are [lng, lat] while our tour data is [lat, lng].
function createAMapEngine(A: any, element: HTMLElement, cb: EngineCallbacks): MapEngine {
  const pos = (latlng: [number, number]): [number, number] => {
    const [lat, lng] = wgsToGcj(latlng[0], latlng[1]);
    return [lng, lat];
  };

  // English basemap. AMap JS API 2.0's language switches (the documented
  // `lang` Map option and the internal `languageCode`) only work through the
  // multi-language VECTOR tiles, which the server gates behind key privileges
  // (web_map/get_tile answers infocode 10012 INSUFFICIENT_PRIVILEGES for this
  // key, for both 'en' and 'zh_en'). The classic raster tiles instead carry
  // the language in the URL and need no privilege, so the vector base is
  // replaced with lang=en raster tiles (verified: same tile differs from
  // lang=zh_cn). They are GCJ-02 like the vector base, so marker positions
  // keep matching. The raster map stays light in both app themes (same as the
  // OSM dev fallback); dark vector styling cannot show English labels without
  // a privileged key.
  const englishBase = new A.TileLayer({
    tileUrl:
      "https://wprd0{1,2,3,4}.is.autonavi.com/appmaptile?x=[x]&y=[y]&z=[z]&lang=en&size=1&scale=1&style=7",
    zooms: [3, 18],
  });

  const map = new A.Map(element, {
    zoom: 3,
    center: pos([20, 10]),
    viewMode: "2D",
    zooms: [3, 18],
    layers: [englishBase],
  });

  // isCustom:true removes AMap's default white InfoWindow shell; our own
  // .mapmonitor-popup class uses CSS variables that flip with the app theme.
  const info = new A.InfoWindow({ isCustom: true, anchor: "bottom-center", offset: new A.Pixel(0, -4), autoMove: false });
  let infoTimer = 0;
  let activePointId = "";
  let openPoint: InfraPoint | null = null;

  // Latency labels + status badges appear only from LABEL_ZOOM in; listen to
  // both zoomchange (live during the gesture) and zoomend (settled value).
  const syncLabels = () => {
    element.classList.toggle("mapmonitor-zoom-labels", map.getZoom() >= LABEL_ZOOM);
  };
  map.on("zoomchange", syncLabels);
  map.on("zoomend", syncLabels);
  syncLabels();

  // isCustom also removes AMap's built-in dismiss-on-map-click behavior;
  // clicking empty map space closes the popup manually.
  map.on("click", () => {
    openPoint = null;
    info.close();
  });

  const mainMarkers = new Map<string, any>();
  for (const [index, point] of INFRA_POINTS.entries()) {
    const marker = new A.Marker({
      position: pos(point.latlng),
      content: pinHtml(point.kind, false, cb.statusFor(point)),
      anchor: "center",
      title: point.name,
      cursor: "pointer",
    });
    marker.on("click", () => cb.onStopClick(index));
    marker.setMap(map);
    mainMarkers.set(point.id, marker);
  }

  const edgeMarkers = new Map<string, any[]>();
  for (const point of INFRA_POINTS) {
    if (point.edges == null) continue;
    edgeMarkers.set(
      point.id,
      point.edges.map((edge) => {
        const marker = new A.Marker({
          position: pos(edge.latlng),
          content: edgeDotHtml(),
          anchor: "center",
          title: `${point.name} · ${edge.name}`,
          cursor: "pointer",
        });
        marker.on("click", () => {
          info.setContent(popupShell(`<strong>${edge.name}</strong><br/>${point.name} edge`));
          info.open(map, marker.getPosition());
        });
        return marker;
      }),
    );
  }

  return {
    showStop(point: InfraPoint) {
      activePointId = point.id;
      const hasEdges = point.edges != null && point.edges.length > 0;
      for (const markers of edgeMarkers.values()) {
        for (const marker of markers) marker.setMap(null);
      }
      if (hasEdges) {
        for (const marker of edgeMarkers.get(point.id) ?? []) marker.setMap(map);
      }
      for (const candidate of INFRA_POINTS) {
        const marker = mainMarkers.get(candidate.id);
        if (marker == null) continue;
        const active = candidate.id === point.id;
        if (active && hasEdges) {
          marker.setMap(null);
          continue;
        }
        if (marker.getMap() == null) marker.setMap(map);
        marker.setContent(pinHtml(candidate.kind, active, cb.statusFor(candidate)));
        marker.setzIndex(active ? 100 : 10);
      }
      if (hasEdges) {
        // Expand bounds by ~20% margin so the R2 APAC view pulls back further
        // and all 8 cities breathe with room to spare.
        const positions = point.edges!.map((edge) => pos(edge.latlng));
        const lngs = positions.map((p) => p[0]);
        const lats = positions.map((p) => p[1]);
        const minLng = Math.min(...lngs);
        const maxLng = Math.max(...lngs);
        const minLat = Math.min(...lats);
        const maxLat = Math.max(...lats);
        const lngMargin = Math.max((maxLng - minLng) * 0.2, 2);
        const latMargin = Math.max((maxLat - minLat) * 0.2, 2);
        map.setBounds(
          new A.Bounds(
            [minLng - lngMargin, minLat - latMargin],
            [maxLng + lngMargin, maxLat + latMargin],
          ),
          false,
        );
      } else {
        const center = pos(point.latlng);
        const activeMarker = mainMarkers.get(point.id);
        map.setZoomAndCenter(TOUR_ZOOM, center, false);
        // Open the InfoWindow only after the fly animation settles; anchor it
        // to the marker's rendered position so it sits right next to the dot.
        window.clearTimeout(infoTimer);
        infoTimer = window.setTimeout(() => {
          openPoint = point;
          info.setContent(popupShell(cb.buildPopup(point)));
          info.open(map, activeMarker != null ? activeMarker.getPosition() : center);
        }, 600);
      }
    },
    refresh() {
      for (const candidate of INFRA_POINTS) {
        const marker = mainMarkers.get(candidate.id);
        if (marker == null || marker.getMap() == null) continue;
        marker.setContent(pinHtml(candidate.kind, candidate.id === activePointId, cb.statusFor(candidate)));
      }
      // Keep an already-open InfoWindow in sync with fresh popup context
      // (e.g. the wallet connecting or disconnecting mid-tour).
      if (openPoint != null) info.setContent(popupShell(cb.buildPopup(openPoint)));
    },
    destroy() {
      window.clearTimeout(infoTimer);
      try { map.destroy(); } catch { /* already torn down */ }
    },
  };
}

function createLeafletEngine(element: HTMLElement, cb: EngineCallbacks): MapEngine {
  const markers = new Map<string, L.Marker>();
  const edgeMarkers = new Map<string, L.Marker[]>();
  let activePointId = "";
  const map = L.map(element, {
    center: [20, 10],
    zoom: 2,
    minZoom: 2,
    worldCopyJump: true,
    attributionControl: false,
  });
  L.tileLayer("https://tile.openstreetmap.org/{z}/{x}/{y}.png", { maxZoom: 19 }).addTo(map);

  const syncLabels = () => {
    element.classList.toggle("mapmonitor-zoom-labels", map.getZoom() >= LABEL_ZOOM);
  };
  map.on("zoomend", syncLabels);
  syncLabels();

  for (const [index, point] of INFRA_POINTS.entries()) {
    const marker = L.marker(point.latlng, { icon: dotIcon(point.kind, false, cb.statusFor(point)), title: point.name })
      .addTo(map)
      .bindPopup(cb.buildPopup(point));
    marker.on("click", () => cb.onStopClick(index));
    markers.set(point.id, marker);
  }
  for (const point of INFRA_POINTS) {
    if (point.edges == null) continue;
    edgeMarkers.set(
      point.id,
      point.edges.map((edge) =>
        L.marker(edge.latlng, { icon: edgeIcon(), title: `${point.name} · ${edge.name}` })
          .bindPopup(`<strong>${edge.name}</strong><br/>${point.name} edge`),
      ),
    );
  }

  return {
    showStop(point: InfraPoint) {
      activePointId = point.id;
      for (const list of edgeMarkers.values()) {
        for (const marker of list) marker.remove();
      }
      const hasEdges = point.edges != null && point.edges.length > 0;
      if (hasEdges) {
        for (const marker of edgeMarkers.get(point.id) ?? []) marker.addTo(map);
      }
      for (const candidate of INFRA_POINTS) {
        const marker = markers.get(candidate.id);
        if (marker == null) continue;
        const active = candidate.id === point.id;
        if (active && hasEdges) {
          marker.remove();
          continue;
        }
        if (!map.hasLayer(marker)) marker.addTo(map);
        marker.setIcon(dotIcon(candidate.kind, active, cb.statusFor(candidate)));
        marker.setZIndexOffset(active ? 1000 : 0);
      }
      if (hasEdges) {
        map.flyToBounds(L.latLngBounds(point.edges!.map((edge) => edge.latlng)), { duration: 2.4, padding: [48, 48] });
      } else {
        map.flyTo(point.latlng, TOUR_ZOOM, { duration: 2.4 });
        markers.get(point.id)?.openPopup();
      }
    },
    refresh() {
      for (const candidate of INFRA_POINTS) {
        const marker = markers.get(candidate.id);
        if (marker == null) continue;
        marker.setIcon(dotIcon(candidate.kind, candidate.id === activePointId, cb.statusFor(candidate)));
        marker.getPopup()?.setContent(cb.buildPopup(candidate));
      }
    },
    destroy() {
      map.remove();
      element.classList.remove("mapmonitor-zoom-labels");
    },
  };
}

// ---------------------------------------------------------------------------
// Page: 45s tour + live storage board + live marquee probes (90s cadence).
// ---------------------------------------------------------------------------

export default function MapMonitor() {
  const wallet = useWallet();
  const walletConnected = wallet.connected != null;

  // `?stop=<id>` deep-links the tour to a specific endpoint for demos.
  const [activeIndex, setActiveIndex] = useState(() => {
    const requested = new URLSearchParams(window.location.search).get("stop");
    const index = INFRA_POINTS.findIndex((point) => point.id === requested);
    return index >= 0 ? index : 0;
  });
  const mapElementRef = useRef<HTMLDivElement | null>(null);
  const engineRef = useRef<MapEngine | null>(null);
  // The auto-tour holds off while the user explores after a marker click.
  const pauseUntilRef = useRef(0);
  const [mapReady, setMapReady] = useState(false);
  const [mapEngine, setMapEngine] = useState<"amap" | "osm">(AMAP_KEY ? "amap" : "osm");
  const current = INFRA_POINTS[activeIndex];

  const [configRevision, setConfigRevision] = useState(0);
  const cfg = useMemo(() => loadConfig(), [configRevision]);
  const profile = networkProfile(cfg);

  // ---- Live storage stats (chain + manifests, read-only) ----
  type BoardState =
    | { state: "loading" }
    | { state: "unconfigured" }
    | { state: "error" }
    | { state: "ready"; data: StorageStats; cached: boolean };
  const [board, setBoard] = useState<BoardState>({ state: "loading" });
  const [statsRevision, setStatsRevision] = useState(0);

  // Fresh cache (< 7 days) renders instantly without re-querying; `force`
  // (the card's refresh button) always walks the chain again.
  const collect = useCallback((force: boolean) => {
    const cfg = loadConfig();
    if (!isSuiteDirectoryConfigured(cfg.suiteDirectory)) {
      setBoard({ state: "unconfigured" });
      return;
    }
    if (!force) {
      const cached = loadCachedStorageStats(cfg);
      if (cached != null) {
        setBoard({ state: "ready", data: cached, cached: true });
        return;
      }
    }
    setBoard({ state: "loading" });
    collectStorageStats(cfg).then(
      (data) => {
        saveCachedStorageStats(cfg, data);
        setBoard({ state: "ready", data, cached: false });
      },
      () => { setBoard({ state: "error" }); },
    );
  }, []);

  useEffect(() => {
    collect(false);
  }, [collect, statsRevision]);

  useEffect(() => {
    const refresh = () => {
      setStatsRevision((revision) => revision + 1);
      setConfigRevision((revision) => revision + 1);
    };
    window.addEventListener(CONFIG_CHANGED_EVENT, refresh);
    return () => window.removeEventListener(CONFIG_CHANGED_EVENT, refresh);
  }, []);

  const boardData = board.state === "ready" ? board.data : null;
  const totalPacks = boardData != null ? boardData.totalPacks + boardData.v1.packs : null;
  const totalRepos = boardData != null ? boardData.repos + boardData.v1.repos : null;

  // ---- Live marquee probes (the Monitor page's collection functions) ----
  const [probe, setProbe] = useState<ProbeSnapshotState>(INITIAL_PROBE);
  const probeRunRef = useRef(0);

  const runProbes = useCallback(async () => {
    const checkedAt = Date.now();
    const configured = isSuiteDirectoryConfigured(cfg.suiteDirectory);
    setProbe((prev) => ({ ...prev, refreshing: true }));
    // Sources land in the snapshot independently as they settle: one slow or
    // hanging probe (a long activity walk, a stalled LCD) must not freeze the
    // other cards. A newer probe run invalidates every write from this one,
    // so a slow straggler can never clobber fresher observations.
    const run = ++probeRunRef.current;
    const apply = (
      patch: Partial<ProbeSnapshotState> | ((prev: ProbeSnapshotState) => Partial<ProbeSnapshotState>),
    ) => {
      if (probeRunRef.current !== run) return;
      setProbe((prev) => ({ ...prev, ...(typeof patch === "function" ? patch(prev) : patch) }));
    };

    const suitePromise = configured ? verifySuite(cfg) : Promise.resolve(null);
    const latestPromise = latestEvmBlock(cfg);
    const activityPromise = configured ? contractActivity(cfg, 50) : Promise.resolve([] as ContractTx[]);
    const v1Promise = prepareCosmWasmV1Snapshot();
    const ipfsPromises = MARQUEE_GATEWAYS.map((gateway) => probeGatewayFromBrowser(gateway.endpoint));

    if (!configured) {
      apply({
        evm: {
          state: "not-configured",
          detail: "Add a verified EVM V2/V3 SuiteDirectory in Settings",
          checkedAt,
        },
        evmV4: {
          state: "not-configured",
          detail: "Add a verified EVM V4 SuiteDirectory in Settings",
          checkedAt,
        },
      });
    } else {
      suitePromise.then(
        (binding) => {
          if (binding == null) return;
          apply({
            evm: {
              state: "healthy",
              detail: `Verified on ${profile.label}`,
              checkedAt,
            },
            evmV4: binding.version === 4n
              ? {
                  state: "healthy",
                  detail: `Verified on ${profile.label}`,
                  checkedAt,
                }
              : {
                  state: "not-configured",
                  detail: `Directory is suite v${binding.version}· successor not selected`,
                  checkedAt,
                },
          });
        },
        (reason) => {
          const suiteError = formatError(reason);
          apply({
            evm: {
              state: "unavailable",
              detail: suiteError || "Suite verification failed",
              checkedAt,
              error: suiteError || undefined,
            },
            evmV4: {
              state: "unavailable",
              detail: "Suite verification failed· successor state unknown",
              checkedAt,
            },
          });
        },
      );
    }

    latestPromise.then(
      (value) => apply({ latestBlock: value }),
      () => apply({ latestBlock: null }),
    );

    activityPromise.then(
      (value) => apply({
        activityCount: value.length,
        activityError: configured ? "" : "SuiteDirectory is not configured",
      }),
      (reason) => apply({
        activityCount: null,
        activityError: formatError(reason) || (configured ? "" : "SuiteDirectory is not configured"),
      }),
    );

    v1Promise.then(
      (height) => apply({
        v1: {
          state: "healthy",
          detail: `Read-only snapshot at block ${formatBlock(height)}`,
          checkedAt,
          snapshotHeight: height,
        },
      }),
      (reason) => {
        const v1Error = formatError(reason);
        apply({
          v1: {
            state: "unavailable",
            detail: v1Error || "Archive endpoint is unavailable",
            checkedAt,
            error: v1Error || undefined,
            snapshotHeight: null,
          },
        });
      },
    );

    ipfsPromises.forEach((promise, index) => {
      const gateway = MARQUEE_GATEWAYS[index];
      void promise.then(
        (result) => {
          const state: SourceState = result.responseSamples === 0
            ? "unavailable"
            : result.ok
              ? "healthy"
              : "degraded";
          const detail = result.responseSamples > 0
            ? `${result.reachableSamples}/${result.sampleCount} reachable · ${result.latencyMs ?? "—"} ms median · browser probes`
            : result.error || "Gateway probe failed";
          apply((prev) => ({
            ipfs: {
              ...prev.ipfs,
              [gateway.id]: {
                state,
                detail,
                checkedAt,
                error: result.error || undefined,
                latencyMs: result.latencyMs ?? null,
                statusCode: result.status ?? null,
                probeSource: result.source ?? null,
                sampleCount: result.sampleCount ?? BROWSER_GATEWAY_PROBE_SAMPLE_COUNT,
                responseSamples: result.responseSamples ?? 0,
                reachableSamples: result.reachableSamples ?? 0,
              } satisfies IpfsGatewaySnapshot,
            },
          }));
        },
        (reason) => {
          const error = formatError(reason);
          apply((prev) => ({
            ipfs: {
              ...prev.ipfs,
              [gateway.id]: {
                state: "unavailable" as SourceState,
                detail: error || "Gateway probe failed",
                checkedAt,
                error: error || undefined,
                latencyMs: null,
                statusCode: null,
                probeSource: null,
                sampleCount: BROWSER_GATEWAY_PROBE_SAMPLE_COUNT,
                responseSamples: 0,
                reachableSamples: 0,
              } satisfies IpfsGatewaySnapshot,
            },
          }));
        },
      );
    });

    await Promise.allSettled([suitePromise, latestPromise, activityPromise, v1Promise, ...ipfsPromises]);
    if (probeRunRef.current !== run) return;
    apply({ refreshing: false, lastRefresh: checkedAt });
  }, [cfg, profile.label]);

  // One full collection on mount, then the Monitor page's 90s cadence.
  useEffect(() => {
    void runProbes();
    const timer = window.setInterval(() => void runProbes(), PROBE_REFRESH_INTERVAL_MS);
    return () => window.clearInterval(timer);
  }, [runProbes]);

  // ---- On-demand endpoint pings for the active stop ----
  // The tour (or a user click) activates one stop at a time; only that
  // endpoint is pinged, and only when its cached average has gone stale.
  // Nothing polls the other endpoints until the tour reaches them again.
  const [pointProbes, setPointProbes] = useState<Record<string, PointProbeEntry>>(() => loadPointProbeCache());
  const inflightPointProbesRef = useRef<Set<string>>(new Set());

  useEffect(() => {
    const target = POINT_PROBE_TARGET_MAP.get(current.id);
    if (target == null) return;
    const cached = pointProbes[current.id];
    if (cached != null && Date.now() - cached.sampledAt < POINT_PROBE_TTL_MS) return;
    if (inflightPointProbesRef.current.has(current.id)) return;
    inflightPointProbesRef.current.add(current.id);
    void samplePointLatency(target.url).then((samples) => {
      inflightPointProbesRef.current.delete(current.id);
      setPointProbes((prev) => {
        const previous = prev[current.id];
        // A run outside the previous entry's validity window starts a fresh
        // rolling window instead of averaging across the gap.
        const base = previous != null && Date.now() - previous.sampledAt < POINT_PROBE_TTL_MS
          ? previous.samples
          : [];
        return {
          ...prev,
          [current.id]: {
            samples: [...base, ...samples].slice(-POINT_PROBE_SAMPLE_CAP),
            reachable: samples.length > 0,
            sampledAt: Date.now(),
          },
        };
      });
    });
  }, [current, pointProbes]);

  // Persist the ping cache so a reopen inside the TTL window reuses it.
  useEffect(() => {
    savePointProbeCache(pointProbes);
  }, [pointProbes]);

  // ---- Per-point live status for the map pins ----
  // A fresh on-demand ping entry decides the badge and the latency label
  // (gateways additionally require the average to stay under the ceiling).
  // Without a fresh entry the board's observed packs decide, which also
  // covers the R2 anycast stop (no single endpoint to ping).
  const pointStatuses = useMemo(() => {
    const statuses = new Map<string, PointLiveStatus>();
    for (const point of INFRA_POINTS) {
      const entry = pointProbes[point.id];
      if (entry != null && Date.now() - entry.sampledAt < POINT_PROBE_TTL_MS) {
        const latencyMs = averageLatency(entry.samples);
        const gateway = POINT_PROBE_TARGET_MAP.get(point.id)?.gateway === true;
        statuses.set(point.id, {
          online: entry.reachable && latencyMs != null && (!gateway || latencyMs < GATEWAY_ONLINE_LATENCY_MS),
          latencyMs,
        });
        continue;
      }
      const live = liveBoardRow(point.id, boardData);
      statuses.set(point.id, live.packs == null ? UNKNOWN_STATUS : { online: live.packs > 0, latencyMs: null });
    }
    return statuses;
  }, [pointProbes, boardData]);

  const statusFor = useCallback(
    (point: InfraPoint): PointLiveStatus => pointStatuses.get(point.id) ?? UNKNOWN_STATUS,
    [pointStatuses],
  );

  // Popup body: base tour info, then (wallet-gated) a short sample of the
  // owner's repositories observed at this endpoint, then the owner link.
  // With more than one observed owner the popup reports the count instead of
  // guessing which owner's repositories to list.
  const ownerCount = boardData?.observedOwners ?? null;
  const pointRepos = useMemo(() => {
    const rows = new Map<string, { names: readonly string[]; total: number | null }>();
    for (const point of INFRA_POINTS) rows.set(point.id, livePointRepos(point.id, boardData));
    return rows;
  }, [boardData]);
  const buildPopup = useCallback(
    (point: InfraPoint): string => {
      const base = point.edges != null && point.edges.length > 0
        ? `<strong>${point.name}</strong><br/>${KIND_META[point.kind].label} · ${point.edges.length} APAC edges<br/>${point.edges.map((edge) => `· ${edge.name}`).join("<br/>")}`
        : `<strong>${point.name}</strong><br/>${KIND_META[point.kind].label} · ${point.endpoint}`;
      if (!walletConnected) return base;
      if (ownerCount != null && ownerCount > 1) {
        return `${base}<br/><span class="mapmonitor-popup-meta">${ownerCount} owners observed</span>`;
      }
      const repos = pointRepos.get(point.id) ?? { names: [], total: null };
      const shown = repos.names.slice(0, POPUP_REPO_ROW_LIMIT);
      const rows = shown
        .map((name) => `<a class="mapmonitor-popup-repo" href="${OBSERVED_OWNER_PATH}/${encodeURIComponent(name)}" target="_blank" rel="noreferrer" title="${escapeHtml(name)}">${escapeHtml(name)}</a>`)
        .join("");
      const others = Math.max(0, (repos.total ?? shown.length) - shown.length);
      const othersRow = others > 0 ? `<span class="mapmonitor-popup-others">+ ${others} others…</span>` : "";
      const list = rows !== "" || othersRow !== ""
        ? `<div class="mapmonitor-popup-repos">${rows}${othersRow}</div>`
        : "";
      return `${base}<br/>${list}<a class="mapmonitor-popup-link" href="${OBSERVED_OWNER_PATH}" target="_blank" rel="noreferrer">View owner repos →</a>`;
    },
    [walletConnected, ownerCount, pointRepos],
  );

  // The engines are built once; fresh callbacks reach them through refs, and
  // a callback change re-renders the live pins and any open popup.
  const buildPopupRef = useRef(buildPopup);
  const statusForRef = useRef(statusFor);
  useEffect(() => {
    buildPopupRef.current = buildPopup;
    statusForRef.current = statusFor;
    engineRef.current?.refresh();
  }, [buildPopup, statusFor, mapReady]);

  // Create the map engine: AMap/Gaode in production (mainland-safe
  // borders, GCJ-02, enterprise key), Tencent Maps as the secondary licensed
  // engine, Leaflet/OpenStreetMap as the key-less dev fallback. The 45s tour
  // timer lives here so it survives either engine.
  useEffect(() => {
    const element = mapElementRef.current;
    if (element == null) return;
    let cancelled = false;

    const callbacks: EngineCallbacks = {
      onStopClick: (index: number) => {
        // A user click parks the tour on that stop for the grace window;
        // every further click re-arms it.
        pauseUntilRef.current = Date.now() + TOUR_CLICK_PAUSE_MS;
        setActiveIndex(index);
      },
      buildPopup: (point) => buildPopupRef.current(point),
      statusFor: (point) => statusForRef.current(point),
    };

    const bootLeaflet = () => {
      if (cancelled || !element.isConnected) return;
      setMapEngine("osm");
      engineRef.current = createLeafletEngine(element, callbacks);
      setMapReady(true);
    };

    if (AMAP_KEY) {
      void loadAMap().then(
        (A) => {
          if (cancelled || !element.isConnected) return;
          engineRef.current = createAMapEngine(A, element, callbacks);
          setMapEngine("amap");
          setMapReady(true);
        },
        () => {
          console.warn("AMap unavailable; falling back to OpenStreetMap (dev only).");
          bootLeaflet();
        },
      );
    } else {
      bootLeaflet();
    }

    const tourTimer = window.setInterval(() => {
      // Skip the tick while the post-click grace window is open.
      if (Date.now() < pauseUntilRef.current) return;
      setActiveIndex((index) => (index + 1) % INFRA_POINTS.length);
    }, TOUR_INTERVAL_MS);

    return () => {
      cancelled = true;
      window.clearInterval(tourTimer);
      engineRef.current?.destroy();
      engineRef.current = null;
    };
  }, []);

  // Fly to the active stop, light it up, and open its info bubble. Runs on
  // mount once the engine is ready and on every tour tick after it.
  useEffect(() => {
    engineRef.current?.showStop(current);
  }, [current, mapReady]);

  return (
    <div className="mapmonitor-page">
      <div className="monitor-heading">
        <div>
          <div className="monitor-eyebrow">
            <MapIcon size={15} /> Map monitor <Badge variant="outline">Infrastructure tour</Badge>
          </div>
          <h1>Reachable infrastructure</h1>
          <p>Storage buckets, IPFS nodes, and gateways the igit client can reach, on a rotating world tour.</p>
        </div>
        <div className="monitor-heading-actions">
          <span className="monitor-refresh-time">{INFRA_POINTS.length} endpoints · next stop every 45s · hover to pause marquee</span>
        </div>
      </div>

      <div className="mapmonitor-map-wrap">
        <div ref={mapElementRef} className="mapmonitor-map" aria-label="World map with igit infrastructure endpoints" />
        {mapEngine === "osm" && <span className="mapmonitor-map-credit">© OpenStreetMap · dev fallback</span>}
        <div className="mapmonitor-focus-card" aria-live="polite">
          <span className="mapmonitor-focus-kicker">Now touring</span>
          <b>{current.name}</b>
          <span className="mapmonitor-focus-kind" data-kind={current.kind}>{KIND_META[current.kind].label}</span>
          {current.edges != null ? (
            <ul className="mapmonitor-focus-edges-list" aria-label="Lit APAC edge nodes">
              {current.edges.map((edge) => (
                <li key={edge.name}><i aria-hidden="true" />{edge.name}</li>
              ))}
            </ul>
          ) : (
            <code title={current.endpoint}>{current.endpoint}</code>
          )}
          <span className="mapmonitor-focus-note">{current.description}</span>
        </div>

        {/* Bottom-right board: totals plus per-endpoint file/repo counters. */}
        <div className="mapmonitor-stats-card" aria-label="Storage overview">
          <div className="mapmonitor-stats-header">
            <span className="mapmonitor-focus-kicker">Storage overview</span>
            <button
              type="button"
              className="mapmonitor-stats-refresh"
              onClick={() => collect(true)}
              disabled={board.state === "loading"}
              title="Re-query chain, manifests, and archive (bypasses the weekly cache)"
              aria-label="Refresh storage stats"
            >
              <RefreshCw size={11} className={board.state === "loading" ? "spin" : undefined} />
            </button>
          </div>
          <div className="mapmonitor-stats-total">
            <span>
              <b>{totalPacks != null ? totalPacks.toLocaleString("en-US") : "—"}</b>
              <small>Packfiles</small>
            </span>
            <span>
              <b>{totalRepos != null ? totalRepos.toLocaleString("en-US") : "—"}</b>
              <small>Repos bound</small>
            </span>
          </div>
          <ul className="mapmonitor-stats-endpoints">
            <li className="mapmonitor-stats-head">
              <i aria-hidden="true" /><span>Endpoint</span><b>Packs</b><b>Repos</b>
            </li>
            {INFRA_POINTS
              .filter((point) => {
                // Rows with no observed packs/repos stay hidden while live
                // data is available; they reappear as soon as content lands
                // on that endpoint.
                if (boardData == null) return true;
                const live = liveBoardRow(point.id, boardData);
                return live.packs !== 0 || live.repos !== 0;
              })
              .map((point) => {
                const live = liveBoardRow(point.id, boardData);
                return (
                  <li key={point.id} title={point.name}>
                    <i aria-hidden="true" style={{ background: KIND_META[point.kind].color }} />
                    <span>{point.name}</span>
                    <b>{live.packs != null ? live.packs.toLocaleString("en-US") : "—"}</b>
                    <b>{live.repos != null ? live.repos.toLocaleString("en-US") : "—"}</b>
                  </li>
                );
              })}
            <li title="CosmWasm V1 archive · read-only snapshot, separate trust root">
              <i aria-hidden="true" style={{ background: "#71717a" }} />
              <span>CosmWasm V1 archive</span>
              <b>{boardData != null ? boardData.v1.packs.toLocaleString("en-US") : "—"}</b>
              <b>{boardData != null ? boardData.v1.repos.toLocaleString("en-US") : "—"}</b>
            </li>
          </ul>
          <span className="mapmonitor-stats-note">
            {board.state === "loading" && "Sampling chain, manifests, and archive…"}
            {board.state === "unconfigured" && "SuiteDirectory not configured"}
            {board.state === "error" && "Stats unavailable · chain read failed"}
            {board.state === "ready" &&
              `${board.cached ? `Cached ${sampleAgeLabel(board.data.sampledAt)}` : "Live"} · ${board.data.observedOwners} owners · ${board.data.repos}/${board.data.reposDiscovered} EVM + ${board.data.v1.repos} V1 repos${board.data.truncated ? " · bounded" : ""}`}
          </span>
        </div>
      </div>

      <section className="mapmonitor-marquee" aria-label="Monitor cards carousel">
        <div className="mapmonitor-marquee-track">
          <MarqueeGroup probe={probe} profileLabel={profile.label} />
          {/* Exact duplicate enables the seamless translateX(-50%) loop. */}
          <MarqueeGroup probe={probe} profileLabel={profile.label} hidden />
        </div>
      </section>
    </div>
  );
}
