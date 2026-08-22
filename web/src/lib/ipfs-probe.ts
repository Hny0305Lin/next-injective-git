export const BROWSER_GATEWAY_PROBE_SAMPLE_COUNT = 3;
export const BROWSER_GATEWAY_PROBE_TIMEOUT_MS = 8_000;

export interface BrowserGatewayProbeResult {
  ok: boolean;
  status: number | null;
  latencyMs: number | null;
  error?: string;
  source: "browser";
  sampleCount: number;
  responseSamples: number;
  reachableSamples: number;
}

interface GatewayProbeSample {
  ok: boolean;
  status: number | null;
  latencyMs: number;
  error?: string;
}

interface BrowserGatewayProbeOptions {
  fetchImpl?: typeof fetch;
  now?: () => number;
  sampleCount?: number;
  timeoutMs?: number;
}

function elapsedMs(started: number, now: () => number) {
  return Math.max(0, Math.round(now() - started));
}

function isAbortError(error: unknown) {
  return error != null
    && typeof error === "object"
    && "name" in error
    && error.name === "AbortError";
}

async function cancelBody(response: Response) {
  try {
    await response.body?.cancel();
  } catch {
    // The response headers are sufficient for this reachability probe.
  }
}

export function medianLatency(values: readonly number[]): number | null {
  if (values.length === 0) return null;
  const sorted = [...values].sort((left, right) => left - right);
  const middle = Math.floor(sorted.length / 2);
  return sorted.length % 2 === 1
    ? sorted[middle]
    : Math.round((sorted[middle - 1] + sorted[middle]) / 2);
}

function modalStatus(values: readonly number[]): number | null {
  if (values.length === 0) return null;
  const counts = new Map<number, number>();
  let selected = values[0];
  let selectedCount = 0;
  for (const value of values) {
    const count = (counts.get(value) ?? 0) + 1;
    counts.set(value, count);
    if (count > selectedCount) {
      selected = value;
      selectedCount = count;
    }
  }
  return selected;
}

async function probeGatewayOnce(
  endpoint: string,
  fetchImpl: typeof fetch,
  now: () => number,
  timeoutMs: number,
): Promise<GatewayProbeSample> {
  const controller = new AbortController();
  const timer = setTimeout(() => controller.abort(), timeoutMs);
  const started = now();
  try {
    let response = await fetchImpl(endpoint, {
      method: "HEAD",
      headers: { Accept: "text/plain" },
      cache: "no-store",
      signal: controller.signal,
    });
    if (response.status === 405 || response.status === 501) {
      await cancelBody(response);
      response = await fetchImpl(endpoint, {
        method: "GET",
        headers: { Accept: "text/plain", Range: "bytes=0-0" },
        cache: "no-store",
        signal: controller.signal,
      });
    }
    await cancelBody(response);
    const status = Number.isInteger(response.status) && response.status >= 100 && response.status <= 599
      ? response.status
      : null;
    return {
      ok: status !== null && status < 500,
      status,
      latencyMs: elapsedMs(started, now),
    };
  } catch (error) {
    return {
      ok: false,
      status: null,
      latencyMs: elapsedMs(started, now),
      error: isAbortError(error) ? "gateway probe timed out" : "gateway probe failed",
    };
  } finally {
    clearTimeout(timer);
  }
}

export async function probeGatewayFromBrowser(
  gateway: string,
  {
    fetchImpl = globalThis.fetch,
    now = () => performance.now(),
    sampleCount = BROWSER_GATEWAY_PROBE_SAMPLE_COUNT,
    timeoutMs = BROWSER_GATEWAY_PROBE_TIMEOUT_MS,
  }: BrowserGatewayProbeOptions = {},
): Promise<BrowserGatewayProbeResult> {
  if (typeof fetchImpl !== "function") throw new Error("fetch is unavailable");
  if (!Number.isInteger(sampleCount) || sampleCount < 1) throw new Error("sample count must be a positive integer");
  const endpoint = `${gateway.replace(/\/+$/, "")}/ipfs/`;
  const samples: GatewayProbeSample[] = [];

  for (let index = 0; index < sampleCount; index += 1) {
    samples.push(await probeGatewayOnce(endpoint, fetchImpl, now, timeoutMs));
  }

  const responses = samples.filter((sample) => sample.status !== null);
  const reachableSamples = samples.filter((sample) => sample.ok).length;
  const error = reachableSamples === sampleCount
    ? undefined
    : responses.length === 0
      ? samples.every((sample) => sample.error === "gateway probe timed out")
        ? "gateway probes timed out"
        : "gateway probes failed"
      : `${reachableSamples}/${sampleCount} browser probes reached the gateway`;

  return {
    ok: reachableSamples === sampleCount,
    status: modalStatus(responses.map((sample) => sample.status as number)),
    latencyMs: medianLatency(responses.map((sample) => sample.latencyMs)),
    error,
    source: "browser",
    sampleCount,
    responseSamples: responses.length,
    reachableSamples,
  };
}
