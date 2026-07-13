import { useState } from "react";
import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import { Pencil, Server, Trash2 } from "lucide-react";
import { agents, type FleetAgent, type FleetAgentConfig } from "@/lib/api";
import { Button } from "@/components/ui/button";
import { Badge } from "@/components/ui/badge";
import { Card, CardContent } from "@/components/ui/card";
import { Modal, Field, inputClass } from "@/components/ui/modal";

function online(lastSeen: string): boolean {
  return Date.now() - new Date(lastSeen).getTime() < 90_000;
}

export function FleetPage() {
  const qc = useQueryClient();
  const list = useQuery({ queryKey: ["agents"], queryFn: agents.list, refetchInterval: 30_000 });
  const [editing, setEditing] = useState<FleetAgent | null>(null);
  const del = useMutation({
    mutationFn: (id: string) => agents.remove(id),
    onSuccess: () => qc.invalidateQueries({ queryKey: ["agents"] }),
  });

  return (
    <div className="mx-auto max-w-5xl space-y-6">
      <div>
        <h1 className="font-display text-2xl font-semibold tracking-tight">Fleet</h1>
        <p className="mt-1 text-sm text-muted-foreground">
          tollan-agent collectors and their centralized configuration.
        </p>
      </div>

      <Card>
        <CardContent className="p-0">
          <table className="w-full text-left text-sm">
            <thead>
              <tr className="border-b border-border text-xs uppercase tracking-wide text-muted-foreground">
                <th className="p-3 font-medium">Host</th>
                <th className="p-3 font-medium">OS</th>
                <th className="p-3 font-medium">Tags</th>
                <th className="p-3 font-medium">Shipped</th>
                <th className="p-3 font-medium">Cfg</th>
                <th className="p-3 font-medium">Status</th>
                <th className="p-3" />
              </tr>
            </thead>
            <tbody>
              {list.data?.map((a) => (
                <tr key={a.id} className="border-b border-border last:border-0">
                  <td className="p-3 font-medium">{a.hostname}</td>
                  <td className="p-3 font-mono text-xs">{a.os}</td>
                  <td className="p-3">
                    <div className="flex flex-wrap gap-1">
                      {a.tags.map((t) => (
                        <Badge key={t} variant="muted">
                          {t}
                        </Badge>
                      ))}
                    </div>
                  </td>
                  <td className="p-3 font-mono text-xs tabular-nums">{a.shipped.toLocaleString()}</td>
                  <td className="p-3 font-mono text-xs">v{a.config_version}</td>
                  <td className="p-3">
                    <Badge variant={online(a.last_seen) ? "success" : "muted"}>
                      {online(a.last_seen) ? "online" : "offline"}
                    </Badge>
                  </td>
                  <td className="p-3 text-right">
                    <button onClick={() => setEditing(a)} className="rounded p-1 text-muted-foreground hover:bg-muted">
                      <Pencil className="size-4" />
                    </button>
                    <button onClick={() => del.mutate(a.id)} className="rounded p-1 text-muted-foreground hover:text-destructive">
                      <Trash2 className="size-4" />
                    </button>
                  </td>
                </tr>
              ))}
            </tbody>
          </table>
          {list.data && list.data.length === 0 && (
            <div className="p-10 text-center text-sm text-muted-foreground">
              <Server className="mx-auto mb-2 size-6 opacity-50" />
              No agents enrolled. Install <code className="font-mono">tollan-agent</code> and point it at this server.
            </div>
          )}
        </CardContent>
      </Card>

      {editing && (
        <AgentEditor
          agent={editing}
          onClose={() => setEditing(null)}
          onSaved={() => {
            setEditing(null);
            qc.invalidateQueries({ queryKey: ["agents"] });
          }}
        />
      )}
    </div>
  );
}

function AgentEditor({ agent, onClose, onSaved }: { agent: FleetAgent; onClose: () => void; onSaved: () => void }) {
  const [tags, setTags] = useState(agent.tags.join(", "));
  const [paths, setPaths] = useState((agent.config.files?.[0]?.paths ?? []).join("\n"));
  const [journald, setJournald] = useState(agent.config.journald);

  const save = useMutation({
    mutationFn: () => {
      const config: FleetAgentConfig = {
        version: agent.config.version,
        files: [{ paths: paths.split("\n").map((p) => p.trim()).filter(Boolean) }],
        journald,
        windows_event_log: agent.config.windows_event_log,
      };
      return agents.update(agent.id, {
        tags: tags.split(",").map((t) => t.trim()).filter(Boolean),
        config,
      });
    },
    onSuccess: onSaved,
  });

  return (
    <Modal title={`Configure ${agent.hostname}`} onClose={onClose} wide>
      <form
        className="space-y-3"
        onSubmit={(e) => {
          e.preventDefault();
          save.mutate();
        }}
      >
        <Field label="Tags (comma-separated)">
          <input className={inputClass} value={tags} onChange={(e) => setTags(e.target.value)} />
        </Field>
        <Field label="File globs to tail (one per line)">
          <textarea
            className="min-h-24 w-full rounded-md border border-input bg-background px-3 py-2 font-mono text-sm outline-none focus-visible:ring-2 focus-visible:ring-ring"
            placeholder={"/var/log/*.log\n/var/log/nginx/access.log"}
            value={paths}
            onChange={(e) => setPaths(e.target.value)}
          />
        </Field>
        <label className="flex items-center gap-2 text-sm">
          <input type="checkbox" checked={journald} onChange={(e) => setJournald(e.target.checked)} />
          Collect journald (Linux)
        </label>
        <p className="text-xs text-muted-foreground">
          The agent applies this configuration on its next heartbeat.
        </p>
        <div className="flex justify-end gap-2">
          <Button type="button" variant="ghost" onClick={onClose}>
            Cancel
          </Button>
          <Button type="submit" disabled={save.isPending}>
            Save
          </Button>
        </div>
      </form>
    </Modal>
  );
}
