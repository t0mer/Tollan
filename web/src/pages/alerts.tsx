import { useState } from "react";
import { Link } from "react-router-dom";
import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import { Bell, Pencil, Plus, Settings2, Trash2 } from "lucide-react";
import { channels, eventDefinitions, api, type EventDefinition } from "@/lib/api";
import { Button } from "@/components/ui/button";
import { Badge } from "@/components/ui/badge";
import { Card, CardContent, CardHeader, CardTitle } from "@/components/ui/card";
import { Modal, Field, inputClass, selectClass } from "@/components/ui/modal";

function emptyDef(): EventDefinition {
  return {
    name: "", enabled: true, type: "filter", query: "level:error",
    window_seconds: 300, threshold: 5, channels: [], grace_seconds: 300, backlog: 5,
  };
}

export function AlertsPage() {
  const qc = useQueryClient();
  const defs = useQuery({ queryKey: ["eventDefs"], queryFn: eventDefinitions.list });
  const events = useQuery({ queryKey: ["events"], queryFn: api.events, refetchInterval: 30_000 });
  const [editing, setEditing] = useState<EventDefinition | null>(null);
  const del = useMutation({
    mutationFn: (id: string) => eventDefinitions.remove(id),
    onSuccess: () => qc.invalidateQueries({ queryKey: ["eventDefs"] }),
  });

  return (
    <div className="mx-auto max-w-5xl space-y-6">
      <div className="flex items-center justify-between">
        <div>
          <h1 className="font-display text-2xl font-semibold tracking-tight">Alerts</h1>
          <p className="mt-1 text-sm text-muted-foreground">Event definitions and their firings.</p>
        </div>
        <div className="flex gap-2">
          <Button variant="outline" asChild>
            <Link to="/notifications">
              <Settings2 className="size-4" /> Channels
            </Link>
          </Button>
          <Button onClick={() => setEditing(emptyDef())}>
            <Plus className="size-4" /> New event
          </Button>
        </div>
      </div>

      <Card>
        <CardHeader>
          <CardTitle>Event definitions</CardTitle>
        </CardHeader>
        <CardContent className="p-0">
          <table className="w-full text-left text-sm">
            <thead>
              <tr className="border-b border-border text-xs uppercase tracking-wide text-muted-foreground">
                <th className="p-3 font-medium">Name</th>
                <th className="p-3 font-medium">Type</th>
                <th className="p-3 font-medium">Condition</th>
                <th className="p-3 font-medium">Status</th>
                <th className="p-3" />
              </tr>
            </thead>
            <tbody>
              {defs.data?.map((d) => (
                <tr key={d.id} className="border-b border-border last:border-0">
                  <td className="p-3 font-medium">{d.name}</td>
                  <td className="p-3 font-mono text-xs">{d.type}</td>
                  <td className="p-3 font-mono text-xs text-muted-foreground">
                    {d.query || "*"} ≥ {d.threshold} / {d.window_seconds}s
                  </td>
                  <td className="p-3">
                    <Badge variant={d.enabled ? "success" : "muted"}>{d.enabled ? "on" : "off"}</Badge>
                  </td>
                  <td className="p-3 text-right">
                    <button onClick={() => setEditing(d)} className="rounded p-1 text-muted-foreground hover:bg-muted">
                      <Pencil className="size-4" />
                    </button>
                    <button onClick={() => d.id && del.mutate(d.id)} className="rounded p-1 text-muted-foreground hover:text-destructive">
                      <Trash2 className="size-4" />
                    </button>
                  </td>
                </tr>
              ))}
            </tbody>
          </table>
          {defs.data && defs.data.length === 0 && (
            <div className="p-6 text-center text-sm text-muted-foreground">No event definitions yet.</div>
          )}
        </CardContent>
      </Card>

      <Card>
        <CardHeader>
          <CardTitle>Recent events</CardTitle>
        </CardHeader>
        <CardContent>
          {events.data && events.data.length > 0 ? (
            <ul className="space-y-2">
              {events.data.map((e) => (
                <li key={e.id} className="flex items-start gap-3 border-b border-border pb-2 last:border-0">
                  <Bell className="mt-0.5 size-4 shrink-0 text-warning" />
                  <div className="min-w-0">
                    <div className="flex items-center gap-2">
                      <span className="text-sm font-medium">{e.definition_name}</span>
                      <span className="font-mono text-xs text-muted-foreground">
                        {new Date(e.fired_at).toISOString().replace("T", " ").slice(0, 19)}
                      </span>
                    </div>
                    <div className="whitespace-pre-wrap font-mono text-xs text-muted-foreground">{e.message}</div>
                  </div>
                </li>
              ))}
            </ul>
          ) : (
            <p className="text-sm text-muted-foreground">No events have fired.</p>
          )}
        </CardContent>
      </Card>

      {editing && (
        <EventEditor
          initial={editing}
          onClose={() => setEditing(null)}
          onSaved={() => {
            setEditing(null);
            qc.invalidateQueries({ queryKey: ["eventDefs"] });
          }}
        />
      )}
    </div>
  );
}

