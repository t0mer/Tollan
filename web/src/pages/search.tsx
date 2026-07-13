import { useState } from "react";
import { keepPreviousData, useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import { Bookmark, ChevronDown, ChevronRight, Clock, Search as SearchIcon, Trash2, X } from "lucide-react";
import { api, type LogMessage, type SearchParams } from "@/lib/api";
import { useSearchHistory } from "@/lib/history";
import { Button } from "@/components/ui/button";
import { Badge } from "@/components/ui/badge";
import { Histogram } from "@/components/histogram";
import { FieldFacets } from "@/components/field-facets";
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
  return new Date(iso).toISOString().replace("T", " ").replace(/\.\d+Z$/, "Z");
}

// addClause appends field:value to a query, quoting values that need it.
function addClause(query: string, field: string, value: string) {
  const needsQuote = /[\s:()"]/.test(value);
  const clause = `${field}:${needsQuote ? `"${value}"` : value}`;
  return query.trim() ? `${query.trim()} ${clause}` : clause;
}

export function SearchPage() {
  const qc = useQueryClient();
  const history = useSearchHistory();

  const [text, setText] = useState("");
  const [rangeIdx, setRangeIdx] = useState(2); // Last 24h
  const [params, setParams] = useState<SearchParams>({ from: "now-24h", limit: 100, order: "desc" });
  const [saveOpen, setSaveOpen] = useState(false);

  const base: SearchParams = { q: params.q, from: params.from, stream: params.stream };

  const results = useQuery({
    queryKey: ["search", params],
    queryFn: () => api.search(params),
    placeholderData: keepPreviousData,
  });
  const histogram = useQuery({
    queryKey: ["histogram", base],
    queryFn: () => api.histogram(base),
    placeholderData: keepPreviousData,
  });
  const facets = useQuery({
    queryKey: ["facets", base],
    queryFn: () => api.fields({ ...base, sample: 500, top: 8 }),
    placeholderData: keepPreviousData,
  });
  const saved = useQuery({ queryKey: ["saved"], queryFn: api.savedSearches });

  const del = useMutation({
    mutationFn: (id: string) => api.deleteSavedSearch(id),
    onSuccess: () => qc.invalidateQueries({ queryKey: ["saved"] }),
  });

  function run(q: string, from?: string) {
    setParams({ q: q.trim() || undefined, from, limit: 100, order: "desc" });
    if (q.trim()) history.push(q);
  }
  function submit() {
    run(text, RANGES[rangeIdx].from);
  }
  function applyClause(field: string, value: string) {
    const q = addClause(text, field, value);
    setText(q);
    run(q, RANGES[rangeIdx].from);
  }
  function applyQuery(q: string, range?: string) {
    setText(q);
    const idx = RANGES.findIndex((r) => r.from === range);
    if (idx >= 0) setRangeIdx(idx);
    run(q, range);
  }

  return (
    <div className="space-y-4">
      <div>
        <h1 className="font-display text-2xl font-semibold tracking-tight">Search</h1>
        <p className="mt-1 text-sm text-muted-foreground">
          Query language: <code className="font-mono">field:value</code>, ranges, wildcards,
          AND/OR/NOT, and free text.
        </p>
      </div>

      <form
        className="flex flex-wrap items-center gap-2"
        onSubmit={(e) => {
          e.preventDefault();
          submit();
        }}
      >
        <div className="relative min-w-56 flex-1">
          <SearchIcon className="pointer-events-none absolute left-3 top-1/2 size-4 -translate-y-1/2 text-muted-foreground" />
          <input
            value={text}
            onChange={(e) => setText(e.target.value)}
            placeholder='e.g.  level:error AND source:web01'
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
        <Button type="button" variant="outline" onClick={() => setSaveOpen(true)}>
          <Bookmark className="size-4" /> Save
        </Button>
      </form>

      <Histogram data={histogram.data} />

      <div className="grid grid-cols-1 gap-4 lg:grid-cols-[16rem_1fr]">
        <aside className="space-y-5 lg:max-h-[70vh] lg:overflow-y-auto">
          <Section title="Saved searches" icon={<Bookmark className="size-3.5" />}>
            {saved.data && saved.data.length > 0 ? (
              <ul className="space-y-0.5">
                {saved.data.map((s) => (
                  <li key={s.id} className="group flex items-center gap-1">
                    <button
                      onClick={() => applyQuery(s.query, s.time_range || undefined)}
                      className="flex-1 truncate rounded px-1 py-0.5 text-left text-xs hover:bg-muted"
                      title={s.query}
                    >
                      {s.name}
                    </button>
                    <button
                      onClick={() => del.mutate(s.id)}
                      className="rounded p-0.5 text-muted-foreground opacity-0 hover:text-destructive group-hover:opacity-100"
                      title="Delete"
                    >
                      <Trash2 className="size-3.5" />
                    </button>
                  </li>
                ))}
              </ul>
            ) : (
              <p className="px-1 text-xs text-muted-foreground">None yet.</p>
            )}
          </Section>

          <Section title="Fields" icon={<ChevronDown className="size-3.5" />}>
            <FieldFacets facets={facets.data} onSelect={applyClause} loading={facets.isLoading} />
          </Section>

          {history.items.length > 0 && (
            <Section title="History" icon={<Clock className="size-3.5" />}>
              <ul className="space-y-0.5">
                {history.items.map((h) => (
                  <li key={h}>
                    <button
                      onClick={() => applyQuery(h, RANGES[rangeIdx].from)}
                      className="w-full truncate rounded px-1 py-0.5 text-left font-mono text-xs hover:bg-muted"
                      title={h}
                    >
                      {h}
                    </button>
                  </li>
                ))}
              </ul>
            </Section>
          )}
        </aside>

        <div className="min-w-0 space-y-2">
          <div className="text-sm text-muted-foreground">
            {results.isError
              ? "Search failed — check your query syntax."
              : results.data
                ? `${results.data.count} of ${results.data.total} messages`
                : results.isLoading
                  ? "Searching…"
                  : ""}
          </div>
          <ResultsTable messages={results.data?.messages} />
        </div>
      </div>

      {saveOpen && (
        <SaveDialog
          query={text}
          timeRange={RANGES[rangeIdx].from ?? ""}
          onClose={() => setSaveOpen(false)}
          onSaved={() => {
            setSaveOpen(false);
            qc.invalidateQueries({ queryKey: ["saved"] });
          }}
        />
      )}
    </div>
  );
}

function Section({
  title,
  icon,
  children,
}: {
  title: string;
  icon: React.ReactNode;
  children: React.ReactNode;
}) {
  return (
    <div>
      <div className="mb-1 flex items-center gap-1.5 text-xs font-semibold uppercase tracking-wide text-muted-foreground">
        {icon}
        {title}
      </div>
      {children}
    </div>
  );
}

function ResultsTable({ messages }: { messages?: LogMessage[] }) {
  return (
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
        <tbody>{messages?.map((m) => <Row key={m.id} m={m} />)}</tbody>
      </table>
      {messages && messages.length === 0 && (
        <div className="p-8 text-center text-sm text-muted-foreground">
          No messages match this query.
        </div>
      )}
    </div>
  );
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

function SaveDialog({
  query,
  timeRange,
  onClose,
  onSaved,
}: {
  query: string;
  timeRange: string;
  onClose: () => void;
  onSaved: () => void;
}) {
  const [name, setName] = useState("");
  const create = useMutation({
    mutationFn: () => api.createSavedSearch({ name: name.trim(), query, time_range: timeRange }),
    onSuccess: onSaved,
  });
  return (
    <div className="fixed inset-0 z-50 grid place-items-center bg-black/50 p-4" onClick={onClose}>
      <div
        className="w-full max-w-sm rounded-lg border border-border bg-card p-5 shadow-lg"
        onClick={(e) => e.stopPropagation()}
      >
        <div className="mb-3 flex items-center justify-between">
          <h2 className="font-display font-semibold">Save search</h2>
          <button onClick={onClose} className="rounded p-1 text-muted-foreground hover:bg-muted">
            <X className="size-4" />
          </button>
        </div>
        <form
          onSubmit={(e) => {
            e.preventDefault();
            if (name.trim()) create.mutate();
          }}
          className="space-y-3"
        >
          <input
            autoFocus
            value={name}
            onChange={(e) => setName(e.target.value)}
            placeholder="Name"
            className="h-9 w-full rounded-md border border-input bg-background px-3 text-sm outline-none focus-visible:ring-2 focus-visible:ring-ring"
          />
          <div className="rounded-md bg-muted px-3 py-2 font-mono text-xs text-muted-foreground">
            {query || "(empty query)"}
          </div>
          {create.isError && (
            <p className="text-xs text-destructive">Could not save. Try a different name.</p>
          )}
          <div className="flex justify-end gap-2">
            <Button type="button" variant="ghost" onClick={onClose}>
              Cancel
            </Button>
            <Button type="submit" disabled={!name.trim() || create.isPending}>
              Save
            </Button>
          </div>
        </form>
      </div>
    </div>
  );
}
