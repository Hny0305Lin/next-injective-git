import type { LucideIcon } from "lucide-react";
import { Card, CardContent } from "../../components/ui/card";
import { Skeleton } from "../../components/ui/skeleton";

export function MetricCard({
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
