import { AlertTriangle, CheckCircle2, LoaderCircle, XCircle } from "lucide-react";
import type { SourceState, PublicProviderStatus } from "../../lib/types";

export function sourceBadge(state: SourceState, healthyLabel = "Healthy") {
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

export function publicProviderBadge(status: PublicProviderStatus) {
  return status === "current"
    ? { label: "Current path", className: "monitor-badge-healthy" }
    : { label: "Legacy copy", className: "monitor-badge-warning" };
}
