import { useQuery } from "@tanstack/react-query";
import {
  Area,
  AreaChart,
  Bar,
  BarChart,
  CartesianGrid,
  Cell,
  Line,
  LineChart,
  Pie,
  PieChart,
  ResponsiveContainer,
  Tooltip,
  XAxis,
  YAxis,
} from "recharts";
import { Download } from "lucide-react";
import { api, exportUrl, type AggParams, type SearchParams, type Widget as WidgetSpec } from "@/lib/api";
import { Histogram } from "@/components/histogram";

// Categorical palette: jade primary plus a balanced, theme-neutral set.
const PALETTE = ["#17b8a6", "#6366f1", "#f59e0b", "#ef4444", "#8b5cf6", "#10b981", "#0ea5e9", "#ec4899"];

function baseParams(w: WidgetSpec, dashRange?: string): SearchParams {
  return { q: w.query || undefined, from: w.time_range || dashRange };
}

function aggParams(w: WidgetSpec, dashRange?: string): AggParams {
  return {
    ...baseParams(w, dashRange),
    group_by: w.group_by || undefined,
    metric: w.metric || "count",
    metric_field: w.metric_field || undefined,
    limit: 20,
  };
}

function fmtNum(n: number): string {
  if (Math.abs(n) >= 1000) return n.toLocaleString(undefined, { maximumFractionDigits: 1 });
  return String(Math.round(n * 100) / 100);
}

/** Widget renders one dashboard widget, fetching its own data on the dashboard
 * refresh interval. */
export function Widget({
  spec,
  dashRange,
  refreshMs,
}: {
  spec: WidgetSpec;
  dashRange?: string;
  refreshMs?: number;
}) {
  return (
    <div className="flex h-full flex-col rounded-lg border border-border bg-card">
      <div className="flex items-center justify-between border-b border-border px-3 py-2">
        <span className="truncate text-sm font-medium">{spec.title}</span>
        <div className="flex items-center gap-2">
          <a
            href={exportUrl(baseParams(spec, dashRange), "csv")}
            className="text-muted-foreground hover:text-foreground"
            title="Export underlying data (CSV)"
          >
            <Download className="size-3.5" />
          </a>
          <span className="font-mono text-[10px] uppercase text-muted-foreground">{spec.type}</span>
        </div>
      </div>
      <div className="min-h-0 flex-1 p-2">
        <WidgetBody spec={spec} dashRange={dashRange} refreshMs={refreshMs} />
      </div>
    </div>
  );
}

function WidgetBody({ spec, dashRange, refreshMs }: { spec: WidgetSpec; dashRange?: string; refreshMs?: number }) {
  switch (spec.type) {
    case "histogram":
      return <HistogramWidget spec={spec} dashRange={dashRange} refreshMs={refreshMs} />;
    case "table":
      return <TableWidget spec={spec} dashRange={dashRange} refreshMs={refreshMs} />;
    case "stat":
      return <StatWidget spec={spec} dashRange={dashRange} refreshMs={refreshMs} />;
    case "topn":
      return <TopNWidget spec={spec} dashRange={dashRange} refreshMs={refreshMs} />;
    case "map":
      return <MapWidget spec={spec} dashRange={dashRange} refreshMs={refreshMs} />;
    default:
      return <ChartWidget spec={spec} dashRange={dashRange} refreshMs={refreshMs} />;
  }
}

function useAgg(spec: WidgetSpec, dashRange?: string, refreshMs?: number) {
  const params = aggParams(spec, dashRange);
  return useQuery({
    queryKey: ["agg", params],
    queryFn: () => api.aggregate(params),
    refetchInterval: refreshMs,
  });
}

function HistogramWidget({ spec, dashRange, refreshMs }: WidgetProps) {
  const params = baseParams(spec, dashRange);
  const q = useQuery({
    queryKey: ["whistogram", params],
    queryFn: () => api.histogram(params),
    refetchInterval: refreshMs,
  });
  return <Histogram data={q.data} />;
}

function StatWidget({ spec, dashRange, refreshMs }: WidgetProps) {
  const q = useAgg({ ...spec, group_by: "" }, dashRange, refreshMs);
  const value = q.data?.rows[0]?.value ?? 0;
  return (
    <div className="flex h-full flex-col items-center justify-center">
      <span className="font-display text-4xl font-semibold tabular-nums">{fmtNum(value)}</span>
      <span className="mt-1 text-xs text-muted-foreground">
        {spec.metric || "count"}
        {spec.metric_field ? ` ${spec.metric_field}` : ""}
      </span>
    </div>
  );
}

function TopNWidget({ spec, dashRange, refreshMs }: WidgetProps) {
  const q = useAgg(spec, dashRange, refreshMs);
  const rows = q.data?.rows ?? [];
  const max = Math.max(1, ...rows.map((r) => r.value));
  return (
    <div className="h-full overflow-y-auto">
      <table className="w-full text-sm">
        <tbody>
          {rows.map((r) => (
            <tr key={r.key} className="border-b border-border last:border-0">
              <td className="relative py-1.5 pl-2 pr-3 font-mono text-xs">
                <div
                  className="absolute inset-y-1 left-0 -z-0 rounded-sm bg-primary/10"
                  style={{ width: `${(r.value / max) * 100}%` }}
                />
                <span className="relative">{r.key || "—"}</span>
              </td>
              <td className="py-1.5 pr-2 text-right font-mono text-xs tabular-nums">{fmtNum(r.value)}</td>
            </tr>
          ))}
        </tbody>
      </table>
      {rows.length === 0 && <Empty />}
    </div>
  );
}

