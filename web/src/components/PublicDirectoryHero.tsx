import { useState } from "react";
import { Compass, Gauge, HardDrive, LayoutDashboard, Settings2 } from "lucide-react";

type HeroVariant = "discover" | "dashboard" | "monitor" | "ipfs" | "settings";

interface PublicDirectoryHeroProps {
  variant?: HeroVariant;
}

const HERO_CONTENT: Record<HeroVariant, { eyebrow: string; title: string; description: string }> = {
  discover: {
    eyebrow: "PUBLIC REPOSITORY DIRECTORY",
    title: "Explore repositories on Injective",
    description: "igit is Git hosting with repository metadata and moderation state anchored in the Injective EVM Suite. Browse public repositories without connecting a wallet.",
  },
  dashboard: {
    eyebrow: "YOUR INJECTIVE WORKSPACE",
    title: "Your repositories, on-chain",
    description: "Create, manage, and browse Git repositories secured by the Injective EVM Suite.",
  },
  monitor: {
    eyebrow: "INJECTIVE NETWORK STATUS",
    title: "Live network health",
    description: "Live health and activity across igit Suites, IPFS gateways, and storage services.",
  },
  ipfs: {
    eyebrow: "PACKFILE INSPECTOR",
    title: "Inspect IPFS content",
    description: "Check packfile availability and audit the content-addressed data referenced by repositories.",
  },
  settings: {
    eyebrow: "NETWORK CONFIGURATION",
    title: "Configure your connection",
    description: "Review the Injective network profile and verify the SuiteDirectory used by this browser.",
  },
};

const HERO_ICONS = {
  discover: Compass,
  dashboard: LayoutDashboard,
  monitor: Gauge,
  ipfs: HardDrive,
  settings: Settings2,
} satisfies Record<HeroVariant, typeof Compass>;

const INJECTIVE_WATERMARKS = [
  { src: "/injective-brand/icon-ocean.svg", kind: "icon" },
  { src: "/injective-brand/icon-background-ocean.svg", kind: "icon-background" },
  { src: "/injective-brand/logo-ocean.svg", kind: "logo" },
  { src: "/injective-brand/logotype-ocean.svg", kind: "logotype" },
] as const;

const WATERMARK_SESSION_KEY = "igit.injective-watermark.previous-kind";
let previousWatermarkKind: string | null = null;

function chooseWatermark() {
  let previous = previousWatermarkKind;
  try {
    previous = window.sessionStorage.getItem(WATERMARK_SESSION_KEY) ?? previous;
  } catch {
    // Keep the in-memory value when session storage is unavailable.
  }

  const candidates = INJECTIVE_WATERMARKS.filter((watermark) => watermark.kind !== previous);
  const selected = candidates[Math.floor(Math.random() * candidates.length)] ?? INJECTIVE_WATERMARKS[0];
  previousWatermarkKind = selected.kind;
  try {
    window.sessionStorage.setItem(WATERMARK_SESSION_KEY, selected.kind);
  } catch {
    // The in-memory value still prevents consecutive duplicates this session.
  }
  return selected;
}

export function PublicDirectoryHero({ variant = "discover" }: PublicDirectoryHeroProps) {
  const content = HERO_CONTENT[variant];
  const Icon = HERO_ICONS[variant];
  const [watermark] = useState(chooseWatermark);

  return (
    <section className="discover-hero public-directory-hero" aria-label={content.eyebrow}>
      <div>
        <p className="eyebrow">{content.eyebrow}</p>
        <h1>{content.title}</h1>
        <p>{content.description}</p>
      </div>
      <div className="discover-hero-icon"><Icon size={22} aria-hidden="true" /></div>
      <img
        className={`injective-watermark injective-watermark-${watermark.kind}`}
        src={watermark.src}
        alt=""
        aria-hidden="true"
      />
    </section>
  );
}
