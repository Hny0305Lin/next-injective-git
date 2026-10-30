// Regional-visit detection for the MapMonitor page.
//
// The page renders OpenStreetMap tiles, whose boundary depiction differs from
// mainland-China official maps. Visitors whose browser/OS environment looks
// like mainland China, Hong Kong, Macau, or Taiwan therefore get a one-time
// compliance notice (see MapMonitorBoundaryNotice). Everything here runs
// locally in the browser: no network request, no storage of raw signals, no
// upload. The only thing persisted is the notice version this visitor already
// acknowledged (see NOTICE_VERSION in the component); raw signal values are
// never written anywhere.
//
// Detection is deliberately multi-signal and additive rather than a country
// lookup, because no browser API exposes the visitor's country and a GeoIP
// call would contradict the privacy posture of this page. Signals:
//
//   - UA fingerprint: Chinese-distributed browser brands (360, UC, QQ, Quark,
//     Sogou, Baidu, Maxthon), mainland/handset vendor UA markers, and the
//     Simplified/Traditional-Chinese UI languages of those browsers.
//   - System time zone: Greater-China IANA zones plus the UTC+8 offset that
//     the OS reports (which survives a UTC system clock).
//   - Date formatting: the Intl locale and the rendered date pattern
//     (year/month/day order and separator) — the same class of signal used by
//     commercial abuse scoring to infer locale from formatting.
//   - Typography corroboration: whether the host sets the typographic quotes
//     and ideographs at one em (CJK metrics). This is a WEAK forward signal:
//     every Windows/macOS build ships CJK fonts, so the canvas probes report
//     true for Western visitors too. Measured behaviour on Windows shows the
//     glyph fallback is per-glyph, not per-locale, so these probes cannot by
//     themselves separate a Chinese visitor from a German one; they only
//     corroborate the locale/clock signals.

/** Signals that make up the evidence trail (kept for diagnostics/tests). */
export type RegionSignalId =
  | "browser-language"
  | "user-agent"
  | "timezone"
  | "timezone-offset"
  | "date-format"
  | "intl-locale"
  | "cjk-quotes"
  | "cjk-fonts";

export type RegionSignal = {
  id: RegionSignalId;
  /** Weight this signal contributes when it scores 1. */
  weight: number;
  /** 0..1 — how strongly the signal points at Greater China. */
  score: number;
  /** Human-readable value, for diagnostics only; never persisted. */
  value: string;
};

export type RegionVerdict = "likely-greater-china" | "possibly-greater-china" | "unlikely";

export type RegionAssessment = {
  /** 0..100 weighted total. */
  total: number;
  verdict: RegionVerdict;
  /** Signals that actually hit (score > 0). */
  hits: RegionSignal[];
  /** True when the notice should be shown. */
  notice: boolean;
};

/** Ambient browser facts the scorer reads, so the score stays unit-testable. */
export type RegionEnvironment = {
  /** `navigator.languages` (or a single `navigator.language`). */
  languages: readonly string[];
  userAgent: string;
  /** `navigator.platform`/`userAgentData.platform`, lower-cased. */
  platform?: string;
  /** `Intl.DateTimeFormat().resolvedOptions().timeZone`. */
  timeZone?: string;
  /** `new Date().getTimezoneOffset()` — minutes WEST of UTC, so -480 = UTC+8. */
  timezoneOffsetMinutes?: number;
  /** `Intl.DateTimeFormat().resolvedOptions().locale`. */
  intlLocale?: string;
  /** Rendered numeric date, e.g. "2026/10/30" (from Intl or toLocaleDateString). */
  dateSample?: string;
  /** The host sets “”/‘’ with full-width CJK metrics (canvas probe). */
  cjkQuotationFamily?: boolean;
  /** Rendered typographic quotes, e.g. “…” — used only to detect 「」. */
  quoteSample?: string;
  /** Ideographs resolve at one em each through the generic font stack. */
  cjkFontPresent?: boolean;
};

