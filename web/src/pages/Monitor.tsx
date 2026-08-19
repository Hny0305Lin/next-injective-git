import { useCallback, useEffect, useMemo, useState, type ReactNode } from "react";
import { Link } from "react-router-dom";
import type { Hex } from "viem";
import {
  Activity,
  AlertTriangle,
  Archive as ArchiveIcon,
  ArrowUpRight,
  Box,
  CheckCircle2,
  Clock3,
  Cloud,
  Database,
  ExternalLink,
  Gauge,
  HardDrive,
  Info,
  LoaderCircle,
  MapPin,
  RefreshCw,
  Server,
  ShieldCheck,
  Wifi,
  XCircle,
  type LucideIcon,
} from "lucide-react";
import { Alert, AlertDescription, AlertTitle } from "../components/ui/alert";
import { Badge } from "../components/ui/badge";
import {
  Card,
  CardAction,
  CardContent,
  CardDescription,
  CardHeader,
  CardTitle,
} from "../components/ui/card";
import { Separator } from "../components/ui/separator";
import { Skeleton } from "../components/ui/skeleton";
import {
  Table,
  TableBody,
  TableCell,
  TableHead,
  TableHeader,
  TableRow,
} from "../components/ui/table";
import { Tabs, TabsContent, TabsList, TabsTrigger } from "../components/ui/tabs";
import { Tooltip, TooltipContent, TooltipTrigger } from "../components/ui/tooltip";
import { Button } from "../components/ui/button";
import {
  CONFIG_CHANGED_EVENT,
  isSuiteDirectoryConfigured,
  loadConfig,
  networkProfile,
} from "../lib/profile";
import {
  contractActivity,
  EVM_ACTIVITY_BLOCK_WINDOW,
  formatError,
  timeAgo,
  type ContractTx,
} from "../lib/chain";
import { COSMWASM_V1_ARCHIVE, prepareCosmWasmV1Snapshot } from "../lib/cosmwasm-v1";
import { rpcRequest, verifySuite } from "../lib/transport";
import type { AppConfig } from "../lib/profile";

type SourceState = "loading" | "healthy" | "not-configured" | "unavailable" | "degraded";

interface SourceSnapshot {
  state: SourceState;
  detail: string;
  checkedAt: number | null;
  error?: string;
}

interface MonitorSnapshot {
  evm: SourceSnapshot;
  latestBlock: bigint | null;
  activity: ContractTx[];
  activityError: string;
  v1: SourceSnapshot & { snapshotHeight: number | null };
  ipfs: SourceSnapshot & {
    latencyMs: number | null;
    statusCode: number | null;
    probeSource: "server" | "browser" | null;
  };
}

interface IpfsProbeResult {
  ok: boolean;
  status: number | null;
  latencyMs: number | null;
  error?: string;
  source: "server" | "browser";
}

const INITIAL_SNAPSHOT: MonitorSnapshot = {
  evm: { state: "loading", detail: "Checking SuiteDirectory", checkedAt: null },
  latestBlock: null,
  activity: [],
  activityError: "",
  v1: { state: "loading", detail: "Checking archive endpoint", checkedAt: null, snapshotHeight: null },
  ipfs: { state: "loading", detail: "Checking gateway", checkedAt: null, latencyMs: null, statusCode: null, probeSource: null },
};

const REFRESH_INTERVAL_MS = 60_000;

type PublicNodeStatus = "current" | "legacy";

interface PublicStorageNode {
  id: string;
  title: string;
  description: string;
  kind: "IPFS gateway" | "Storage provider";
  region: string;
  role: string;
  endpoint: string;
  href: string;
  status: PublicNodeStatus;
  icon: LucideIcon;
}

