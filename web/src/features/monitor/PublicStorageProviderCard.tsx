import { ExternalLink, HardDrive, MapPin } from "lucide-react";
import { Badge } from "../../components/ui/badge";
import {
  Card,
  CardAction,
  CardContent,
  CardDescription,
  CardHeader,
  CardTitle,
} from "../../components/ui/card";
import type { PublicStorageProvider } from "../../lib/types";
import { publicProviderBadge } from "./badges";

export function PublicStorageProviderCard({ provider }: { provider: PublicStorageProvider }) {
  const Icon = provider.icon;
  const status = publicProviderBadge(provider.status);
  return (
    <Card className="monitor-node-card">
      <CardHeader className="monitor-card-header">
        <div className="monitor-source-title">
          <span className="monitor-icon-tile"><Icon size={16} /></span>
          <div>
            <CardTitle>{provider.title}</CardTitle>
            <CardDescription>{provider.description}</CardDescription>
          </div>
        </div>
        <CardAction>
          <Badge variant="outline" className={status.className}>{status.label}</Badge>
        </CardAction>
      </CardHeader>
      <CardContent className="monitor-node-content">
        <div className="monitor-node-meta">
          <span><MapPin size={13} /> {provider.region}</span>
          <span><HardDrive size={13} /> {provider.kind}</span>
        </div>
        <p className="monitor-node-role">{provider.role}</p>
        <div className="monitor-node-endpoint">
          <code title={provider.endpoint}>{provider.endpoint}</code>
          <a href={provider.href} target="_blank" rel="noreferrer" title={`Open ${provider.title} endpoint`}>
            <ExternalLink size={13} /> Endpoint
          </a>
        </div>
      </CardContent>
    </Card>
  );
}
