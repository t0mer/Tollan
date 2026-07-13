import { useState } from "react";
import { ChevronDown, ChevronRight } from "lucide-react";
import type { FieldFacet } from "@/lib/api";
import { cn } from "@/lib/utils";

// FieldFacets is the guided-search sidebar: fields present in the current result
// sample with their top values and counts. Clicking a value adds field:value to
// the query.
export function FieldFacets({
  facets,
  onSelect,
  loading,
}: {
  facets?: FieldFacet[];
  onSelect: (field: string, value: string) => void;
  loading?: boolean;
}) {
  if (loading) {
    return <p className="px-1 text-xs text-muted-foreground">Loading fields…</p>;
  }
  if (!facets || facets.length === 0) {
    return <p className="px-1 text-xs text-muted-foreground">No fields in results.</p>;
  }
  return (
    <div className="space-y-1">
      {facets.map((f) => (
        <FacetGroup key={f.field} facet={f} onSelect={onSelect} />
      ))}
    </div>
  );
}

function FacetGroup({
  facet,
  onSelect,
}: {
  facet: FieldFacet;
  onSelect: (field: string, value: string) => void;
}) {
  const [open, setOpen] = useState(true);
  return (
    <div>
      <button
        onClick={() => setOpen((o) => !o)}
        className="flex w-full items-center gap-1 rounded px-1 py-1 text-left text-xs font-medium text-foreground hover:bg-muted"
      >
        {open ? <ChevronDown className="size-3" /> : <ChevronRight className="size-3" />}
        <span className="font-mono">{facet.field}</span>
      </button>
      {open && (
        <ul className="ml-4 space-y-0.5 border-l border-border pl-2">
          {facet.values.map((v) => (
            <li key={v.value}>
              <button
                onClick={() => onSelect(facet.field, v.value)}
                className="group flex w-full items-center justify-between gap-2 rounded px-1 py-0.5 text-left hover:bg-muted"
                title={`Add ${facet.field}:${v.value}`}
              >
                <span className={cn("truncate font-mono text-xs")}>{v.value}</span>
                <span className="shrink-0 font-mono text-[10px] text-muted-foreground">
                  {v.count}
                </span>
              </button>
            </li>
          ))}
        </ul>
      )}
    </div>
  );
}
