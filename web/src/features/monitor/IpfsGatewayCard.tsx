import { Clock3, ExternalLink, MapPin, Wifi } from "lucide-react";
import { Badge } from "../../components/ui/badge";
import {
  Card,
  CardAction,
  CardContent,
  CardDescription,
  CardHeader,
  CardTitle,
} from "../../components/ui/card";
import type { IpfsGatewayDefinition, IpfsGatewaySnapshot } from "../../lib/types";
import { sourceBadge } from "./badges";
import { formatCheckedAt } from "./utils";

export function IpfsGatewayCard({
  gateway,
  snapshot,
}: {
  gateway: IpfsGatewayDefinition;
  snapshot: IpfsGatewaySnapshot;
}) {
  const Icon = gateway.icon;
  const status = sourceBadge(snapshot.state, "Reachable");
  const StatusIcon = status.icon;
  return (
    <Card className="monitor-gateway-card">
      <CardHeader className="monitor-card-header">
        <div className="monitor-source-title">
          <span className="monitor-icon-tile"><Icon size={16} /></span>
          <div>
            <CardTitle>{gateway.title}</CardTitle>
            <CardDescription>{gateway.description}</CardDescription>
          </div>
        </div>
        <CardAction>
          <Badge variant="outline" className={status.className}>
            <StatusIcon className={snapshot.state === "loading" ? "animate-spin" : undefined} />
            {status.label}
          </Badge>
        </CardAction>
      </CardHeader>
      <CardContent className="monitor-node-content">
        <div className="monitor-node-meta monitor-gateway-meta">
          <span><MapPin size={13} /> {gateway.region}</span>
          <span><Wifi size={13} /> {snapshot.probeSource === "browser" ? `${snapshot.sampleCount} browser samples` : "Probe pending"}</span>
          <span><Clock3 size={13} /> {formatCheckedAt(snapshot.checkedAt)}</span>
        </div>
        <p className="monitor-node-role">{snapshot.detail}</p>
        <div className="monitor-node-endpoint">
          <code title={gateway.endpoint}>{gateway.endpoint}</code>
          <a href={gateway.href} target="_blank" rel="noreferrer" title={`Open ${gateway.title} endpoint`}>
            <ExternalLink size={13} /> Open gateway
          </a>
        </div>
      </CardContent>
    </Card>
  );
}
