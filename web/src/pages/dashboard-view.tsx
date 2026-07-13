import { useEffect, useMemo, useState } from "react";
import { Link, useParams } from "react-router-dom";
import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import { ArrowLeft, Maximize2, Pencil, Plus, Save, Trash2, X } from "lucide-react";
import { dashboards, type Dashboard, type Widget as WidgetSpec, type WidgetType } from "@/lib/api";
import { Button } from "@/components/ui/button";
import { Widget } from "@/components/widget";
import { Modal, Field, inputClass, selectClass } from "@/components/ui/modal";

const RANGES = [
  { label: "Last 15m", v: "now-15m" },
  { label: "Last 1h", v: "now-1h" },
  { label: "Last 24h", v: "now-24h" },
  { label: "Last 7d", v: "now-168h" },
  { label: "All time", v: "" },
];
const REFRESH = [
  { label: "Off", v: 0 },
  { label: "10s", v: 10 },
  { label: "30s", v: 30 },
  { label: "1m", v: 60 },
  { label: "5m", v: 300 },
];
const TYPES: WidgetType[] = ["table", "histogram", "bar", "line", "area", "pie", "stat", "topn", "map"];

function newWidget(): WidgetSpec {
  return { id: crypto.randomUUID(), type: "bar", title: "New widget", query: "", group_by: "source", metric: "count", w: 6, h: 3 };
}

export function DashboardViewPage() {
  const { id } = useParams();
  const qc = useQueryClient();
  const list = useQuery({ queryKey: ["dashboards"], queryFn: dashboards.list });
  const loaded = useMemo(() => list.data?.find((d) => d.id === id), [list.data, id]);

  const [dash, setDash] = useState<Dashboard | null>(null);
  const [editing, setEditing] = useState<WidgetSpec | null>(null);
  const [editMode, setEditMode] = useState(false);
  const [tv, setTv] = useState(false);

  useEffect(() => {
    if (loaded && !dash) setDash(loaded);
  }, [loaded, dash]);

  const save = useMutation({
    mutationFn: () => dashboards.update(dash!.id!, dash!),
    onSuccess: () => qc.invalidateQueries({ queryKey: ["dashboards"] }),
  });

  if (!dash) return <div className="p-8 text-sm text-muted-foreground">Loading…</div>;

  const refreshMs = dash.refresh_seconds ? dash.refresh_seconds * 1000 : undefined;

  const upsertWidget = (w: WidgetSpec) => {
    const exists = dash.widgets.some((x) => x.id === w.id);
    setDash({ ...dash, widgets: exists ? dash.widgets.map((x) => (x.id === w.id ? w : x)) : [...dash.widgets, w] });
  };
  const removeWidget = (wid: string) => setDash({ ...dash, widgets: dash.widgets.filter((x) => x.id !== wid) });

  const grid = (
    <div className="grid grid-cols-1 gap-3 md:grid-cols-12">
      {dash.widgets.map((w) => (
        <div
          key={w.id}
          className="relative md:[grid-column:span_var(--w)]"
          style={{ ["--w" as string]: String(w.w), height: `${w.h * 88}px` }}
        >
          <Widget spec={w} dashRange={dash.time_range} refreshMs={refreshMs} />
          {editMode && (
            <div className="absolute right-2 top-2 flex gap-1">
              <button
                onClick={() => setEditing(w)}
                className="rounded bg-card/90 p-1 text-muted-foreground shadow hover:text-foreground"
              >
                <Pencil className="size-3.5" />
              </button>
              <button
                onClick={() => removeWidget(w.id)}
                className="rounded bg-card/90 p-1 text-muted-foreground shadow hover:text-destructive"
              >
                <Trash2 className="size-3.5" />
              </button>
            </div>
          )}
        </div>
      ))}
      {dash.widgets.length === 0 && (
        <div className="col-span-full grid place-items-center rounded-lg border border-dashed border-border p-12 text-sm text-muted-foreground">
          No widgets. Click “Add widget”.
        </div>
      )}
    </div>
  );

  if (tv) {
    return (
      <div className="fixed inset-0 z-50 overflow-auto bg-background p-4">
        <div className="mb-3 flex items-center justify-between">
          <h2 className="font-display text-lg font-semibold">{dash.name}</h2>
          <button onClick={() => setTv(false)} className="rounded p-1 text-muted-foreground hover:bg-muted">
            <X className="size-5" />
          </button>
        </div>
        {grid}
      </div>
    );
  }

  return (
    <div className="space-y-4">
      <div className="flex flex-wrap items-center gap-2">
        <Link to="/dashboards" className="text-muted-foreground hover:text-foreground">
          <ArrowLeft className="size-5" />
        </Link>
        <h1 className="font-display text-xl font-semibold tracking-tight">{dash.name}</h1>
        <div className="ml-auto flex flex-wrap items-center gap-2">
          <select
            className={`${selectClass} w-auto`}
            value={dash.time_range ?? ""}
            onChange={(e) => setDash({ ...dash, time_range: e.target.value })}
          >
            {RANGES.map((r) => (
              <option key={r.label} value={r.v}>
                {r.label}
              </option>
            ))}
          </select>
          <select
            className={`${selectClass} w-auto`}
            value={dash.refresh_seconds ?? 0}
            onChange={(e) => setDash({ ...dash, refresh_seconds: Number(e.target.value) })}
          >
            {REFRESH.map((r) => (
              <option key={r.label} value={r.v}>
                ⟳ {r.label}
              </option>
            ))}
          </select>
          <Button variant="outline" onClick={() => setTv(true)}>
            <Maximize2 className="size-4" /> TV
          </Button>
          <Button variant={editMode ? "default" : "outline"} onClick={() => setEditMode((e) => !e)}>
            <Pencil className="size-4" /> {editMode ? "Done" : "Edit"}
          </Button>
          {editMode && (
            <Button variant="outline" onClick={() => setEditing(newWidget())}>
              <Plus className="size-4" /> Add widget
            </Button>
          )}
          <Button onClick={() => save.mutate()} disabled={save.isPending}>
            <Save className="size-4" /> Save
          </Button>
        </div>
      </div>

      {grid}

      {editing && (
        <WidgetEditor
          initial={editing}
          onClose={() => setEditing(null)}
          onSave={(w) => {
            upsertWidget(w);
            setEditing(null);
          }}
        />
      )}
    </div>
  );
}

