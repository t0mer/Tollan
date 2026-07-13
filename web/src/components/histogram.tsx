import { useMemo } from "react";
import { Bar, BarChart, ResponsiveContainer, Tooltip, XAxis, YAxis } from "recharts";
import type { Histogram as HistogramData } from "@/lib/api";

// densify fills zero-count gaps between the first and last returned bucket so the
// bars read as a continuous timeline rather than collapsing sparse data.
function densify(h: HistogramData) {
  if (h.buckets.length === 0) return [];
  const step = h.interval_ms || 60_000;
  const first = h.buckets[0].start_ms;
  const last = h.buckets[h.buckets.length - 1].start_ms;
  const counts = new Map(h.buckets.map((b) => [b.start_ms, b.count]));
  const out: { t: number; count: number }[] = [];
  for (let t = first; t <= last; t += step) out.push({ t, count: counts.get(t) ?? 0 });
  return out;
}

function fmt(ms: number, step: number) {
  const d = new Date(ms);
  const p = (n: number) => String(n).padStart(2, "0");
  if (step < 86_400_000) return `${p(d.getHours())}:${p(d.getMinutes())}`;
  return `${p(d.getMonth() + 1)}-${p(d.getDate())}`;
}

export function Histogram({ data }: { data?: HistogramData }) {
  const bars = useMemo(() => (data ? densify(data) : []), [data]);
  const step = data?.interval_ms ?? 60_000;

  if (!data || bars.length === 0) {
    return (
      <div className="flex h-24 items-center justify-center rounded-lg border border-border bg-card text-xs text-muted-foreground">
        No data in range
      </div>
    );
  }

  return (
    <div className="h-24 rounded-lg border border-border bg-card p-2">
      <ResponsiveContainer width="100%" height="100%">
        <BarChart data={bars} margin={{ top: 4, right: 8, bottom: 0, left: 0 }}>
          <XAxis
            dataKey="t"
            tickFormatter={(t) => fmt(t, step)}
            tick={{ fontSize: 10, fill: "var(--muted-foreground)" }}
            axisLine={false}
            tickLine={false}
            minTickGap={40}
          />
          <YAxis
            width={28}
            tick={{ fontSize: 10, fill: "var(--muted-foreground)" }}
            axisLine={false}
            tickLine={false}
            allowDecimals={false}
          />
          <Tooltip
            cursor={{ fill: "var(--muted)" }}
            contentStyle={{
              background: "var(--card)",
              border: "1px solid var(--border)",
              borderRadius: 8,
              fontSize: 12,
            }}
            labelFormatter={(t) => new Date(Number(t)).toISOString().replace("T", " ").slice(0, 19)}
            formatter={(v: number) => [v, "messages"]}
          />
          <Bar dataKey="count" fill="var(--primary)" radius={[2, 2, 0, 0]} maxBarSize={28} isAnimationActive={false} />
        </BarChart>
      </ResponsiveContainer>
    </div>
  );
}
