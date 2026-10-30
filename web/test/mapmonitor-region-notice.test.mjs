import assert from "node:assert/strict";
import test from "node:test";

import {
  GREATER_CHINA_TIMEZONES,
  NOTICE_THRESHOLD,
  assessRegion,
} from "../src/lib/region-notice.ts";

// Realistic Chinese-visitor environment: Simplified-Chinese UI, UTC+8 OS clock,
// zh-CN Intl locale and Y/M/D date rendering.
function mainlandVisitor(overrides = {}) {
  return {
    languages: ["zh-CN", "zh", "en"],
    userAgent:
      "Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/120.0.0.0 Safari/537.36",
    platform: "win32",
    timeZone: "Asia/Shanghai",
    timezoneOffsetMinutes: -480,
    intlLocale: "zh-CN",
    dateSample: "2026/10/30",
    cjkQuotationFamily: true,
    quoteSample: "\u201c\u201d\u2018\u2019",
    cjkFontPresent: true,
    ...overrides,
  };
}

function westernVisitor(overrides = {}) {
  return {
    languages: ["en-US", "en"],
    userAgent:
      "Mozilla/5.0 (Macintosh; Intel Mac OS X 10_15_7) AppleWebKit/605.1.15 (KHTML, like Gecko) Version/17.0 Safari/605.1.15",
    platform: "macintel",
    timeZone: "America/New_York",
    timezoneOffsetMinutes: 240,
    intlLocale: "en-US",
    dateSample: "10/30/2026",
    cjkQuotationFamily: false,
    quoteSample: "\u201c\u201d\u2018\u2019",
    cjkFontPresent: false,
    ...overrides,
  };
}

test("a mainland-China browser environment triggers the notice", () => {
  const result = assessRegion(mainlandVisitor());
  assert.ok(result.total >= NOTICE_THRESHOLD, `total ${result.total} should reach ${NOTICE_THRESHOLD}`);
  assert.equal(result.notice, true);
  assert.equal(result.verdict, "likely-greater-china");
  const ids = result.hits.map((signal) => signal.id);
  assert.ok(ids.includes("browser-language"));
  assert.ok(ids.includes("timezone"));
  assert.ok(ids.includes("date-format"));
});

test("a Western browser environment never triggers the notice", () => {
  const result = assessRegion(westernVisitor());
  assert.equal(result.notice, false);
  assert.equal(result.verdict, "unlikely");
  assert.equal(result.hits.length, 0);
});

test("Hong Kong, Macau, and Taiwan visitors are covered", () => {
  for (const zone of ["Asia/Hong_Kong", "Asia/Macau", "Asia/Taipei"]) {
    const result = assessRegion(
      mainlandVisitor({ timeZone: zone, languages: ["zh-HK", "zh", "en"], intlLocale: "zh-HK" }),
    );
    assert.ok(result.notice, `${zone} should trigger the notice (total ${result.total})`);
  }
  assert.ok(GREATER_CHINA_TIMEZONES.includes("asia/hong_kong"));
  assert.ok(GREATER_CHINA_TIMEZONES.includes("asia/macau"));
  assert.ok(GREATER_CHINA_TIMEZONES.includes("asia/taipei"));
});

test("Chinese-distributed browser UAs are detected", () => {
  const brands = {
    "360": "Mozilla/5.0 (Windows NT 10.0; WOW64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/86.0.4240.198 Safari/537.36 QIHU 360SE",
    UC: "Mozilla/5.0 (Linux; U; Android 13) AppleWebKit/537.36 (KHTML, like Gecko) Version/4.0 Chrome/100.0 Mobile Safari/537.36 UCBrowser/15.2.1.1319",
    QQ: "Mozilla/5.0 (Linux; U; Android 12) AppleWebKit/537.36 (KHTML, like Gecko) Version/4.0 Chrome/109.0 Mobile Safari/537.36 MQQBrowser/14.6",
    Quark: "Mozilla/5.0 (Linux; U; Android 13) AppleWebKit/537.36 (KHTML, like Gecko) Version/4.0 Chrome/110.0 Mobile Safari/537.36 Quark/6.3.0.320",
    WeChat: "Mozilla/5.0 (iPhone; CPU iPhone OS 17_0 like Mac OS X) AppleWebKit/605.1.15 MicroMessenger/8.0.40",
  };
  for (const [brand, userAgent] of Object.entries(brands)) {
    const result = assessRegion(
      mainlandVisitor({
        languages: ["zh-CN"],
        userAgent,
        timeZone: "UTC",
        timezoneOffsetMinutes: 0,
        intlLocale: "en-US",
        dateSample: "10/30/2026",
        cjkFontPresent: false,
      }),
    );
    const ua = result.hits.find((signal) => signal.id === "user-agent");
    assert.ok(ua != null, `${brand} UA should be detected`);
    assert.ok(ua.score >= 1, `${brand} UA should score full weight (got ${ua.score})`);
    assert.ok(result.notice, `${brand} UA with a zh-CN UI should still trigger the notice`);
  }
});