function WidgetEditor({
  initial,
  onClose,
  onSave,
}: {
  initial: WidgetSpec;
  onClose: () => void;
  onSave: (w: WidgetSpec) => void;
}) {
  const [w, setW] = useState<WidgetSpec>(initial);
  const needsAgg = ["bar", "line", "area", "pie", "stat", "topn", "map"].includes(w.type);
  const needsGroup = ["bar", "line", "area", "pie", "topn", "map"].includes(w.type);
  const needsMetricField = needsAgg && w.metric && w.metric !== "count";

  return (
    <Modal title="Widget" onClose={onClose} wide>
      <form
        className="space-y-3"
        onSubmit={(e) => {
          e.preventDefault();
          onSave(w);
        }}
      >
        <div className="grid grid-cols-2 gap-3">
          <Field label="Title">
            <input className={inputClass} value={w.title} onChange={(e) => setW({ ...w, title: e.target.value })} />
          </Field>
          <Field label="Type">
            <select className={selectClass} value={w.type} onChange={(e) => setW({ ...w, type: e.target.value as WidgetType })}>
              {TYPES.map((t) => (
                <option key={t} value={t}>
                  {t}
                </option>
              ))}
            </select>
          </Field>
        </div>
        <Field label="Query (optional; blank = all)">
          <input
            className={`${inputClass} font-mono`}
            placeholder="level:error"
            value={w.query ?? ""}
            onChange={(e) => setW({ ...w, query: e.target.value })}
          />
        </Field>
        {needsAgg && (
          <div className="grid grid-cols-3 gap-3">
            {needsGroup && (
              <Field label="Group by field">
                <input className={`${inputClass} font-mono`} value={w.group_by ?? ""} onChange={(e) => setW({ ...w, group_by: e.target.value })} />
              </Field>
            )}
            <Field label="Metric">
              <select className={selectClass} value={w.metric ?? "count"} onChange={(e) => setW({ ...w, metric: e.target.value })}>
                {["count", "sum", "avg", "min", "max", "p50", "p90", "p95", "p99"].map((m) => (
                  <option key={m} value={m}>
                    {m}
                  </option>
                ))}
              </select>
            </Field>
            {needsMetricField && (
              <Field label="Metric field">
                <input className={`${inputClass} font-mono`} value={w.metric_field ?? ""} onChange={(e) => setW({ ...w, metric_field: e.target.value })} />
              </Field>
            )}
          </div>
        )}
        <div className="grid grid-cols-2 gap-3">
          <Field label={`Width (cols): ${w.w}`}>
            <input type="range" min={2} max={12} value={w.w} onChange={(e) => setW({ ...w, w: Number(e.target.value) })} />
          </Field>
          <Field label={`Height: ${w.h}`}>
            <input type="range" min={1} max={6} value={w.h} onChange={(e) => setW({ ...w, h: Number(e.target.value) })} />
          </Field>
        </div>
        <div className="flex justify-end gap-2">
          <Button type="button" variant="ghost" onClick={onClose}>
            Cancel
          </Button>
          <Button type="submit">Apply</Button>
        </div>
      </form>
    </Modal>
  );
}
