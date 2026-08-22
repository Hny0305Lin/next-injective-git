import type { ReactNode } from "react";
import type { LucideIcon } from "lucide-react";
import { Badge } from "../../components/ui/badge";
import {
  Card,
  CardAction,
  CardContent,
  CardDescription,
  CardHeader,
  CardTitle,
} from "../../components/ui/card";
import type { SourceSnapshot } from "../../lib/types";
import { sourceBadge } from "./badges";

export function SourceCard({
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