test("ordinary Chrome/Safari/Edge UAs are not mistaken for Chinese browsers", () => {
  for (const userAgent of [
    "Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/122.0.0.0 Safari/537.36",
    "Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/122.0.0.0 Safari/537.36 Edg/122.0.0.0",
    "Mozilla/5.0 (Macintosh; Intel Mac OS X 10_15_7) AppleWebKit/605.1.15 (KHTML, like Gecko) Version/17.2 Safari/605.1.15",
    "Mozilla/5.0 (X11; Linux x86_64; rv:121.0) Gecko/20100101 Firefox/121.0",
  ]) {
    const result = assessRegion(westernVisitor({ userAgent }));
    const ua = result.hits.find((signal) => signal.id === "user-agent");
    assert.equal(ua, undefined, `${userAgent.slice(0, 40)}… must not be a UA hit`);
  }
});

test("a UTC clock alone does not hide a mainland visitor", () => {
  // Travellers and CI images often force UTC; the language, Intl locale, date
  // order, and CJK typography still carry the regional signal.
  const result = assessRegion(
    mainlandVisitor({ timeZone: "UTC", timezoneOffsetMinutes: 0 }),
  );
  assert.equal(result.notice, true);
});

test("a Chinese-language UI without a China clock or fonts is still a hit", () => {
  const result = assessRegion(
    westernVisitor({ languages: ["zh-CN"], intlLocale: "zh-CN", dateSample: "2026/10/30" }),
  );
  // Language (26) + Intl locale (6) + date order (~5.6) is over the threshold:
  // a visitor who deliberately runs a zh-CN UI elsewhere is treated as Greater
  // China, which errs on the side of showing the notice.
  assert.equal(result.notice, true);
  assert.ok(result.total >= NOTICE_THRESHOLD);
});

test("a CJK font alone (no Chinese locale or clock) stays quiet", () => {
  const result = assessRegion(westernVisitor({ cjkFontPresent: true }));
  const fonts = result.hits.find((signal) => signal.id === "cjk-fonts");
  assert.ok(fonts != null);
  assert.ok(fonts.score * fonts.weight < NOTICE_THRESHOLD);
  assert.equal(result.notice, false);
});

test("UTC+8 is the only offset that scores; UTC+9 does not", () => {
  const tokyo = assessRegion(
    westernVisitor({ languages: ["ja-JP", "ja"], timeZone: "Asia/Tokyo", timezoneOffsetMinutes: -540 }),
  );
  const offsetHit = tokyo.hits.find((signal) => signal.id === "timezone-offset");
  assert.equal(offsetHit, undefined);
  assert.equal(tokyo.notice, false);
});

test("date order is read from the rendered sample, not from a guess", () => {
  const cjkLong = assessRegion(westernVisitor({ dateSample: "2026年10月30日" }));
  const ymd = assessRegion(westernVisitor({ dateSample: "2026/10/30" }));
  const us = assessRegion(westernVisitor({ dateSample: "10/30/2026" }));
  const eur = assessRegion(westernVisitor({ dateSample: "30.10.2026" }));
  const score = (r) => r.hits.find((signal) => signal.id === "date-format")?.score ?? 0;
  assert.ok(score(cjkLong) > score(ymd));
  assert.ok(score(ymd) > score(eur));
  assert.equal(score(us), 0);
});

