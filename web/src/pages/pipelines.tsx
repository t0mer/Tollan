import { useState } from "react";
import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import { Pencil, Plus, Trash2 } from "lucide-react";
import { pipelines, type Pipeline, type PipelineRule } from "@/lib/api";
import { Button } from "@/components/ui/button";
import { Badge } from "@/components/ui/badge";
import { Card, CardContent } from "@/components/ui/card";
import { Modal, Field, inputClass } from "@/components/ui/modal";

function emptyPipeline(): Pipeline {
  return { name: "", enabled: true, stages: ["_all"], rules: [{ name: "rule1", when: "true", then: [""] }] };
}

export function PipelinesPage() {
  const qc = useQueryClient();
  const list = useQuery({ queryKey: ["pipelines"], queryFn: pipelines.list });
  const [editing, setEditing] = useState<Pipeline | null>(null);
  const del = useMutation({
    mutationFn: (id: string) => pipelines.remove(id),
    onSuccess: () => qc.invalidateQueries({ queryKey: ["pipelines"] }),
  });

  return (
    <div className="mx-auto max-w-5xl space-y-6">
      <div className="flex items-center justify-between">
        <div>
          <h1 className="font-display text-2xl font-semibold tracking-tight">Pipelines</h1>
          <p className="mt-1 text-sm text-muted-foreground">
            Normalize and enrich messages with <code className="font-mono">when … then …</code> rules.
          </p>
        </div>
        <Button onClick={() => setEditing(emptyPipeline())}>
          <Plus className="size-4" /> New pipeline
        </Button>
      </div>

      <Card>
        <CardContent className="p-0">
          <table className="w-full text-left text-sm">
            <thead>
              <tr className="border-b border-border text-xs uppercase tracking-wide text-muted-foreground">
                <th className="p-3 font-medium">Name</th>
                <th className="p-3 font-medium">Stages</th>
                <th className="p-3 font-medium">Rules</th>
                <th className="p-3 font-medium">Status</th>
                <th className="p-3" />
              </tr>
            </thead>
            <tbody>
              {list.data?.map((p) => (
                <tr key={p.id} className="border-b border-border last:border-0">
                  <td className="p-3 font-medium">{p.name}</td>
                  <td className="p-3 font-mono text-xs">{p.stages.join(", ")}</td>
                  <td className="p-3">{p.rules.length}</td>
                  <td className="p-3">
                    <Badge variant={p.enabled ? "success" : "muted"}>{p.enabled ? "enabled" : "disabled"}</Badge>
                  </td>
                  <td className="p-3 text-right">
                    <button onClick={() => setEditing(p)} className="rounded p-1 text-muted-foreground hover:bg-muted">
                      <Pencil className="size-4" />
                    </button>
                    <button
                      onClick={() => p.id && del.mutate(p.id)}
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
            <div className="p-6 text-center text-sm text-muted-foreground">No pipelines yet.</div>
          )}
        </CardContent>
      </Card>

      {editing && (
        <PipelineEditor
          initial={editing}
          onClose={() => setEditing(null)}
          onSaved={() => {
            setEditing(null);
            qc.invalidateQueries({ queryKey: ["pipelines"] });
          }}
        />
      )}
    </div>
  );
}

function PipelineEditor({
  initial,
  onClose,
  onSaved,
}: {
  initial: Pipeline;
  onClose: () => void;
  onSaved: () => void;
}) {
  const [p, setP] = useState<Pipeline>(initial);
  const save = useMutation({
    mutationFn: () => (p.id ? pipelines.update(p.id, p) : pipelines.create(p)),
    onSuccess: onSaved,
  });

  const setRule = (i: number, patch: Partial<PipelineRule>) =>
    setP({ ...p, rules: p.rules.map((r, j) => (j === i ? { ...r, ...patch } : r)) });

  return (
    <Modal title={p.id ? "Edit pipeline" : "New pipeline"} onClose={onClose} wide>
      <form
        className="space-y-4"
        onSubmit={(e) => {
          e.preventDefault();
          if (p.name.trim()) save.mutate();
        }}
      >
        <div className="grid grid-cols-2 gap-3">
          <Field label="Name">
            <input className={inputClass} value={p.name} onChange={(e) => setP({ ...p, name: e.target.value })} />
          </Field>
          <Field label="Stages (comma-separated; _all runs pre-routing)">
            <input
              className={`${inputClass} font-mono`}
              value={p.stages.join(",")}
              onChange={(e) => setP({ ...p, stages: e.target.value.split(",").map((x) => x.trim()).filter(Boolean) })}
            />
          </Field>
        </div>
        <label className="flex items-center gap-2 text-sm">
          <input type="checkbox" checked={p.enabled} onChange={(e) => setP({ ...p, enabled: e.target.checked })} />
          Enabled
        </label>

        <div className="space-y-3">
          <div className="flex items-center justify-between">
            <span className="text-xs font-medium text-muted-foreground">Rules</span>
            <button
              type="button"
              className="text-xs text-primary hover:underline"
              onClick={() => setP({ ...p, rules: [...p.rules, { name: `rule${p.rules.length + 1}`, when: "true", then: [""] }] })}
            >
              + add rule
            </button>
          </div>
          {p.rules.map((r, i) => (
            <div key={i} className="space-y-2 rounded-md border border-border p-3">
              <div className="flex items-center gap-2">
                <input
                  className={inputClass}
                  placeholder="rule name"
                  value={r.name}
                  onChange={(e) => setRule(i, { name: e.target.value })}
                />
                <button
                  type="button"
                  className="rounded p-1 text-muted-foreground hover:text-destructive"
                  onClick={() => setP({ ...p, rules: p.rules.filter((_, j) => j !== i) })}
                >
                  <Trash2 className="size-4" />
                </button>
              </div>
              <Field label="when">
                <input
                  className={`${inputClass} font-mono`}
                  placeholder={`eq(level, "error") && has(src_ip)`}
                  value={r.when}
                  onChange={(e) => setRule(i, { when: e.target.value })}
                />
              </Field>
              <Field label="then (one action per line)">
                <textarea
                  className="min-h-16 w-full rounded-md border border-input bg-background px-3 py-2 font-mono text-sm outline-none focus-visible:ring-2 focus-visible:ring-ring"
                  placeholder={`set("env", "prod")\ngrok(message, "%{WORD:verb} %{URIPATH:path}")`}
                  value={r.then.join("\n")}
                  onChange={(e) => setRule(i, { then: e.target.value.split("\n") })}
                />
              </Field>
            </div>
          ))}
        </div>

        {save.isError && <p className="text-xs text-destructive">Save failed — check rule syntax.</p>}
        <div className="flex justify-end gap-2">
          <Button type="button" variant="ghost" onClick={onClose}>
            Cancel
          </Button>
          <Button type="submit" disabled={!p.name.trim() || save.isPending}>
            Save
          </Button>
        </div>
      </form>
    </Modal>
  );
}