// --- Weights: browser identity + locale are the strongest tells, followed by
// --- the OS clock, then formatting/typography corroboration.
const WEIGHTS: Record<RegionSignalId, number> = {
  "browser-language": 28,
  "user-agent": 24,
  timezone: 18,
  "timezone-offset": 8,
  "date-format": 7,
  "intl-locale": 6,
  // Weak forward signals: a CJK-capable host is the norm on Windows/macOS and
  // says nothing about where the visitor is, so these only corroborate.
  "cjk-quotes": 5,
  "cjk-fonts": 4,
};

/**
 * Total at or above which the notice is shown (out of 100).
 *
 * Calibrated against the two populations this page must separate: a Western
 * browser on a CJK-capable host scores 8 (font/punctuation corroboration
 * only), Tokyo/Seoul locales score 13, and Singapore-style UTC+8 without a
 * Greater-China zone scores 13. A mainland visitor with an English UI and the
 * Shanghai clock scores 31, so the line sits at 30: above every non-China
 * environment measured, below the weakest real Greater-China visitor.
 */
export const NOTICE_THRESHOLD = 30;

/** IANA zones that mean mainland China, Hong Kong, Macau, or Taiwan. */
export const GREATER_CHINA_TIMEZONES: readonly string[] = [
  // mainland
  "asia/shanghai",
  "asia/chongqing",
  "asia/chungking",
  "asia/harbin",
  "asia/urumqi",
  "asia/kashgar",
  "asia/kashi",
  "prc",
  // Hong Kong / Macau
  "asia/hong_kong",
  "asia/macau",
  "asia/macao",
  // Taiwan
  "asia/taipei",
];

/** Chinese-distributed browser brands that ship a recognisable UA token. */
const CN_BROWSER_UA: readonly { name: string; pattern: RegExp }[] = [
  // 360 安全浏览器 / 360 极速浏览器. The PC shell reports QIHU 360SE /
  // QIHU 360EE and a "360SE" chrome token; the mobile shell reports QihooBrowser.
  { name: "360", pattern: /qihu\s*360se|qihu\s*360ee|qihoobrowser|360se\b|360ee\b/i },
  // UC Browser (PC/Android/iOS) — UBrowser is the internal engine token.
  { name: "UC", pattern: /\bucweb\b|\bubrowser\b|\bucbrowser\b|uc\s*webview|\buc\/[0-9]/i },
  // QQ 浏览器 (mobile) and the QQ/WeChat in-app webviews.
  { name: "QQ", pattern: /\bmqqbrowser\b|qqbrowser|\bqq\/[0-9]|micromessenger|\bqqtheme\b/i },
  // 夸克 (Quark) browser, Alibaba.
  { name: "Quark", pattern: /\bquark\b/i },
  // 搜狗浏览器.
  { name: "Sogou", pattern: /metasr|sogoumobilebrowser|\bse\s*[0-9]+\./i },
  // 百度浏览器 / 百度 App 内置 WebView.
  { name: "Baidu", pattern: /\bbaidubrowser\b|baiduboxapp|\bbdbrowser\b/i },
  // 傲游 (Maxthon).
  { name: "Maxthon", pattern: /\bmaxthon\b/i },
  // 华为 / 小米 / OPPO / vivo / 荣耀 内置浏览器内核标记.
  { name: "CN-Android", pattern: /huaweibrowser|miuibrowser|heytapbrowser|vivobrowser|honorbrowser/i },
];

/** Simplified-Chinese UI tags that only Chinese-locale builds emit. */
const HANS_LANG = /^(zh|zh-cn|zh-sg|zh-hans|zh-hans-cn|zh-hans-sg)$/;
/** Traditional-Chinese UI tags (HK/MO/TW). */
const HANT_LANG = /^(zh-tw|zh-hk|zh-mo|zh-hant|zh-hant-tw|zh-hant-hk|zh-hant-mo)$/;