function TableWidget({ spec, dashRange, refreshMs }: WidgetProps) {
  const params: SearchParams = { ...baseParams(spec, dashRange), limit: 50, order: "desc" };
  const q = useQuery({
    queryKey: ["wtable", params],
    queryFn: () => api.search(params),
    refetchInterval: refreshMs,
  });
  const msgs = q.data?.messages ?? [];
  return (
    <div className="h-full overflow-auto">
      <table className="w-full text-left text-xs">
        <tbody>
          {msgs.map((m) => (
            <tr key={m.id} className="border-b border-border last:border-0">
              <td className="whitespace-nowrap py-1 pr-2 font-mono text-muted-foreground">
                {new Date(m.timestamp).toISOString().slice(11, 19)}
              </td>
              <td className="py-1 pr-2 font-mono">{m.source}</td>
              <td className="py-1 font-mono">{m.message}</td>
            </tr>
          ))}
        </tbody>
      </table>
      {msgs.length === 0 && <Empty />}
    </div>
  );
}

function ChartWidget({ spec, dashRange, refreshMs }: WidgetProps) {
  const q = useAgg(spec, dashRange, refreshMs);
  const data = (q.data?.rows ?? []).map((r) => ({ name: r.key || "—", value: r.value }));
  if (data.length === 0) return <Empty />;

  const axis = { fontSize: 10, fill: "var(--muted-foreground)" };
  const tooltip = (
    <Tooltip
      contentStyle={{ background: "var(--card)", border: "1px solid var(--border)", borderRadius: 8, fontSize: 12 }}
      formatter={(v: number) => fmtNum(v)}
    />
  );

  return (
    <ResponsiveContainer width="100%" height="100%">
      {spec.type === "pie" ? (
        <PieChart>
          <Pie data={data} dataKey="value" nameKey="name" innerRadius="45%" outerRadius="80%" paddingAngle={2} isAnimationActive={false}>
            {data.map((_, i) => (
              <Cell key={i} fill={PALETTE[i % PALETTE.length]} />
            ))}
          </Pie>
          {tooltip}
        </PieChart>
      ) : spec.type === "line" ? (
        <LineChart data={data} margin={{ top: 8, right: 8, bottom: 0, left: -12 }}>
          <CartesianGrid strokeDasharray="3 3" stroke="var(--border)" vertical={false} />
          <XAxis dataKey="name" tick={axis} axisLine={false} tickLine={false} minTickGap={20} />
          <YAxis tick={axis} axisLine={false} tickLine={false} width={36} />
          {tooltip}
          <Line dataKey="value" stroke="var(--primary)" strokeWidth={2} dot={false} isAnimationActive={false} />
        </LineChart>
      ) : spec.type === "area" ? (
        <AreaChart data={data} margin={{ top: 8, right: 8, bottom: 0, left: -12 }}>
          <CartesianGrid strokeDasharray="3 3" stroke="var(--border)" vertical={false} />
          <XAxis dataKey="name" tick={axis} axisLine={false} tickLine={false} minTickGap={20} />
          <YAxis tick={axis} axisLine={false} tickLine={false} width={36} />
          {tooltip}
          <Area dataKey="value" stroke="var(--primary)" fill="var(--primary)" fillOpacity={0.15} strokeWidth={2} isAnimationActive={false} />
        </AreaChart>
      ) : (
        <BarChart data={data} margin={{ top: 8, right: 8, bottom: 0, left: -12 }}>
          <CartesianGrid strokeDasharray="3 3" stroke="var(--border)" vertical={false} />
          <XAxis dataKey="name" tick={axis} axisLine={false} tickLine={false} minTickGap={8} />
          <YAxis tick={axis} axisLine={false} tickLine={false} width={36} />
          {tooltip}
          <Bar dataKey="value" fill="var(--primary)" radius={[2, 2, 0, 0]} maxBarSize={48} isAnimationActive={false} />
        </BarChart>
      )}
    </ResponsiveContainer>
  );
}

function MapWidget({ spec, dashRange, refreshMs }: WidgetProps) {
  const q = useAgg(spec, dashRange, refreshMs);
  const points = (q.data?.rows ?? [])
    .map((r) => {
      const [lat, lon] = r.key.split(",").map(Number);
      return { lat, lon, value: r.value };
    })
    .filter((p) => Number.isFinite(p.lat) && Number.isFinite(p.lon));
  const max = Math.max(1, ...points.map((p) => p.value));

  // Equirectangular projection onto a 360x180 viewBox.
  return (
    <div className="grid h-full place-items-center">
      {points.length === 0 ? (
        <Empty label="No geo points (enrich with geoip())" />
      ) : (
        <svg viewBox="0 0 360 180" className="h-full w-full">
          <rect x="0" y="0" width="360" height="180" fill="var(--muted)" opacity="0.4" rx="4" />
          {[45, 90, 135].map((y) => (
            <line key={y} x1="0" y1={y} x2="360" y2={y} stroke="var(--border)" strokeWidth="0.5" />
          ))}
          {[90, 180, 270].map((x) => (
            <line key={x} x1={x} y1="0" x2={x} y2="180" stroke="var(--border)" strokeWidth="0.5" />
          ))}
          {points.map((p, i) => (
            <circle
              key={i}
              cx={(p.lon + 180) * (360 / 360)}
              cy={(90 - p.lat) * (180 / 180)}
              r={3 + (p.value / max) * 6}
              fill="var(--primary)"
              fillOpacity="0.6"
            />
          ))}
        </svg>
      )}
    </div>
  );
}

type WidgetProps = { spec: WidgetSpec; dashRange?: string; refreshMs?: number };

function Empty({ label }: { label?: string }) {
  return (
    <div className="grid h-full place-items-center text-xs text-muted-foreground">
      {label || "No data"}
    </div>
  );
}