test("scores are clamped and weights add up to 100", () => {
  const full = assessRegion(mainlandVisitor());
  assert.ok(full.total <= 100);
  // Every signal at full strength must be reachable within the cap.
  const maxed = assessRegion(
    mainlandVisitor({
      languages: ["zh-CN"],
      userAgent: "MQQBrowser/14.6",
      timeZone: "Asia/Shanghai",
      timezoneOffsetMinutes: -480,
      intlLocale: "zh-CN",
      dateSample: "2026年10月30日",
      cjkQuotationFamily: true,
      cjkFontPresent: true,
    }),
  );
  assert.ok(maxed.total > 90, `expected a near-maximum score, got ${maxed.total}`);
  assert.ok(maxed.total <= 100);
});

test("missing environment fields degrade to a quiet verdict instead of throwing", () => {
  const result = assessRegion({
    languages: [],
    userAgent: "",
    timeZone: "",
    intlLocale: "",
    dateSample: "",
  });
  assert.equal(result.notice, false);
  assert.equal(result.hits.length, 0);
  assert.equal(result.total, 0);
});

// --- Notice component wiring -------------------------------------------------

async function source(path) {
  const { readFile } = await import("node:fs/promises");
  return readFile(new URL(path, import.meta.url), "utf8");
}

test("the notice text is verbatim and the compliance bullets are in order", async () => {
  const component = await source("../src/components/mapmonitor-boundary-notice.tsx");
  assert.match(
    component,
    /iGit 平台愿意遵守中国大陆和港澳台地区相关法律法规做合法化配合，当前您访问的 MapMonitor 页面正在使用 OpenStreetMap 开源地图引擎，该开源地图引擎存在对中国大陆的地界标注问题，我们确保您正在查看该通知并请您知悉：/,
  );
  const bullets = [
    "台湾是中国大陆的领土组成部分，包括台湾岛、澎湖列岛、金门马祖区域等诸多列岛。",
    "西藏自治区藏南等标注错误的区域是中国大陆的领土组成部分，非他国领土区域。",
    "新疆维吾尔自治区标注错误的区域是中国大陆的领土组成部分，非他国领土区域。",
    "黑龙江省标注错误的区域是中国大陆的领土组成部分，非他国领土区域。",
    "黄海东海南海（OpenStreetMap 里标注为南中国海）标注错误的区域是中国大陆的海域领土组成部分，非他国领土区域。",
    "香港和澳门自古以来是中国大陆的领土。",
  ];
  for (const bullet of bullets) assert.ok(component.includes(bullet), `missing bullet: ${bullet}`);
  // Order matters: Taiwan first, Hong Kong/Macau last.
  const order = bullets.map((bullet) => component.indexOf(bullet));
  assert.deepEqual(order, [...order].sort((a, b) => a - b));
  assert.ok(component.includes("我们正在努力适配中国大陆地区访问，因此造成的不便我们深表歉意！"));
});