// These are intentionally public topology facts, not live provider metrics.
// Provider capacity and credentials remain on the server-side archive monitor.
const PUBLIC_STORAGE_NODES: readonly PublicStorageNode[] = [
  {
    id: "us-archive",
    title: "US archive node",
    description: "Read-only gateway for the durable US archive.",
    kind: "IPFS gateway",
    region: "United States",
    role: "Current archive path",
    endpoint: "https://igit-us.haohanyh.ovh",
    href: "https://igit-us.haohanyh.ovh/healthz",
    status: "current",
    icon: Server,
  },
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

function shortAddress(value: string, size = 8) {
  return value.length > size * 2 ? `${value.slice(0, size)}...${value.slice(-4)}` : value;
}

function formatBlock(value: bigint | number | null) {
  return value == null ? "—" : typeof value === "bigint" ? value.toLocaleString("en-US") : value.toLocaleString("en-US");
}

function formatCheckedAt(value: number | null) {
  return value == null ? "Not checked" : timeAgo(value / 1000);
}

function formatAction(action: string) {
  return action.replaceAll("_", " ");
}

function activityContext(row: ContractTx) {
  const attributes = row.attributes;
  const repository = attributes.repo ?? attributes.repository ?? attributes.name;
  if (repository) return repository;
  const owner = attributes.owner ?? attributes.recipient;
  if (owner) return shortAddress(owner, 8);
  return "Suite module event";
}

function activityBuckets(rows: ContractTx[]) {
  const buckets = Array.from({ length: 12 }, () => 0);
  if (rows.length === 0) return buckets;
  const heights = rows.map((row) => Number(row.height)).filter(Number.isFinite);
  const minimum = Math.min(...heights);
  const maximum = Math.max(...heights);
  const span = Math.max(maximum - minimum, 1);
  rows.forEach((row) => {
    const height = Number(row.height);
    if (!Number.isFinite(height)) return;
    const index = Math.min(11, Math.floor(((height - minimum) / span) * 12));
    buckets[index] += 1;
  });
  return buckets;
}

function sourceBadge(state: SourceState, healthyLabel = "Healthy") {
  if (state === "healthy") {
    return { label: healthyLabel, className: "monitor-badge-healthy", icon: CheckCircle2 };
  }
  if (state === "loading") {
    return { label: "Checking", className: "monitor-badge-loading", icon: LoaderCircle };
  }
  if (state === "not-configured") {
    return { label: "Not configured", className: "monitor-badge-warning", icon: AlertTriangle };
  }
  if (state === "degraded") {
    return { label: "Degraded", className: "monitor-badge-warning", icon: AlertTriangle };
  }
  return { label: "Unavailable", className: "monitor-badge-danger", icon: XCircle };
}

function publicNodeBadge(status: PublicNodeStatus) {
  return status === "current"
    ? { label: "Current path", className: "monitor-badge-healthy" }
    : { label: "Legacy copy", className: "monitor-badge-warning" };
}

function SourceCard({
  icon: Icon,
  title,
  source,
  description,
  healthyLabel,
  children,
}: {
  icon: LucideIcon;
  title: string;
  source: SourceSnapshot;
  description: string;
  healthyLabel?: string;
  children?: ReactNode;
}) {
  const status = sourceBadge(source.state, healthyLabel);
  const StatusIcon = status.icon;
  return (
    <Card className="monitor-source-card">
      <CardHeader className="monitor-card-header">
        <div className="monitor-source-title">
          <span className="monitor-icon-tile"><Icon size={16} /></span>
          <div>
            <CardTitle>{title}</CardTitle>
            <CardDescription>{description}</CardDescription>
          </div>
        </div>
        <CardAction>
          <Badge variant="outline" className={status.className}>
            <StatusIcon className={source.state === "loading" ? "animate-spin" : undefined} />
            {status.label}
          </Badge>
        </CardAction>
      </CardHeader>
      <CardContent className="monitor-source-content">
        <p className="monitor-source-detail">{source.detail}</p>
        {children}
      </CardContent>
    </Card>
  );
}

function MetricCard({
  icon: Icon,
  label,
  value,
  detail,
  loading,
}: {
  icon: LucideIcon;
  label: string;
  value: string;
  detail: string;
  loading?: boolean;
}) {
  return (
    <Card size="sm" className="monitor-metric-card">
      <CardContent className="monitor-metric-content">
        <div className="monitor-metric-label"><Icon size={14} /> {label}</div>
        {loading ? <Skeleton className="monitor-metric-skeleton" /> : <strong>{value}</strong>}
        <span>{detail}</span>
      </CardContent>
    </Card>
  );
}

function PublicStorageNodeCard({ node }: { node: PublicStorageNode }) {
  const Icon = node.icon;
  const status = publicNodeBadge(node.status);
  return (
    <Card className="monitor-node-card">
      <CardHeader className="monitor-card-header">
        <div className="monitor-source-title">
          <span className="monitor-icon-tile"><Icon size={16} /></span>
          <div>
            <CardTitle>{node.title}</CardTitle>
            <CardDescription>{node.description}</CardDescription>
          </div>
        </div>
        <CardAction>
          <Badge variant="outline" className={status.className}>{status.label}</Badge>
        </CardAction>
      </CardHeader>
      <CardContent className="monitor-node-content">
        <div className="monitor-node-meta">
          <span><MapPin size={13} /> {node.region}</span>
          <span><HardDrive size={13} /> {node.kind}</span>
        </div>
        <p className="monitor-node-role">{node.role}</p>
        <div className="monitor-node-endpoint">
          <code title={node.endpoint}>{node.endpoint}</code>
          <a href={node.href} target="_blank" rel="noreferrer" title={`Open ${node.title} endpoint`}>
            <ExternalLink size={13} /> Endpoint
          </a>
        </div>
      </CardContent>
    </Card>
  );
}

async function latestEvmBlock(cfg: AppConfig): Promise<bigint> {
  const raw = await rpcRequest<Hex>(cfg, "eth_blockNumber");
  if (!/^0x(?:0|[1-9a-fA-F][0-9a-fA-F]*)$/.test(raw)) {
    throw new Error("EVM RPC returned an invalid latest block");
  }
  return BigInt(raw);
}

function parseIpfsProbePayload(value: unknown): IpfsProbeResult {
  if (value == null || typeof value !== "object") throw new Error("IPFS health API returned malformed JSON");
  const record = value as Record<string, unknown>;
  const status = record.status === null ? null : record.status;
  const latencyMs = record.latencyMs === null ? null : record.latencyMs;
  if (typeof record.ok !== "boolean" || (status !== null && !Number.isInteger(status)) || (latencyMs !== null && !Number.isFinite(latencyMs))) {
    throw new Error("IPFS health API returned an invalid probe result");
  }
  return {
    ok: record.ok,
    status: status as number | null,
    latencyMs: latencyMs as number | null,
    error: typeof record.error === "string" ? record.error : undefined,
    source: "server",
  };
}

async function probeIpfsGatewayFromApi(profile: string): Promise<IpfsProbeResult | null> {
  const controller = new AbortController();
  const timer = setTimeout(() => controller.abort(), 12_000);
  try {
    const response = await fetch(`/api/ipfs-health?profile=${encodeURIComponent(profile)}`, {
      headers: { Accept: "application/json" },
      signal: controller.signal,
    });
    const contentType = response.headers.get("content-type") ?? "";
    // Vite's dev server does not serve Vercel functions. Keep local previews
    // useful by falling back only when the health API is absent, not when the
    // API reports a gateway failure.
    if (response.status === 404 || !contentType.includes("application/json")) return null;
    if (!response.ok) throw new Error(`IPFS health API failed (HTTP ${response.status})`);
    return parseIpfsProbePayload(await response.json());
  } finally {
    clearTimeout(timer);
  }
}

async function probeIpfsGatewayDirect(gateway: string): Promise<IpfsProbeResult> {
  const endpoint = `${gateway.replace(/\/+$/, "")}/ipfs/`;
  const controller = new AbortController();
  const timer = setTimeout(() => controller.abort(), 8_000);
  const started = performance.now();
  try {
    let response = await fetch(endpoint, {
      method: "HEAD",
      headers: { Accept: "text/plain" },
      signal: controller.signal,
    });
    if (response.status === 405 || response.status === 501) {
      response = await fetch(endpoint, {
        method: "GET",
        headers: { Accept: "text/plain", Range: "bytes=0-0" },
        signal: controller.signal,
      });
      void response.body?.cancel();
    }
    return {
      ok: response.status < 500,
      status: response.status,
      latencyMs: Math.round(performance.now() - started),
      source: "browser",
    };
  } catch (error) {
    if (error instanceof DOMException && error.name === "AbortError") {
      throw new Error("gateway probe timed out");
    }
    throw error;
  } finally {
    clearTimeout(timer);
  }
}

async function probeIpfsGateway(gateway: string, profile: string): Promise<IpfsProbeResult> {
  const serverResult = await probeIpfsGatewayFromApi(profile);
  return serverResult ?? await probeIpfsGatewayDirect(gateway);
}

export default function Monitor() {
  const [configRevision, setConfigRevision] = useState(0);
  const cfg = useMemo(() => loadConfig(), [configRevision]);
  const profile = networkProfile(cfg);
  const [snapshot, setSnapshot] = useState<MonitorSnapshot>(INITIAL_SNAPSHOT);
  const [refreshing, setRefreshing] = useState(false);
  const [lastRefresh, setLastRefresh] = useState<number | null>(null);

  useEffect(() => {
    const refreshConfig = () => setConfigRevision((revision) => revision + 1);
    window.addEventListener(CONFIG_CHANGED_EVENT, refreshConfig);
    return () => window.removeEventListener(CONFIG_CHANGED_EVENT, refreshConfig);
  }, []);

  const refresh = useCallback(async () => {
    const checkedAt = Date.now();
    const configured = isSuiteDirectoryConfigured(cfg.suiteDirectory);
    setRefreshing(true);

    const suitePromise = configured ? verifySuite(cfg) : Promise.resolve(null);
    const latestPromise = latestEvmBlock(cfg);
    const activityPromise = configured ? contractActivity(cfg, 50) : Promise.resolve([] as ContractTx[]);
    const v1Promise = prepareCosmWasmV1Snapshot();
    const ipfsPromise = probeIpfsGateway(cfg.ipfsGateway, cfg.profile);

    const [suite, latest, activity, v1, ipfs] = await Promise.allSettled([
      suitePromise,
      latestPromise,
      activityPromise,
      v1Promise,
      ipfsPromise,
    ]);

    const suiteError = suite.status === "rejected" ? formatError(suite.reason) : "";
    const latestError = latest.status === "rejected" ? formatError(latest.reason) : "";
    const activityError = activity.status === "rejected" ? formatError(activity.reason) : configured ? "" : "SuiteDirectory is not configured";
    const v1Error = v1.status === "rejected" ? formatError(v1.reason) : "";
    const ipfsError = ipfs.status === "rejected" ? formatError(ipfs.reason) : "";
    const v1Height = v1.status === "fulfilled" ? v1.value : null;
    const ipfsResult = ipfs.status === "fulfilled" ? ipfs.value : null;
    const ipfsState: SourceState = !ipfsResult || ipfsResult.status == null
      ? "unavailable"
      : ipfsResult.ok
        ? "healthy"
        : "degraded";
    const ipfsDetail = ipfsResult?.status != null
      ? `${ipfsResult.status} response in ${ipfsResult.latencyMs ?? "—"} ms · ${ipfsResult.source} probe`
      : ipfsResult?.error || ipfsError || "Gateway probe failed";

    setSnapshot({
      evm: {
        state: !configured ? "not-configured" : suite.status === "fulfilled" ? "healthy" : "unavailable",
        detail: !configured
          ? "Add a verified EVM V2 SuiteDirectory in Settings"
          : suite.status === "fulfilled"
            ? `Verified on ${profile.label}`
            : suiteError || "Suite verification failed",
        checkedAt,
        error: suiteError || undefined,
      },
      latestBlock: latest.status === "fulfilled" ? latest.value : null,
      activity: activity.status === "fulfilled" ? activity.value : [],
      activityError: activityError || latestError,
      v1: {
        state: v1.status === "fulfilled" ? "healthy" : "unavailable",
        detail: v1.status === "fulfilled"
          ? `Read-only snapshot at block ${formatBlock(v1Height)}`
          : v1Error || "Archive endpoint is unavailable",
        checkedAt,
        error: v1Error || undefined,
        snapshotHeight: v1Height,
      },
      ipfs: {
        state: ipfsState,
        detail: ipfsDetail,
        checkedAt,
        error: ipfsResult?.error || ipfsError || undefined,
        latencyMs: ipfsResult?.latencyMs ?? null,
        statusCode: ipfsResult?.status ?? null,
        probeSource: ipfsResult?.source ?? null,
      },
    });
    setLastRefresh(checkedAt);
    setRefreshing(false);
  }, [cfg, profile.label]);

  useEffect(() => {
    void refresh();
    const interval = window.setInterval(() => void refresh(), REFRESH_INTERVAL_MS);
    return () => window.clearInterval(interval);
  }, [refresh]);

  const buckets = useMemo(() => activityBuckets(snapshot.activity), [snapshot.activity]);
  const maxBucket = Math.max(...buckets, 1);
  const suiteBadge = sourceBadge(snapshot.evm.state);
  const v1Badge = sourceBadge(snapshot.v1.state);
  const ipfsBadge = sourceBadge(snapshot.ipfs.state, "Reachable");
  const evmExplorer = `${profile.evmExplorer.replace(/\/+$/, "")}/address/${cfg.suiteDirectory}`;

  return (
    <div className="monitor-page">
      <div className="monitor-heading">
        <div>
          <div className="monitor-eyebrow"><Gauge size={15} /> Public monitor <Badge variant="outline">Read-only</Badge></div>
          <h1>Network pulse</h1>
          <p>Live observation window for the igit protocol surface.</p>
        </div>
        <div className="monitor-heading-actions">
          <span className="monitor-refresh-time"><Clock3 size={14} /> Updated {formatCheckedAt(lastRefresh)}</span>
          <Button variant="outline" size="sm" onClick={() => void refresh()} disabled={refreshing}>
            <RefreshCw className={refreshing ? "animate-spin" : undefined} />
            Refresh
          </Button>
        </div>
      </div>

      <div className="monitor-source-grid" aria-label="Public data source status">
        <SourceCard
          icon={ShieldCheck}
          title="EVM V2 Suite"
          source={snapshot.evm}
          description="Verified Directory trust root"
        >
          <div className="monitor-source-meta">
            <span><Box size={13} /> Latest block <b>{formatBlock(snapshot.latestBlock)}</b></span>
            {isSuiteDirectoryConfigured(cfg.suiteDirectory) && (
              <a href={evmExplorer} target="_blank" rel="noreferrer" title="Open the EVM SuiteDirectory in Blockscout">
                <ExternalLink size={13} /> {shortAddress(cfg.suiteDirectory, 8)}
              </a>
            )}
          </div>
        </SourceCard>

        <SourceCard
          icon={ArchiveIcon}
          title="CosmWasm V1 archive"
          source={snapshot.v1}
          description="Independent historical read-only surface"
        >
          <div className="monitor-source-meta">
            <span><Box size={13} /> Snapshot <b>{formatBlock(snapshot.v1.snapshotHeight)}</b></span>
            <code title={COSMWASM_V1_ARCHIVE.contract}>{shortAddress(COSMWASM_V1_ARCHIVE.contract, 8)}</code>
          </div>
          <div className="monitor-link-row">
            <Link to="/archive/cosmwasm-v1"><ArrowUpRight size={13} /> Open archive viewer</Link>
            <a href={COSMWASM_V1_ARCHIVE.explorer} target="_blank" rel="noreferrer"><ExternalLink size={13} /> Explorer</a>
          </div>
        </SourceCard>

        <SourceCard
          icon={HardDrive}
          title="IPFS gateway"
          source={snapshot.ipfs}
          description="Server-side public gateway reachability probe"
          healthyLabel="Reachable"
        >
          <div className="monitor-source-meta">
            <span><Wifi size={13} /> Latency <b>{snapshot.ipfs.latencyMs == null ? "—" : `${snapshot.ipfs.latencyMs} ms`}</b></span>
            <code title={cfg.ipfsGateway}>{shortAddress(cfg.ipfsGateway.replace(/^https?:\/\//, ""), 14)}</code>
          </div>
          <div className="monitor-link-row">
            <Link to="/ipfs"><ArrowUpRight size={13} /> Open IPFS explorer</Link>
          </div>
        </SourceCard>
      </div>

      <section className="monitor-topology" aria-labelledby="monitor-topology-title">
        <div className="monitor-topology-heading">
          <div>
            <div className="monitor-topology-kicker"><Server size={14} /> Storage nodes</div>
            <h2 id="monitor-topology-title">Public archive topology</h2>
            <p>Read-only endpoints and provider roles for the current storage path.</p>
          </div>
          <Badge variant="outline">Public inventory</Badge>
        </div>
        <div className="monitor-node-grid">
          {PUBLIC_STORAGE_NODES.map((node) => <PublicStorageNodeCard key={node.id} node={node} />)}
        </div>
        <p className="monitor-topology-note"><Info size={14} /> Provider badges describe the configured role; private capacity and operational telemetry stay server-side.</p>
      </section>

      {snapshot.evm.state !== "healthy" && (
        <Alert variant={snapshot.evm.state === "not-configured" ? "default" : "destructive"} className="monitor-alert">
          <AlertTriangle />
          <div>
            <AlertTitle>{snapshot.evm.state === "not-configured" ? "EVM V2 is not configured" : "EVM V2 is unavailable"}</AlertTitle>
            <AlertDescription>
              {snapshot.evm.state === "not-configured"
                ? "The public monitor is still available, but EVM activity needs a verified SuiteDirectory."
                : snapshot.evm.detail}
            </AlertDescription>
          </div>
          {snapshot.evm.state === "not-configured" && <Link className="monitor-alert-link" to="/settings">Review settings <ArrowUpRight size={13} /></Link>}
        </Alert>
      )}

      <div className="monitor-metric-grid">
        <MetricCard
          icon={Activity}
          label="Recent activity"
          value={snapshot.evm.state === "healthy" ? String(snapshot.activity.length) : "—"}
          detail={snapshot.evm.state === "healthy" ? `up to 50 events · ${EVM_ACTIVITY_BLOCK_WINDOW.toLocaleString()} blocks` : "EVM Suite unavailable"}
          loading={snapshot.evm.state === "loading"}
        />
        <MetricCard
          icon={Box}
          label="Latest observed block"
          value={formatBlock(snapshot.latestBlock)}
          detail={profile.label}
          loading={snapshot.latestBlock == null && snapshot.evm.state === "loading"}
        />
        <MetricCard
          icon={Server}
          label="Observation window"
          value={`${(EVM_ACTIVITY_BLOCK_WINDOW / 1000).toFixed(0)}k blocks`}
          detail="EVM activity sample"
        />
        <MetricCard
          icon={Clock3}
          label="Last refresh"
          value={formatCheckedAt(lastRefresh)}
          detail={`auto refresh · ${REFRESH_INTERVAL_MS / 1000}s`}
          loading={lastRefresh == null}
        />
      </div>

      <Tabs defaultValue="activity" className="monitor-tabs">
        <TabsList variant="line" className="monitor-tabs-list">
          <TabsTrigger value="activity"><Activity /> Activity</TabsTrigger>
          <TabsTrigger value="storage"><HardDrive /> Storage</TabsTrigger>
          <TabsTrigger value="migration"><ArchiveIcon /> V1 boundary</TabsTrigger>
        </TabsList>

        <TabsContent value="activity" className="monitor-tab-content">
          <div className="monitor-content-grid">
            <Card className="monitor-chart-card">
              <CardHeader className="monitor-card-header">
                <div>
                  <CardTitle>Activity pulse</CardTitle>
                  <CardDescription>Decoded EVM actions in the latest observation window.</CardDescription>
                </div>
                <CardAction><Badge variant="outline">EVM V2</Badge></CardAction>
              </CardHeader>
              <CardContent>
                <div className="monitor-bars" aria-label="Activity distribution across the observed block window">
                  {buckets.map((count, index) => (
                    <Tooltip key={index}>
                      <TooltipTrigger render={<span className="monitor-bar-slot" />}>
                        <span className="monitor-bar" style={{ height: `${Math.max(8, (count / maxBucket) * 100)}%` }} />
                      </TooltipTrigger>
                      <TooltipContent>{count} event{count === 1 ? "" : "s"}</TooltipContent>
                    </Tooltip>
                  ))}
                </div>
                <div className="monitor-chart-footer">
                  <span>{snapshot.activity.length ? `${snapshot.activity.length} decoded events returned` : snapshot.activityError || "No activity returned"}</span>
                  <span>{EVM_ACTIVITY_BLOCK_WINDOW.toLocaleString()} block sample</span>
                </div>
              </CardContent>
            </Card>

            <Card className="monitor-summary-card">
              <CardHeader>
                <CardTitle>Source notes</CardTitle>
                <CardDescription>Scope and freshness</CardDescription>
              </CardHeader>
              <CardContent className="monitor-summary-list">
                <div><span><ShieldCheck size={14} /> EVM V2</span><b>{suiteBadge.label}</b></div>
                <Separator />
                <div><span><ArchiveIcon size={14} /> V1 archive</span><b>{v1Badge.label}</b></div>
                <Separator />
                <div><span><HardDrive size={14} /> IPFS probe</span><b>{ipfsBadge.label}</b></div>
                <Separator />
                <p><Info size={14} /> Counts are bounded observations, never all-time totals.</p>
              </CardContent>
            </Card>
          </div>

          <Card className="monitor-table-card">
            <CardHeader className="monitor-card-header">
              <div>
                <CardTitle>Recent actions</CardTitle>
                <CardDescription>Confirmed transactions returned by the verified Suite modules.</CardDescription>
              </div>
              <CardAction><Link className="monitor-card-link" to="/explorer">Open explorer <ArrowUpRight size={13} /></Link></CardAction>
            </CardHeader>
            <CardContent className="monitor-table-content">
              {snapshot.evm.state === "loading" ? (
                <div className="monitor-skeleton-list"><Skeleton /><Skeleton /><Skeleton /></div>
              ) : snapshot.activity.length === 0 ? (
                <div className="monitor-empty"><Activity size={20} /><p>{snapshot.activityError || "No recent EVM activity."}</p></div>
              ) : (
                <Table>
                  <TableHeader>
                    <TableRow>
                      <TableHead>Action</TableHead>
                      <TableHead>Context</TableHead>
                      <TableHead>Sender</TableHead>
                      <TableHead>Block</TableHead>
                      <TableHead>Time</TableHead>
                      <TableHead className="text-right">Tx</TableHead>
                    </TableRow>
                  </TableHeader>
                  <TableBody>
                    {snapshot.activity.slice(0, 12).map((row) => (
                      <TableRow key={row.txhash}>
                        <TableCell><Badge variant="secondary" className="monitor-action-badge">{formatAction(row.action || "contract event")}</Badge>{row.code !== 0 && <Badge variant="destructive" className="ml-2">Failed</Badge>}</TableCell>
                        <TableCell className="font-medium">{activityContext(row)}</TableCell>
                        <TableCell><code className="monitor-mono">{shortAddress(row.sender, 7)}</code></TableCell>
                        <TableCell><code className="monitor-mono">{Number(row.height).toLocaleString("en-US")}</code></TableCell>
                        <TableCell className="text-muted-foreground">{timeAgo(Date.parse(row.timestamp) / 1000)}</TableCell>
                        <TableCell className="text-right"><a className="monitor-tx-link" href={`${profile.evmExplorer}/tx/${row.txhash}`} target="_blank" rel="noreferrer" title={row.txhash}><ExternalLink size={13} /> {shortAddress(row.txhash, 6)}</a></TableCell>
                      </TableRow>
                    ))}
                  </TableBody>
                </Table>
              )}
            </CardContent>
          </Card>
        </TabsContent>

        <TabsContent value="storage" className="monitor-tab-content">
          <div className="monitor-content-grid monitor-content-grid-single">
            <Card>
              <CardHeader className="monitor-card-header">
                <div>
                  <CardTitle>Gateway probe</CardTitle>
                  <CardDescription>One server-side reachability check for the configured IPFS gateway.</CardDescription>
                </div>
                <CardAction><Badge variant="outline" className={ipfsBadge.className}>{ipfsBadge.label}</Badge></CardAction>
              </CardHeader>
              <CardContent className="monitor-storage-content">
                <div className="monitor-storage-metrics">
                  <div><span>Endpoint</span><code title={cfg.ipfsGateway}>{cfg.ipfsGateway}</code></div>
                  <div><span>HTTP status</span><b>{snapshot.ipfs.statusCode ?? "—"}</b></div>
                  <div><span>Latency</span><b>{snapshot.ipfs.latencyMs == null ? "—" : `${snapshot.ipfs.latencyMs} ms`}</b></div>
                  <div><span>Probe source</span><b>{snapshot.ipfs.probeSource === "server" ? "Website server" : snapshot.ipfs.probeSource === "browser" ? "Browser fallback" : "—"}</b></div>
                  <div><span>Checked</span><b>{formatCheckedAt(snapshot.ipfs.checkedAt)}</b></div>
                </div>
                <Separator />
                <div className="monitor-storage-footer">
                  <span><HardDrive size={15} /> Packfile inspection remains available per repository.</span>
                  <Link to="/ipfs">Open IPFS explorer <ArrowUpRight size={13} /></Link>
                </div>
              </CardContent>
            </Card>
          </div>
        </TabsContent>

        <TabsContent value="migration" className="monitor-tab-content">
          <Alert className="monitor-migration-alert">
            <ArchiveIcon />
            <div>
              <AlertTitle>Legacy CosmWasm V1 is read-only</AlertTitle>
              <AlertDescription>
                The archive remains available at a session snapshot. Migration to EVM V2 is required and is not available from this page.
              </AlertDescription>
            </div>
          </Alert>
          <Card>
            <CardHeader>
              <CardTitle>Archive boundary</CardTitle>
              <CardDescription>Separate source, separate trust root.</CardDescription>
            </CardHeader>
            <CardContent className="monitor-migration-grid">
              <div><span>Network</span><b>{COSMWASM_V1_ARCHIVE.network}</b></div>
              <div><span>Contract</span><code title={COSMWASM_V1_ARCHIVE.contract}>{shortAddress(COSMWASM_V1_ARCHIVE.contract, 10)}</code></div>
              <div><span>Snapshot height</span><b>{formatBlock(snapshot.v1.snapshotHeight)}</b></div>
              <div><span>Access</span><Badge variant="outline" className="monitor-badge-warning">Read-only</Badge></div>
              <Separator className="monitor-migration-separator" />
              <div className="monitor-link-row monitor-migration-links">
                <Link to="/archive/cosmwasm-v1"><ArrowUpRight size={13} /> Open V1 archive viewer</Link>
                <a href={COSMWASM_V1_ARCHIVE.explorer} target="_blank" rel="noreferrer"><ExternalLink size={13} /> View contract on Explorer</a>
              </div>
            </CardContent>
          </Card>
        </TabsContent>
      </Tabs>
    </div>
  );
}
