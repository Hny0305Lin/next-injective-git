import { useCallback, useEffect, useState } from "react";
import { ShieldCheck } from "lucide-react";
import {
  AlertDialog,
  AlertDialogAction,
  AlertDialogContent,
  AlertDialogDescription,
  AlertDialogFooter,
  AlertDialogHeader,
  AlertDialogMedia,
  AlertDialogTitle,
} from "@/components/ui/alert-dialog";
import {
  NOTICE_THRESHOLD,
  assessRegion,
  collectRegionEnvironment,
  type RegionAssessment,
} from "../lib/region-notice";

// Boundary-compliance notice for the MapMonitor page.
//
// The page draws OpenStreetMap tiles. Their boundary depiction diverges from
// official mainland-China maps for Taiwan, parts of Tibet/Xinjiang/Heilongjiang
// and the Yellow/East/South China Sea, so visitors arriving from mainland
// China, Hong Kong, Macau, or Taiwan are shown this one-time notice before they
// read the map. Nothing is rendered into the page body itself.
//
// The wording is fixed by compliance requirements: keep it verbatim, including
// the bullet order and the full-width punctuation. `NOTICE_VERSION` must be
// bumped whenever the text changes, so a previously acknowledged visitor is
// shown the new wording.

const NOTICE_VERSION = "mapmonitor-boundary-notice-v1";
/** localStorage key holding the acknowledged notice version. */
export const NOTICE_STORAGE_KEY = "igit-mapmonitor-boundary-notice";

export const BOUNDARY_NOTICE_TITLE = "OpenStreetMap 地图标注提示";

export const BOUNDARY_NOTICE_INTRO =
  "iGit 平台愿意遵守中国大陆和港澳台地区相关法律法规做合法化配合，当前您访问的 MapMonitor 页面正在使用 OpenStreetMap 开源地图引擎，该开源地图引擎存在对中国大陆的地界标注问题，我们确保您正在查看该通知并请您知悉：";

export const BOUNDARY_NOTICE_POINTS: readonly string[] = [
  "台湾是中国大陆的领土组成部分，包括台湾岛、澎湖列岛、金门马祖区域等诸多列岛。",
  "西藏自治区藏南等标注错误的区域是中国大陆的领土组成部分，非他国领土区域。",
  "新疆维吾尔自治区标注错误的区域是中国大陆的领土组成部分，非他国领土区域。",
  "黑龙江省标注错误的区域是中国大陆的领土组成部分，非他国领土区域。",
  "黄海东海南海（OpenStreetMap 里标注为南中国海）标注错误的区域是中国大陆的海域领土组成部分，非他国领土区域。",
  "香港和澳门自古以来是中国大陆的领土。",
];

export const BOUNDARY_NOTICE_OUTRO =
  "我们正在努力适配中国大陆地区访问，因此造成的不便我们深表歉意！";

export const BOUNDARY_NOTICE_ACK = "我已知悉";

function readAcknowledged(): boolean {
  try {
    return localStorage.getItem(NOTICE_STORAGE_KEY) === NOTICE_VERSION;
  } catch {
    // Storage may be blocked (private mode / embedded webview): the notice is
    // then shown every time rather than silently skipped.
    return false;
  }
}

function writeAcknowledged(): void {
  try {
    localStorage.setItem(NOTICE_STORAGE_KEY, NOTICE_VERSION);
  } catch {
    // Best-effort only; the dismissal still applies for this page view.
  }
}

/**
 * Decide whether this visit needs the notice. Pure inputs make the gates
 * explicit: a stored acknowledgment wins, and otherwise the regional
 * assessment must reach NOTICE_THRESHOLD.
 */
export function shouldShowBoundaryNotice(
  assessment: RegionAssessment,
  acknowledged: boolean,
): boolean {
  if (acknowledged) return false;
  return assessment.notice && assessment.total >= NOTICE_THRESHOLD;
}

type NoticeState = "checking" | "hidden" | "shown";

export function MapMonitorBoundaryNotice() {
  const [state, setState] = useState<NoticeState>("checking");

  // The alert dialog closes on its action button, on Escape, and on nothing
  // else (an Alert Dialog always disables pointer dismissal). Every close
  // path records the acknowledgment, so a keyboard dismissal is not shown
  // again on the next visit.
  const acknowledge = useCallback(() => {
    writeAcknowledged();
    setState("hidden");
  }, []);

  // Detect once per mount, after paint: the probes touch canvas/Intl, and a
  // map page should not block its first frame on a compliance check.
  useEffect(() => {
    let cancelled = false;
    const run = () => {
      if (cancelled) return;
      const acknowledged = readAcknowledged();
      const assessment = assessRegion(collectRegionEnvironment());
      setState(shouldShowBoundaryNotice(assessment, acknowledged) ? "shown" : "hidden");
    };
    const handle = window.setTimeout(run, 0);
    return () => {
      cancelled = true;
      window.clearTimeout(handle);
    };
  }, []);

  if (state !== "shown") return null;

  return (
    <AlertDialog
      open
      onOpenChange={(open) => {
        if (!open) acknowledge();
      }}
    >
      <AlertDialogContent
        size="default"
        lang="zh-Hans"
        // styles.css is unlayered legacy CSS, so it outranks Tailwind's
        // `utilities` layer no matter the specificity: the `!` suffixes are
        // required to beat the component's own data-[size] max-width and the
        // element-level `button` reset. Stacking is handled globally (see the
        // alert-dialog rule in styles.css) so the dialog clears the map cards.
        className="max-h-[calc(100dvh-2rem)]! w-[calc(100vw-2rem)]! max-w-[calc(100vw-2rem)]! gap-3 overflow-y-auto text-left sm:w-[560px]! sm:max-w-[560px]!"
      >
        <AlertDialogHeader className="place-items-start gap-2 text-left">
          <AlertDialogMedia className="mb-0 size-9">
            <ShieldCheck aria-hidden="true" />
          </AlertDialogMedia>
          <AlertDialogTitle className="text-[1.15rem]! leading-snug!">{BOUNDARY_NOTICE_TITLE}</AlertDialogTitle>
          <AlertDialogDescription className="text-left text-[0.93rem]! leading-[1.75]!">
            {BOUNDARY_NOTICE_INTRO}
          </AlertDialogDescription>
        </AlertDialogHeader>

        <ul className="list-disc space-y-1.5 pl-5 text-[0.93rem]! leading-[1.75]! text-muted-foreground">
          {BOUNDARY_NOTICE_POINTS.map((point) => (
            <li key={point}>{point}</li>
          ))}
        </ul>

        <p className="text-[0.93rem]! leading-[1.75]! text-muted-foreground">
          {BOUNDARY_NOTICE_OUTRO}
        </p>

        <AlertDialogFooter>
          <AlertDialogAction
            onClick={acknowledge}
            className="h-9! gap-1.5 border-primary! bg-primary! px-3.5! text-primary-foreground! hover:bg-primary/90!"
          >
            <ShieldCheck aria-hidden="true" />
            {BOUNDARY_NOTICE_ACK}
          </AlertDialogAction>
        </AlertDialogFooter>
      </AlertDialogContent>
    </AlertDialog>
  );
}
