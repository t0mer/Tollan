import { useState } from "react";
import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import { Pencil, Plus, Trash2 } from "lucide-react";
import { streams, type MatchRule, type Stream } from "@/lib/api";
import { Button } from "@/components/ui/button";
import { Badge } from "@/components/ui/badge";
import { Card, CardContent } from "@/components/ui/card";
import { Modal, Field, inputClass, selectClass } from "@/components/ui/modal";

const RULE_TYPES: MatchRule["type"][] = ["exact", "regex", "presence", "contains", "gt", "lt"];

function emptyStream(): Stream {
  return { name: "", combinator: "and", rules: [{ field: "", type: "exact", value: "" }], retention_days: 0 };
}

export function StreamsPage() {
  const qc = useQueryClient();
  const list = useQuery({ queryKey: ["streams"], queryFn: streams.list });
  const [editing, setEditing] = useState<Stream | null>(null);

  const del = useMutation({
    mutationFn: (id: string) => streams.remove(id),
    onSuccess: () => qc.invalidateQueries({ queryKey: ["streams"] }),
  });

  return (
    <div className="mx-auto max-w-5xl space-y-6">
      <div className="flex items-center justify-between">
        <div>
          <h1 className="font-display text-2xl font-semibold tracking-tight">Streams</h1>
          <p className="mt-1 text-sm text-muted-foreground">
            Route messages into categories with match rules and retention.
          </p>
        </div>
        <Button onClick={() => setEditing(emptyStream())}>
          <Plus className="size-4" /> New stream
        </Button>
      </div>

      <Card>
        <CardContent className="p-0">
          <table className="w-full text-left text-sm">
            <thead>
              <tr className="border-b border-border text-xs uppercase tracking-wide text-muted-foreground">
                <th className="p-3 font-medium">Name</th>
                <th className="p-3 font-medium">Rules</th>
                <th className="p-3 font-medium">Retention</th>
                <th className="p-3" />
              </tr>
            </thead>
            <tbody>
              <tr className="border-b border-border">
                <td className="p-3 font-medium">All messages</td>
                <td className="p-3 text-muted-foreground">default (catch-all)</td>
                <td className="p-3 text-muted-foreground">global</td>
                <td />
              </tr>
              {list.data?.map((s) => (
                <tr key={s.id} className="border-b border-border last:border-0">
                  <td className="p-3 font-medium">{s.name}</td>
                  <td className="p-3">
                    <Badge variant="muted">
                      {s.rules.length} rule{s.rules.length !== 1 ? "s" : ""} ({s.combinator})
                    </Badge>
                  </td>
                  <td className="p-3 font-mono text-xs">
                    {s.retention_days ? `${s.retention_days}d` : "global"}
                  </td>
                  <td className="p-3 text-right">
                    <button
                      onClick={() => setEditing(s)}
                      className="rounded p-1 text-muted-foreground hover:bg-muted"
                    >
                      <Pencil className="size-4" />
                    </button>
                    <button
                      onClick={() => s.id && del.mutate(s.id)}
                      className="rounded p-1 text-muted-foreground hover:text-destructive"
                    >
                      <Trash2 className="size-4" />
                    </button>
                  </td>
                </tr>
              ))}
            </tbody>
          </table>
          {list.data && list.data.length === 0 && (
            <div className="p-6 text-center text-sm text-muted-foreground">
              No user streams yet. Everything flows to the default stream.
            </div>
          )}
        </CardContent>
      </Card>

      {editing && (
        <StreamEditor
          initial={editing}
          onClose={() => setEditing(null)}
          onSaved={() => {
            setEditing(null);
            qc.invalidateQueries({ queryKey: ["streams"] });
          }}
        />
      )}
    </div>
  );
}

function StreamEditor({
  initial,
  onClose,
  onSaved,
}: {
  initial: Stream;
  onClose: () => void;
  onSaved: () => void;
}) {
  const [s, setS] = useState<Stream>(initial);
  const save = useMutation({
    mutationFn: () => (s.id ? streams.update(s.id, s) : streams.create(s)),
    onSuccess: onSaved,
  });

  const setRule = (i: number, patch: Partial<MatchRule>) =>
    setS({ ...s, rules: s.rules.map((r, j) => (j === i ? { ...r, ...patch } : r)) });

  return (
    <Modal title={s.id ? "Edit stream" : "New stream"} onClose={onClose} wide>
      <form
        className="space-y-4"
        onSubmit={(e) => {
          e.preventDefault();
          if (s.name.trim()) save.mutate();
        }}
      >
        <div className="grid grid-cols-2 gap-3">
          <Field label="Name">
            <input className={inputClass} value={s.name} onChange={(e) => setS({ ...s, name: e.target.value })} />
          </Field>
          <Field label="Retention (days, 0 = global)">
            <input
              type="number"
              className={inputClass}
              value={s.retention_days ?? 0}
              onChange={(e) => setS({ ...s, retention_days: Number(e.target.value) })}
            />
          </Field>
        </div>

        <Field label="Match">
          <select
            className={selectClass}
            value={s.combinator}
            onChange={(e) => setS({ ...s, combinator: e.target.value as "and" | "or" })}
          >
            <option value="and">All rules (AND)</option>
            <option value="or">Any rule (OR)</option>
          </select>
        </Field>

        <div className="space-y-2">
          <div className="flex items-center justify-between">
            <span className="text-xs font-medium text-muted-foreground">Rules</span>
            <button
              type="button"
              className="text-xs text-primary hover:underline"
              onClick={() => setS({ ...s, rules: [...s.rules, { field: "", type: "exact", value: "" }] })}
            >
              + add rule
            </button>
          </div>
          {s.rules.map((r, i) => (
            <div key={i} className="flex items-center gap-2">
              <input
                className={`${inputClass} font-mono`}
                placeholder="field"
                value={r.field}
                onChange={(e) => setRule(i, { field: e.target.value })}
              />
              <select className={selectClass} value={r.type} onChange={(e) => setRule(i, { type: e.target.value as MatchRule["type"] })}>
                {RULE_TYPES.map((t) => (
                  <option key={t} value={t}>
                    {t}
                  </option>
                ))}
              </select>
              <input
                className={`${inputClass} font-mono`}
                placeholder="value"
                disabled={r.type === "presence"}
                value={r.value}
                onChange={(e) => setRule(i, { value: e.target.value })}
              />
              <label className="flex items-center gap-1 text-xs text-muted-foreground">
                <input type="checkbox" checked={!!r.negate} onChange={(e) => setRule(i, { negate: e.target.checked })} />
                not
              </label>
              <button
                type="button"
                className="rounded p-1 text-muted-foreground hover:text-destructive"
                onClick={() => setS({ ...s, rules: s.rules.filter((_, j) => j !== i) })}
              >
                <Trash2 className="size-4" />
              </button>
            </div>
          ))}
        </div>

        {save.isError && <p className="text-xs text-destructive">Save failed — check rule syntax.</p>}
        <div className="flex justify-end gap-2">
          <Button type="button" variant="ghost" onClick={onClose}>
            Cancel
          </Button>
          <Button type="submit" disabled={!s.name.trim() || save.isPending}>
            Save
          </Button>
        </div>
      </form>
    </Modal>
  );
}
