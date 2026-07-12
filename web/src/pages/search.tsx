import { useState } from "react";
import { keepPreviousData, useQuery } from "@tanstack/react-query";
import { ChevronDown, ChevronRight, Search as SearchIcon } from "lucide-react";
import { api, type LogMessage, type SearchParams } from "@/lib/api";
import { Button } from "@/components/ui/button";
import { Badge } from "@/components/ui/badge";
import { cn } from "@/lib/utils";

const RANGES: { label: string; from?: string }[] = [
  { label: "Last 15m", from: "now-15m" },
  { label: "Last 1h", from: "now-1h" },
  { label: "Last 24h", from: "now-24h" },
  { label: "Last 7d", from: "now-168h" },
  { label: "All time", from: undefined },
];

function levelVariant(level: unknown): "destructive" | "warning" | "muted" | "default" {
  const l = String(level ?? "").toLowerCase();
  if (["emergency", "alert", "critical", "error"].includes(l)) return "destructive";
  if (["warning", "notice"].includes(l)) return "warning";
  if (l === "") return "muted";
  return "default";
}

function fmtTime(iso: string) {
  const d = new Date(iso);
  return d.toISOString().replace("T", " ").replace(/\.\d+Z$/, "Z");
}

function Row({ m }: { m: LogMessage }) {
  const [open, setOpen] = useState(false);
  const level = m.fields?.level;
  const entries = Object.entries(m.fields ?? {});
  return (
    <>
      <tr
        className="cursor-pointer border-b border-border align-top hover:bg-muted/50"
        onClick={() => setOpen((o) => !o)}
      >
        <td className="w-6 py-2 pl-3 pr-1 text-muted-foreground">
          {open ? <ChevronDown className="size-4" /> : <ChevronRight className="size-4" />}
        </td>
        <td className="whitespace-nowrap py-2 pr-4 font-mono text-xs text-muted-foreground">
          {fmtTime(m.timestamp)}
        </td>
        <td className="py-2 pr-4">
          {level ? (
            <Badge variant={levelVariant(level)}>{String(level)}</Badge>
          ) : (
            <span className="text-muted-foreground">—</span>
          )}
        </td>
        <td className="whitespace-nowrap py-2 pr-4 font-mono text-xs">{m.source || "—"}</td>
        <td className="py-2 pr-3 font-mono text-xs">
          <span className={cn(!open && "line-clamp-1")}>{m.message}</span>
        </td>
      </tr>
      {open && (
        <tr className="border-b border-border bg-muted/30">
          <td />
          <td colSpan={4} className="py-3 pr-3">
            <dl className="grid grid-cols-[max-content_1fr] gap-x-4 gap-y-1 font-mono text-xs">
              <dt className="text-muted-foreground">stream</dt>
              <dd>{m.stream}</dd>
              <dt className="text-muted-foreground">input</dt>
              <dd>{m.input_id}</dd>
              <dt className="text-muted-foreground">received</dt>
              <dd>{fmtTime(m.received_at)}</dd>
              {entries.map(([k, v]) => (
                <FieldPair key={k} k={k} v={v} />
              ))}
            </dl>
          </td>
        </tr>
      )}
    </>
  );
}

function FieldPair({ k, v }: { k: string; v: unknown }) {
  return (
    <>
      <dt className="text-muted-foreground">{k}</dt>
      <dd className="break-all">{typeof v === "object" ? JSON.stringify(v) : String(v)}</dd>
    </>
  );
}

export function SearchPage() {
  const [text, setText] = useState("");
  const [rangeIdx, setRangeIdx] = useState(4); // All time
  const [params, setParams] = useState<SearchParams>({ limit: 100, order: "desc" });

  const query = useQuery({
    queryKey: ["search", params],
    queryFn: () => api.search(params),
    placeholderData: keepPreviousData,
  });

  const run = () =>
    setParams({ q: text.trim() || undefined, from: RANGES[rangeIdx].from, limit: 100, order: "desc" });

  return (
    <div className="mx-auto max-w-6xl space-y-5">
      <div>
        <h1 className="font-display text-2xl font-semibold tracking-tight">Search</h1>
        <p className="mt-1 text-sm text-muted-foreground">
          Query stored logs. Full-text matches the message body.
        </p>
      </div>

      <form
        className="flex flex-wrap items-center gap-2"
        onSubmit={(e) => {
          e.preventDefault();
          run();
        }}
      >
        <div className="relative min-w-56 flex-1">
          <SearchIcon className="pointer-events-none absolute left-3 top-1/2 size-4 -translate-y-1/2 text-muted-foreground" />
          <input
            value={text}
            onChange={(e) => setText(e.target.value)}
            placeholder="search message text…"
            className="h-9 w-full rounded-md border border-input bg-card pl-9 pr-3 font-mono text-sm outline-none focus-visible:ring-2 focus-visible:ring-ring"
          />
        </div>
        <select
          value={rangeIdx}
          onChange={(e) => setRangeIdx(Number(e.target.value))}
          className="h-9 rounded-md border border-input bg-card px-3 text-sm outline-none focus-visible:ring-2 focus-visible:ring-ring"
        >
          {RANGES.map((r, i) => (
            <option key={r.label} value={i}>
              {r.label}
            </option>
          ))}
        </select>
        <Button type="submit">Search</Button>
      </form>

      <div className="flex items-center justify-between text-sm text-muted-foreground">
        <span>
          {query.isError
            ? "Search failed"
            : query.data
              ? `${query.data.count} of ${query.data.total} messages`
              : query.isLoading
                ? "Searching…"
                : ""}
        </span>
      </div>

      <div className="overflow-x-auto rounded-lg border border-border bg-card">
        <table className="w-full border-collapse text-left text-sm">
          <thead>
            <tr className="border-b border-border text-xs uppercase tracking-wide text-muted-foreground">
              <th className="w-6" />
              <th className="py-2 pl-3 pr-4 font-medium">Time</th>
              <th className="py-2 pr-4 font-medium">Level</th>
              <th className="py-2 pr-4 font-medium">Source</th>
              <th className="py-2 pr-3 font-medium">Message</th>
            </tr>
          </thead>
          <tbody>
            {query.data?.messages.map((m) => <Row key={m.id} m={m} />)}
          </tbody>
        </table>
        {query.data && query.data.messages.length === 0 && (
          <div className="p-8 text-center text-sm text-muted-foreground">
            No messages match this query.
          </div>
        )}
      </div>
    </div>
  );
}
