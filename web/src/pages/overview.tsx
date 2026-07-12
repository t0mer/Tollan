import { useQuery } from "@tanstack/react-query";
import { CheckCircle2, CircleAlert } from "lucide-react";
import { api } from "@/lib/api";
import { Card, CardContent, CardHeader, CardTitle } from "@/components/ui/card";
import { Badge } from "@/components/ui/badge";

function StatRow({ label, value }: { label: string; value: string }) {
  return (
    <div className="flex items-center justify-between border-b border-border py-2 last:border-0">
      <span className="text-sm text-muted-foreground">{label}</span>
      <span className="font-mono text-sm">{value}</span>
    </div>
  );
}

export function OverviewPage() {
  const health = useQuery({
    queryKey: ["health"],
    queryFn: api.health,
    refetchInterval: 60_000,
  });
  const version = useQuery({ queryKey: ["version"], queryFn: api.version });

  const healthy = health.data?.status === "ok";

  return (
    <div className="mx-auto max-w-5xl space-y-6">
      <div>
        <h1 className="font-display text-2xl font-semibold tracking-tight">Overview</h1>
        <p className="mt-1 text-sm text-muted-foreground">
          Server status and build. Ingest, search and alerting land as later phases
          come online.
        </p>
      </div>

      <div className="grid gap-4 sm:grid-cols-2">
        <Card>
          <CardHeader className="flex-row items-center justify-between">
            <CardTitle>Server</CardTitle>
            {health.isLoading ? (
              <Badge variant="muted">checking…</Badge>
            ) : healthy ? (
              <Badge variant="success">
                <CheckCircle2 className="size-3.5" /> healthy
              </Badge>
            ) : (
              <Badge variant="destructive">
                <CircleAlert className="size-3.5" /> unreachable
              </Badge>
            )}
          </CardHeader>
          <CardContent>
            <StatRow label="Status" value={health.data?.status ?? "—"} />
            <StatRow label="Version" value={health.data?.version ?? "—"} />
          </CardContent>
        </Card>

        <Card>
          <CardHeader>
            <CardTitle>Build</CardTitle>
          </CardHeader>
          <CardContent>
            <StatRow label="Version" value={version.data?.version ?? "—"} />
            <StatRow label="Commit" value={version.data?.commit ?? "—"} />
            <StatRow label="Go" value={version.data?.go ?? "—"} />
          </CardContent>
        </Card>
      </div>
    </div>
  );
}