function normalizeLanguage(tag: string): string {
  return tag.trim().toLowerCase().replace(/_/g, "-");
}

function languageScore(languages: readonly string[]): { score: number; value: string } {
  const list = languages.map(normalizeLanguage).filter((tag) => tag !== "");
  if (list.length === 0) return { score: 0, value: "unknown" };
  const primary = list[0] ?? "";
  const matches = (pattern: RegExp) => (tag: string) => pattern.test(tag);
  if (matches(HANS_LANG)(primary)) return { score: 1, value: primary };
  if (matches(HANT_LANG)(primary)) return { score: 0.7, value: primary };
  // A Chinese UI language anywhere in the preference list still counts.
  if (list.some((tag) => HANS_LANG.test(tag))) return { score: 0.6, value: list.join(", ") };
  if (list.some((tag) => HANT_LANG.test(tag))) return { score: 0.45, value: list.join(", ") };
  if (list.some((tag) => /^zh(-|$)/.test(tag))) return { score: 0.4, value: list.join(", ") };
  return { score: 0, value: list.join(", ") };
}

function userAgentScore(userAgent: string, platform: string): { score: number; value: string } {
  const probe = `${platform} ${userAgent}`;
  const brands = CN_BROWSER_UA.filter((brand) => brand.pattern.test(probe)).map((brand) => brand.name);
  if (brands.length > 0) return { score: 1, value: brands.join(", ") };
  // Windows + a mainland-only shell hint (QQ/WeChat/UC in-app) lands above;
  // anything else keeps its vendor as context for the diagnostics row.
  const vendor = /windows/i.test(probe)
    ? "Windows"
    : /android/i.test(probe)
      ? "Android"
      : /iphone|ipad|ipod|mac os x/i.test(probe)
        ? "Apple"
        : /cros/i.test(probe)
          ? "ChromeOS"
          : /linux/i.test(probe)
            ? "Linux"
            : "unknown";
  return { score: 0, value: vendor };
}

function timeZoneScore(timeZone: string): { score: number; value: string } {
  const zone = timeZone.trim().toLowerCase();
  if (zone === "") return { score: 0, value: "unknown" };
  if (GREATER_CHINA_TIMEZONES.includes(zone)) return { score: 1, value: timeZone };
  // Some engines report a deprecated alias; the prefix is enough context.
  if (/^(asia\/(shanghai|urumqi|hong_kong|macau|taipei)|prc)/.test(zone)) {
    return { score: 1, value: timeZone };
  }
  return { score: 0, value: timeZone };
}

function timezoneOffsetScore(offsetMinutes: number | undefined): { score: number; value: string } {
  if (offsetMinutes == null || !Number.isFinite(offsetMinutes)) return { score: 0, value: "unknown" };
  // getTimezoneOffset() is minutes WEST of UTC: -480 is UTC+8.
  const sign = offsetMinutes <= 0 ? "+" : "-";
  const hours = Math.abs(offsetMinutes) / 60;
  const value = `UTC${sign}${Number.isInteger(hours) ? hours : hours.toFixed(1)}`;
  // Only UTC+8 exactly; UTC+9 (Japan/Korea) must not be folded in.
  return { score: offsetMinutes === -480 ? 0.6 : 0, value };
}

function intlLocaleScore(locale: string): { score: number; value: string } {
  const tag = normalizeLanguage(locale);
  if (tag === "") return { score: 0, value: "unknown" };
  if (HANS_LANG.test(tag)) return { score: 1, value: locale };
  if (HANT_LANG.test(tag)) return { score: 0.75, value: locale };
  if (/^zh(-|$)/.test(tag)) return { score: 0.6, value: locale };
  return { score: 0, value: locale };
}

/**
 * Locale-derived date formatting. CJK locales render year/month/day with
 * either "/" (zh-CN, zh-TW) or 年月日 (zh-CN long form); en-US renders
 * month/day/year and most of Europe day/month/year. A slot-level check keeps
 * this independent of the trailing time part.
 */