test("the notice renders only as a dialog, never into the page body", async () => {
  const component = await source("../src/components/mapmonitor-boundary-notice.tsx");
  // Built on the shadcn/base-ui Alert Dialog primitive, which supplies
  // role="alertdialog", aria-modal, focus trapping, scroll locking, and the
  // "backdrop click does not dismiss" rule (Base UI forces
  // disablePointerDismissal for alert dialogs).
  assert.match(component, /from "@\/components\/ui\/alert-dialog"/);
  assert.match(component, /<AlertDialog\s/);
  assert.match(component, /<AlertDialogContent/);
  assert.match(component, /<AlertDialogTitle\s/);
  assert.match(component, /<AlertDialogDescription/);
  // Every close path records the acknowledgment (action button and Escape,
  // which Base UI routes through onOpenChange).
  assert.match(component, /onOpenChange=\{\(open\) => \{[\s\S]*?if \(!open\) acknowledge\(\);/);
  assert.match(component, /<AlertDialogAction[\s\S]*?onClick=\{acknowledge\}/);
  // The dismissal is versioned so changed wording is shown again.
  assert.match(component, /const NOTICE_VERSION = "mapmonitor-boundary-notice-v1"/);
  assert.match(component, /localStorage\.setItem\(NOTICE_STORAGE_KEY, NOTICE_VERSION\)/);
  // Detection is local-only: no fetch/XHR in the detector or the component.
  for (const file of [component, await source("../src/lib/region-notice.ts")]) {
    assert.doesNotMatch(file, /fetch\(|XMLHttpRequest|sendBeacon|navigator\.geolocation/);
  }
});

test("MapMonitor mounts the notice component", async () => {
  const page = await source("../src/pages/MapMonitor.tsx");
  assert.match(page, /import \{ MapMonitorBoundaryNotice \} from "\.\.\/components\/mapmonitor-boundary-notice"/);
  assert.match(page, /<MapMonitorBoundaryNotice \/>/);
});

test("the threshold keeps every measured non-China environment quiet", () => {
  // Values observed in real browsers against the dev server; the calmest
  // Greater-China visitor (English UI, Shanghai clock) scores 31, so the line
  // must sit below that and above every one of these.
  const nonChina = [
    { label: "en-US / New York", env: westernVisitor() },
    { label: "ja-JP / Tokyo", env: westernVisitor({ languages: ["ja-JP", "ja"], timeZone: "Asia/Tokyo", timezoneOffsetMinutes: -540, intlLocale: "ja-JP", dateSample: "2026/10/30" }) },
    { label: "ko-KR / Seoul", env: westernVisitor({ languages: ["ko-KR"], timeZone: "Asia/Seoul", timezoneOffsetMinutes: -540, intlLocale: "ko-KR", dateSample: "2026/10/30" }) },
    { label: "en-SG / Singapore", env: westernVisitor({ timeZone: "Asia/Singapore", timezoneOffsetMinutes: -480, dateSample: "30/10/2026" }) },
  ];
  for (const { label, env } of nonChina) {
    const result = assessRegion(env);
    assert.equal(result.notice, false, `${label} must stay quiet (scored ${result.total})`);
    assert.ok(result.total < NOTICE_THRESHOLD, `${label} scored ${result.total}`);
  }
});

test("an English-locale visitor on a Greater-China clock is still caught", () => {
  // Expat / dual-boot case: the strongest remaining evidence is the OS clock.
  // Real browsers on Windows/macOS report the CJK typography probes as true
  // (the host ships CJK fonts), which is the combination measured in-browser.
  for (const zone of ["Asia/Shanghai", "Asia/Hong_Kong"]) {
    const result = assessRegion(
      westernVisitor({
        timeZone: zone,
        timezoneOffsetMinutes: -480,
        cjkFontPresent: true,
        cjkQuotationFamily: true,
      }),
    );
    assert.equal(result.notice, true, `${zone} clock should trigger the notice (scored ${result.total})`);
  }
  // Without those corroborating probes the same clock stays below the line,
  // which is the documented limitation of glyph-based typography probes.
  const bare = assessRegion(westernVisitor({ timeZone: "Asia/Shanghai", timezoneOffsetMinutes: -480 }));
  assert.equal(bare.notice, false);
});

test("a CJK-capable host alone is not evidence of Greater China", () => {
  // Windows and macOS ship CJK fonts everywhere; that must never be enough.
  const result = assessRegion(westernVisitor({ cjkFontPresent: true, cjkQuotationFamily: true }));
  assert.equal(result.notice, false);
  assert.ok(result.total < NOTICE_THRESHOLD);
});

test("the alert-dialog primitive is the project's shadcn base-nova component", async () => {
  const primitive = await source("../src/components/ui/alert-dialog.tsx");
  assert.match(primitive, /from "@base-ui\/react\/alert-dialog"/);
  assert.match(primitive, /data-slot="alert-dialog-content"/);
  assert.match(primitive, /data-slot="alert-dialog-overlay"/);
  assert.match(primitive, /data-slot="alert-dialog-action"/);
  // Theme tokens, not hardcoded colors: the dialog follows the app theme.
  assert.match(primitive, /bg-popover/);
  assert.match(primitive, /text-popover-foreground/);
});