function EventEditor({ initial, onClose, onSaved }: { initial: EventDefinition; onClose: () => void; onSaved: () => void }) {
  const [d, setD] = useState<EventDefinition>(initial);
  const chList = useQuery({ queryKey: ["channels"], queryFn: channels.list });
  const save = useMutation({
    mutationFn: () => (d.id ? eventDefinitions.update(d.id, d) : eventDefinitions.create(d)),
    onSuccess: onSaved,
  });

  const toggleChannel = (id: string) =>
    setD({ ...d, channels: d.channels.includes(id) ? d.channels.filter((x) => x !== id) : [...d.channels, id] });

  return (
    <Modal title={d.id ? "Edit event" : "New event"} onClose={onClose} wide>
      <form
        className="space-y-3"
        onSubmit={(e) => {
          e.preventDefault();
          if (d.name.trim()) save.mutate();
        }}
      >
        <div className="grid grid-cols-2 gap-3">
          <Field label="Name">
            <input className={inputClass} value={d.name} onChange={(e) => setD({ ...d, name: e.target.value })} />
          </Field>
          <Field label="Trigger type">
            <select className={selectClass} value={d.type} onChange={(e) => setD({ ...d, type: e.target.value as EventDefinition["type"] })}>
              <option value="filter">Filter (count matches)</option>
              <option value="aggregation">Aggregation (metric threshold)</option>
            </select>
          </Field>
        </div>
        <Field label="Query">
          <input className={`${inputClass} font-mono`} value={d.query} onChange={(e) => setD({ ...d, query: e.target.value })} />
        </Field>
        <div className="grid grid-cols-2 gap-3">
          <Field label="Window (seconds)">
            <input type="number" className={inputClass} value={d.window_seconds} onChange={(e) => setD({ ...d, window_seconds: Number(e.target.value) })} />
          </Field>
          <Field label="Threshold">
            <input type="number" className={inputClass} value={d.threshold} onChange={(e) => setD({ ...d, threshold: Number(e.target.value) })} />
          </Field>
        </div>
        {d.type === "aggregation" && (
          <div className="grid grid-cols-3 gap-3">
            <Field label="Group by">
              <input className={`${inputClass} font-mono`} value={d.group_by ?? ""} onChange={(e) => setD({ ...d, group_by: e.target.value })} />
            </Field>
            <Field label="Metric">
              <select className={selectClass} value={d.metric ?? "count"} onChange={(e) => setD({ ...d, metric: e.target.value })}>
                {["count", "sum", "avg", "min", "max", "p95"].map((m) => (
                  <option key={m}>{m}</option>
                ))}
              </select>
            </Field>
            <Field label="Metric field">
              <input className={`${inputClass} font-mono`} value={d.metric_field ?? ""} onChange={(e) => setD({ ...d, metric_field: e.target.value })} />
            </Field>
          </div>
        )}
        <div className="grid grid-cols-2 gap-3">
          <Field label="Grace period (seconds)">
            <input type="number" className={inputClass} value={d.grace_seconds ?? 0} onChange={(e) => setD({ ...d, grace_seconds: Number(e.target.value) })} />
          </Field>
          <Field label="Backlog samples">
            <input type="number" className={inputClass} value={d.backlog ?? 5} onChange={(e) => setD({ ...d, backlog: Number(e.target.value) })} />
          </Field>
        </div>
        <Field label="Message template (optional)">
          <textarea
            className="min-h-16 w-full rounded-md border border-input bg-background px-3 py-2 font-mono text-sm outline-none focus-visible:ring-2 focus-visible:ring-ring"
            placeholder="{{.Definition}} fired {{.Count}} times"
            value={d.message_template ?? ""}
            onChange={(e) => setD({ ...d, message_template: e.target.value })}
          />
        </Field>
        <div>
          <span className="text-xs font-medium text-muted-foreground">Notify channels</span>
          <div className="mt-1 flex flex-wrap gap-3">
            {chList.data?.map((c) => (
              <label key={c.id} className="flex items-center gap-2 text-sm">
                <input type="checkbox" checked={d.channels.includes(c.id!)} onChange={() => toggleChannel(c.id!)} />
                {c.name}
              </label>
            ))}
            {(!chList.data || chList.data.length === 0) && (
              <Link to="/notifications" className="text-xs text-primary hover:underline">
                No channels — add one
              </Link>
            )}
          </div>
        </div>
        <label className="flex items-center gap-2 text-sm">
          <input type="checkbox" checked={d.enabled} onChange={(e) => setD({ ...d, enabled: e.target.checked })} /> Enabled
        </label>

        {save.isError && <p className="text-xs text-destructive">Save failed — check the query syntax.</p>}
        <div className="flex justify-end gap-2">
          <Button type="button" variant="ghost" onClick={onClose}>
            Cancel
          </Button>
          <Button type="submit" disabled={!d.name.trim() || save.isPending}>
            Save
          </Button>
        </div>
      </form>
    </Modal>
  );
}
