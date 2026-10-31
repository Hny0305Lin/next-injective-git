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

const NOTICE_VERSION = "mapmonitor-boundary-notice-v2";
/** localStorage key holding the acknowledged notice version. */
export const NOTICE_STORAGE_KEY = "igit-mapmonitor-boundary-notice";

export const BOUNDARY_NOTICE_TITLE = "OpenStreetMap 地图标注提示";

/**
 * Notice body, one entry per paragraph. The wording is verbatim from the
 * compliance text: the source arrives as a single continuous statement, so it
 * is split only at sentence boundaries for readability and not reworded,
 * reordered, or abbreviated.
 */
export const BOUNDARY_NOTICE_PARAGRAPHS: readonly string[] = [
  // 1. Stance and legal scope.
  "iGit 平台坚持一个中国原则，严格遵守中华人民共和国法律法规，以及中国香港特别行政区、中国澳门特别行政区、中国台湾地区及运营所在地的相关规定。",
  // 2. Why OSM diverges, and which regions are affected.
  "MapMonitor 使用 OpenStreetMap（OSM）开源地图数据作为底图，该数据由第三方社区维护，可能存在标注偏差、滞后或错误。目前涉及台湾、藏南、南海诸岛及其附近海域、香港、澳门等区域的主权归属和边界标注问题，以及新疆、黑龙江等边境地区的界线、地名显示偏差。",
  // 3. The substantive statement plus the interim guidance.
  "台湾、藏南、南海诸岛及其附近海域、香港、澳门等均是中国领土不可分割的一部分。在平台切换至符合国家标准的官方地图数据源前，请以国家官方标准地图为准。",
  // 4. Where to report further errors.
  "如您发现其他错误标注，可通过 GitHub Issue 或平台反馈渠道告知我们。",
  // 5. Apology.
  "由此给您带来的不便，我们深表歉意。",
];

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
        </AlertDialogHeader>

        {/* The body is a continuous statement, so each paragraph stands on its
            own line rather than in a bullet list: bullets would imply a
            checklist of separate claims instead of one legal position. */}
        <AlertDialogDescription
          className="space-y-2 text-left text-[0.93rem]! leading-[1.75]!"
          render={<div />}
        >
          {BOUNDARY_NOTICE_PARAGRAPHS.map((paragraph) => (
            <p key={paragraph}>{paragraph}</p>
          ))}
        </AlertDialogDescription>

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