function dateFormatScore(dateSample: string | undefined): { score: number; value: string } {
  const sample = (dateSample ?? "").trim();
  if (sample === "") return { score: 0, value: "unknown" };
  if (/[年月日]/.test(sample)) return { score: 0.9, value: sample };
  const numeric = sample.match(/^(\d{1,4})\s*[/.-]\s*(\d{1,2})\s*[/.-]\s*(\d{1,4})/);
  if (numeric != null) {
    const [, first, , third] = numeric;
    // YYYY/MM/DD is the zh-CN and zh-TW numeric order; M/D/YYYY and D/M/YYYY
    // are the Western orders, so only a 4-digit leading year scores.
    if (first.length === 4 && third.length <= 2) return { score: 0.8, value: sample };
    // M/D/YYYY with a 4-digit trailing year is US English: explicit negative.
    if (third.length === 4) return { score: 0, value: sample };
    return { score: 0.4, value: sample };
  }
  return { score: 0, value: sample };
}

function quoteScore(env: RegionEnvironment): { score: number; value: string } {
  // Corner brackets (「」『』) are CJK typography and never appear in a
  // Western UI. Curly quotes (" " ' ') are shared by both, so the rendered
  // pair on its own proves nothing: only the CJK-font probe decides.
  if (env.cjkQuotationFamily === true) return { score: 1, value: "「」『』" };
  const sample = env.quoteSample ?? "";
  if (/[「」『』]/.test(sample)) return { score: 1, value: sample.trim() };
  return { score: 0, value: sample.trim() || "unknown" };
}

function fontScore(env: RegionEnvironment): { score: number; value: string } {
  if (env.cjkFontPresent === true) return { score: 0.7, value: "CJK system font" };
  return { score: 0, value: "none" };
}

/** Score every signal. Pure: the caller supplies the ambient environment. */
export function assessRegion(env: RegionEnvironment): RegionAssessment {
  const language = languageScore(env.languages);
  const ua = userAgentScore(env.userAgent, env.platform ?? "");
  const zone = timeZoneScore(env.timeZone ?? "");
  const offset = timezoneOffsetScore(env.timezoneOffsetMinutes);
  const dateFormat = dateFormatScore(env.dateSample);
  const locale = intlLocaleScore(env.intlLocale ?? "");
  const quotes = quoteScore(env);
  const fonts = fontScore(env);

  const entries: { id: RegionSignalId; score: number; value: string }[] = [
    { id: "browser-language", ...language },
    { id: "user-agent", ...ua },
    { id: "timezone", ...zone },
    { id: "timezone-offset", ...offset },
    { id: "date-format", ...dateFormat },
    { id: "intl-locale", ...locale },
    { id: "cjk-quotes", ...quotes },
    { id: "cjk-fonts", ...fonts },
  ];

  const signals: RegionSignal[] = entries.map((entry) => ({
    id: entry.id,
    weight: WEIGHTS[entry.id],
    score: Math.max(0, Math.min(1, entry.score)),
    value: entry.value,
  }));
  const total = Math.round(signals.reduce((sum, signal) => sum + signal.score * signal.weight, 0));
  const clamped = Math.min(100, total);
  const verdict: RegionVerdict = clamped >= NOTICE_THRESHOLD
    ? "likely-greater-china"
    : clamped >= 15
      ? "possibly-greater-china"
      : "unlikely";
  return {
    total: clamped,
    verdict,
    hits: signals.filter((signal) => signal.score > 0),
    notice: clamped >= NOTICE_THRESHOLD,
  };
}

// --- Browser probes ---------------------------------------------------------

export function browserLanguages(): string[] {
  if (typeof navigator === "undefined") return [];
  const list = Array.isArray(navigator.languages) && navigator.languages.length > 0
    ? Array.from(navigator.languages)
    : navigator.language != null
      ? [navigator.language]
      : [];
  return list.filter((tag): tag is string => typeof tag === "string" && tag !== "");
}

