import { useQuery } from "@tanstack/react-query";
import { api } from "@/lib/api";
import { Card, CardContent, CardHeader, CardTitle } from "@/components/ui/card";
import { Badge } from "@/components/ui/badge";

export function InputsPage() {
  const { data, isLoading } = useQuery({
    queryKey: ["inputs"],
    queryFn: api.inputs,
    refetchInterval: 60_000,
  });

  return (
    <div className="mx-auto max-w-5xl space-y-6">
      <div>
        <h1 className="font-display text-2xl font-semibold tracking-tight">Inputs</h1>
        <p className="mt-1 text-sm text-muted-foreground">
          Listeners receiving logs. Runtime create/edit arrives in a later phase.
        </p>
      </div>

      <Card>
        <CardHeader>
          <CardTitle>Configured inputs</CardTitle>
        </CardHeader>
        <CardContent>
          {isLoading ? (
            <p className="text-sm text-muted-foreground">Loading…</p>
          ) : !data || data.length === 0 ? (
            <p className="text-sm text-muted-foreground">No inputs configured.</p>
          ) : (
            <table className="w-full text-left text-sm">
              <thead>
                <tr className="border-b border-border text-xs uppercase tracking-wide text-muted-foreground">
                  <th className="py-2 pr-4 font-medium">ID</th>
                  <th className="py-2 pr-4 font-medium">Type</th>
                  <th className="py-2 pr-4 font-medium">Bind</th>
                  <th className="py-2 pr-4 font-medium">Protocol</th>
                  <th className="py-2 font-medium">Status</th>
                </tr>
              </thead>
              <tbody>
                {data.map((i) => (
                  <tr key={i.id} className="border-b border-border last:border-0">
                    <td className="py-2 pr-4 font-mono text-xs">{i.id}</td>
                    <td className="py-2 pr-4">{i.type}</td>
                    <td className="py-2 pr-4 font-mono text-xs">{i.bind}</td>
                    <td className="py-2 pr-4 font-mono text-xs uppercase">{i.protocol}</td>
                    <td className="py-2">
                      <Badge variant={i.running ? "success" : "muted"}>
                        {i.running ? "running" : "stopped"}
                      </Badge>
                    </td>
                  </tr>
                ))}
              </tbody>
            </table>
          )}
        </CardContent>
      </Card>
    </div>
  );
}
