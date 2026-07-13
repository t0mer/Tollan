import { useState } from "react";
import { Link } from "react-router-dom";
import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import { ArrowLeft, Pencil, Plus, Trash2 } from "lucide-react";
import { outputs, streams, type Output } from "@/lib/api";
import { Button } from "@/components/ui/button";
import { Badge } from "@/components/ui/badge";
import { Card, CardContent } from "@/components/ui/card";
import { Modal, Field, inputClass, selectClass } from "@/components/ui/modal";

function emptyOutput(): Output {
  return { name: "", type: "gelf", enabled: true, stream: "", address: "", protocol: "tcp" };
}

export function OutputsPage() {
  const qc = useQueryClient();
  const list = useQuery({ queryKey: ["outputs"], queryFn: outputs.list });
  const streamList = useQuery({ queryKey: ["streams"], queryFn: streams.list });
  const [editing, setEditing] = useState<Output | null>(null);
  const del = useMutation({
    mutationFn: (id: string) => outputs.remove(id),
    onSuccess: () => qc.invalidateQueries({ queryKey: ["outputs"] }),
  });

  return (
    <div className="mx-auto max-w-4xl space-y-6">
      <div className="flex items-center justify-between">
        <div className="flex items-center gap-2">
          <Link to="/system" className="text-muted-foreground hover:text-foreground">
            <ArrowLeft className="size-5" />
          </Link>
          <div>
            <h1 className="font-display text-2xl font-semibold tracking-tight">Outputs</h1>
            <p className="mt-1 text-sm text-muted-foreground">Forward messages to GELF, syslog, raw TCP or stdout.</p>
          </div>
        </div>
        <Button onClick={() => setEditing(emptyOutput())}>
          <Plus className="size-4" /> New output
        </Button>
      </div>

      <Card>
        <CardContent className="p-0">
          <table className="w-full text-left text-sm">
            <thead>
              <tr className="border-b border-border text-xs uppercase tracking-wide text-muted-foreground">
                <th className="p-3 font-medium">Name</th>
                <th className="p-3 font-medium">Type</th>
                <th className="p-3 font-medium">Target</th>
                <th className="p-3 font-medium">Status</th>
                <th className="p-3" />
              </tr>
            </thead>
            <tbody>
              {list.data?.map((o) => (
                <tr key={o.id} className="border-b border-border last:border-0">
                  <td className="p-3 font-medium">{o.name}</td>
                  <td className="p-3 font-mono text-xs">{o.type}</td>
                  <td className="p-3 font-mono text-xs text-muted-foreground">{o.address || "stdout"}</td>
                  <td className="p-3">
                    <Badge variant={o.enabled ? "success" : "muted"}>{o.enabled ? "on" : "off"}</Badge>
                  </td>
                  <td className="p-3 text-right">
                    <button onClick={() => setEditing(o)} className="rounded p-1 text-muted-foreground hover:bg-muted">
                      <Pencil className="size-4" />
                    </button>
                    <button onClick={() => o.id && del.mutate(o.id)} className="rounded p-1 text-muted-foreground hover:text-destructive">
                      <Trash2 className="size-4" />
                    </button>
                  </td>
                </tr>
              ))}
            </tbody>
          </table>
          {list.data && list.data.length === 0 && (
            <div className="p-6 text-center text-sm text-muted-foreground">No outputs configured.</div>
          )}
        </CardContent>
      </Card>

      {editing && (
        <OutputEditor
          initial={editing}
          streams={streamList.data ?? []}
          onClose={() => setEditing(null)}
          onSaved={() => {
            setEditing(null);
            qc.invalidateQueries({ queryKey: ["outputs"] });
          }}
        />
      )}
    </div>
  );
}

function OutputEditor({
  initial,
  streams: streamOpts,
  onClose,
  onSaved,
}: {
  initial: Output;
  streams: { id?: string; name: string }[];
  onClose: () => void;
  onSaved: () => void;
}) {
  const [o, setO] = useState<Output>(initial);
  const save = useMutation({
    mutationFn: () => (o.id ? outputs.update(o.id, o) : outputs.create(o)),
    onSuccess: onSaved,
  });
  const needsAddr = o.type !== "stdout";

  return (
    <Modal title={o.id ? "Edit output" : "New output"} onClose={onClose}>
      <form
        className="space-y-3"
        onSubmit={(e) => {
          e.preventDefault();
          if (o.name.trim()) save.mutate();
        }}
      >
        <Field label="Name">
          <input className={inputClass} value={o.name} onChange={(e) => setO({ ...o, name: e.target.value })} />
        </Field>
        <div className="grid grid-cols-2 gap-3">
          <Field label="Type">
            <select className={selectClass} value={o.type} onChange={(e) => setO({ ...o, type: e.target.value as Output["type"] })}>
              <option value="gelf">GELF</option>
              <option value="tcp_raw">Raw TCP</option>
              <option value="tcp_syslog">TCP syslog (RFC 5424)</option>
              <option value="stdout">STDOUT</option>
            </select>
          </Field>
          <Field label="Stream (blank = all)">
            <select className={selectClass} value={o.stream ?? ""} onChange={(e) => setO({ ...o, stream: e.target.value })}>
              <option value="">All streams</option>
              <option value="default">default</option>
              {streamOpts.map((s) => (
                <option key={s.id} value={s.id}>
                  {s.name}
                </option>
              ))}
            </select>
          </Field>
        </div>
        {needsAddr && (
          <div className="grid grid-cols-2 gap-3">
            <Field label="Address (host:port)">
              <input className={`${inputClass} font-mono`} placeholder="10.0.0.5:12201" value={o.address ?? ""} onChange={(e) => setO({ ...o, address: e.target.value })} />
            </Field>
            {o.type === "gelf" && (
              <Field label="Protocol">
                <select className={selectClass} value={o.protocol ?? "tcp"} onChange={(e) => setO({ ...o, protocol: e.target.value })}>
                  <option value="tcp">TCP</option>
                  <option value="udp">UDP</option>
                </select>
              </Field>
            )}
          </div>
        )}
        <label className="flex items-center gap-2 text-sm">
          <input type="checkbox" checked={o.enabled} onChange={(e) => setO({ ...o, enabled: e.target.checked })} /> Enabled
        </label>
        <div className="flex justify-end gap-2">
          <Button type="button" variant="ghost" onClick={onClose}>
            Cancel
          </Button>
          <Button type="submit" disabled={!o.name.trim() || save.isPending}>
            Save
          </Button>
        </div>
      </form>
    </Modal>
  );
}