function browserPlatform(): string {
  if (typeof navigator === "undefined") return "";
  const data = (navigator as Navigator & { userAgentData?: { platform?: string } }).userAgentData;
  return `${data?.platform ?? ""} ${navigator.platform ?? ""}`.trim().toLowerCase();
}

function resolvedTimeZone(): string {
  try {
    return Intl.DateTimeFormat().resolvedOptions().timeZone ?? "";
  } catch {
    return "";
  }
}

function resolvedLocale(): string {
  try {
    return Intl.DateTimeFormat().resolvedOptions().locale ?? "";
  } catch {
    return "";
  }
}

// A fixed date through the OS locale: the rendered order/spacing is the
// locale's own choice, independent of the app's language.
function numericDateSample(): string {
  try {
    return new Date(2026, 9, 30).toLocaleDateString();
  } catch {
    return "";
  }
}

// Probe host with a 2D context; null when canvas is unavailable (SSR, very
// old engines, or a blocked canvas).
function canvasContext(): CanvasRenderingContext2D | null {
  if (typeof document === "undefined") return null;
  try {
    return document.createElement("canvas").getContext("2d");
  } catch {
    return null;
  }
}

// Generic families only: this measures the OS font fallback, i.e. what the
// system does with these code points on its own, not the page's typography.
const PROBE_FAMILIES: readonly string[] = ["monospace", "sans-serif", "serif"];

function widthRatio(ctx: CanvasRenderingContext2D, text: string, sizePx: number, family: string): number {
  ctx.font = `${sizePx}px ${family}`;
  return ctx.measureText(text).width / sizePx;
}

// Punctuation/font probe. On a host with any CJK-capable font installed (every
// Windows build ships one) the generic stack resolves U+201C/U+201D to the
// full-width CJK glyphs regardless of the UI language, so glyph metrics alone
// cannot separate a Chinese visitor from a Western one. These helpers therefore
// only answer "does this system carry CJK typography at all" — a weak forward
// signal — and the scoring treats them as such.
export function cjkPunctuationProbe(ctx: CanvasRenderingContext2D): boolean {
  const FULL_WIDTH_PAIR = 1.6;
  const pairs = ["“”", "‘’"];
  return pairs.every((pair) =>
    PROBE_FAMILIES.some((family) => widthRatio(ctx, pair, 100, family) >= FULL_WIDTH_PAIR),
  );
}

// Ideographic glyph metrics: two ideographs render at one em each only when a
// CJK-capable font resolves through the generic stack.
export function cjkFontProbe(ctx: CanvasRenderingContext2D): boolean {
  return PROBE_FAMILIES.some((family) => Math.abs(widthRatio(ctx, "中文", 100, family) - 2) <= 0.05);
}

/** Collect the ambient environment from the current browser. */
export function collectRegionEnvironment(): RegionEnvironment {
  const ctx = canvasContext();
  return {
    languages: browserLanguages(),
    userAgent: typeof navigator === "undefined" ? "" : navigator.userAgent ?? "",
    platform: browserPlatform(),
    timeZone: resolvedTimeZone(),
    timezoneOffsetMinutes: typeof Date === "function" ? new Date().getTimezoneOffset() : undefined,
    intlLocale: resolvedLocale(),
    dateSample: numericDateSample(),
    cjkQuotationFamily: ctx != null ? cjkPunctuationProbe(ctx) : false,
    quoteSample: "\u201c\u201d\u2018\u2019",
    cjkFontPresent: ctx != null ? cjkFontProbe(ctx) : false,
  };
}

/** Convenience: assess the current browser. Never throws. */
export function detectRegion(): RegionAssessment {
  try {
    return assessRegion(collectRegionEnvironment());
  } catch {
    return { total: 0, verdict: "unlikely", hits: [], notice: false };
  }
}
